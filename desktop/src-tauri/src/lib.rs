//! HAKAN masaüstü kabuğu (Tauri v2). Bilinçli olarak ince tutuldu
//! (docs/01 §4.2, D5): pencere, OS anahtar zinciri, güncelleyici ve Go
//! sidecar'ın yaşam döngüsü. İş mantığı Go sidecar'dadır
//! (`desktop/sidecar`).

pub mod bridge;
pub mod logmask;
mod secrets;
pub mod supervisor;
pub mod updater;

use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex, RwLock};
use std::time::{Duration, Instant};

use serde_json::Value;
use tauri::menu::{CheckMenuItem, Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{AppHandle, Emitter, Manager, RunEvent, State, WindowEvent};
use tauri_plugin_autostart::{MacosLauncher, ManagerExt};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

use bridge::{Bridge, LineBuffer, RpcError};
use supervisor::{Backoff, Mode};

/// Sidecar'ın ikili adı (`bundle.externalBin` ile aynı).
const SIDECAR: &str = "hakan-sidecar";

/// Ön yüzün çağırabileceği sidecar yöntemleri. Yeni yöntem (#51 yerel ajan)
/// eklenince buraya da yazılır; liste dışı istek sidecar'a ulaşmaz.
const ALLOWED_METHODS: &[&str] = &[
    "ping",
    "version",
    "ubl.inspect",
    "api.health",
    // Yerel ajan (#51). `auth.resume` listede yok: yalnızca kabuk, anahtar
    // zincirindeki belirteçle çağırır.
    "agent.status",
    "auth.login",
    "auth.logout",
    "agent.organizations",
    "agent.use_organization",
    "agent.engagements",
    "watch.add",
    "watch.remove",
    "watch.clear_errors",
    "source.test",
    "source.preview",
];

/// Oturum yenileme belirtecinin anahtar zincirindeki adı.
const REFRESH_KEY: &str = "refresh_token";

const CALL_TIMEOUT: Duration = Duration::from_secs(30);

/// Oturum açılışında başlatılan uygulama bu argümanla açılır: pencere
/// gösterilmez, yalnızca tepside çalışır (#204).
const BACKGROUND_ARG: &str = "--background";

/// Ana pencerenin etiketi (tauri.conf.json).
const MAIN_WINDOW: &str = "main";

struct SidecarState {
    bridge: RwLock<Option<Arc<Bridge>>>,
    child: Arc<Mutex<Option<CommandChild>>>,
    /// Son `ready` olayının verisi (sürüm, pid…).
    ready: Mutex<Option<Value>>,
    /// Her başlatmada artar; geç gelen eski kapanma olayları yok sayılır.
    generation: AtomicU64,
    /// Kabuğun istediği durum: çalışsın / duraklatıldı / kapanıyor (#204).
    mode: Mutex<Mode>,
    backoff: Mutex<Backoff>,
    started_at: Mutex<Instant>,
}

impl Default for SidecarState {
    fn default() -> Self {
        Self {
            bridge: RwLock::default(),
            child: Arc::default(),
            ready: Mutex::default(),
            generation: AtomicU64::new(0),
            mode: Mutex::new(Mode::Running),
            backoff: Mutex::default(),
            started_at: Mutex::new(Instant::now()),
        }
    }
}

/// Güncelleyici durumu (#205). `cfg` derleme zamanında belirlenir; hatalıysa
/// güncelleyici kapalıdır ve ön yüz nedeni gösterir.
struct UpdaterState {
    cfg: Result<updater::Config, updater::Disabled>,
    status: Mutex<updater::Status>,
    /// İndirme / kurulum sürüyor mu? Aynı anda ikinci kurulum başlamaz.
    installing: AtomicBool,
}

fn start_sidecar(app: &AppHandle) -> Result<(), String> {
    let state = app.state::<SidecarState>();
    if let Some(old) = state.child.lock().unwrap().take() {
        let _ = old.kill();
    }
    *state.ready.lock().unwrap() = None;
    let generation = state.generation.fetch_add(1, Ordering::SeqCst) + 1;
    *state.started_at.lock().unwrap() = Instant::now();

    let data_dir = app.path().app_data_dir().map_err(|e| e.to_string())?;
    let (mut rx, child) = app
        .shell()
        .sidecar(SIDECAR)
        .map_err(|e| e.to_string())?
        .args(["--data-dir".as_ref(), data_dir.as_os_str()])
        .set_raw_out(true)
        .spawn()
        .map_err(|e| e.to_string())?;
    *state.child.lock().unwrap() = Some(child);

    let child_ref = state.child.clone();
    let bridge = Arc::new(Bridge::new(Box::new(move |line| {
        let mut guard = child_ref.lock().unwrap();
        match guard.as_mut() {
            Some(c) => c.write(&line).map_err(|e| e.to_string()),
            None => Err("sidecar yok".into()),
        }
    })));
    *state.bridge.write().unwrap() = Some(bridge.clone());

    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let mut lines = LineBuffer::default();
        while let Some(ev) = rx.recv().await {
            match ev {
                CommandEvent::Stdout(chunk) => {
                    for line in lines.push(&chunk) {
                        if let Some(ev) = bridge.on_line(&line) {
                            if ev.event == "ready" {
                                *app.state::<SidecarState>().ready.lock().unwrap() =
                                    Some(ev.data.clone());
                                resume_session(bridge.clone());
                            }
                            if !handle_private_event(&ev) {
                                let _ = app.emit("sidecar://event", ev);
                            }
                        }
                    }
                }
                CommandEvent::Stderr(chunk) => {
                    log::info!("sidecar: {}", String::from_utf8_lossy(&chunk).trim_end());
                }
                CommandEvent::Error(e) => log::error!("sidecar: {e}"),
                CommandEvent::Terminated(p) => {
                    log::warn!("sidecar kapandı: code={:?} signal={:?}", p.code, p.signal);
                    bridge.on_exit();
                    let _ = app.emit("sidecar://exit", p.code);
                    schedule_restart(&app, generation);
                    break;
                }
                _ => {}
            }
        }
    });
    Ok(())
}

/// Kapanan sidecar'ı geri çekilmeli olarak yeniden başlatır (#204). Kabuk
/// duraklatıldıysa ya da kapanıyorsa ya da bu süreç artık güncel değilse
/// hiçbir şey yapmaz.
fn schedule_restart(app: &AppHandle, generation: u64) {
    let state = app.state::<SidecarState>();
    let mode = *state.mode.lock().unwrap();
    if !supervisor::should_restart(mode, generation, state.generation.load(Ordering::SeqCst)) {
        return;
    }
    let ran = state.started_at.lock().unwrap().elapsed();
    let delay = state.backoff.lock().unwrap().on_exit(ran);
    log::warn!("sidecar {} sn sonra yeniden başlatılacak", delay.as_secs());
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        tokio::time::sleep(delay).await;
        let state = app.state::<SidecarState>();
        // Beklerken duraklatıldı, kapanıyor ya da elle başlatıldıysa vazgeç.
        let mode = *state.mode.lock().unwrap();
        if !supervisor::should_restart(mode, generation, state.generation.load(Ordering::SeqCst)) {
            return;
        }
        if let Err(e) = start_sidecar(&app) {
            log::error!("sidecar yeniden başlatılamadı: {e}");
            schedule_restart(&app, state.generation.load(Ordering::SeqCst));
        }
    });
}

/// Sidecar'ı durdurur ve yeniden başlatılmamasını sağlar.
fn stop_sidecar(app: &AppHandle, mode: Mode) {
    let state = app.state::<SidecarState>();
    *state.mode.lock().unwrap() = mode;
    let child = state.child.lock().unwrap().take();
    if let Some(child) = child {
        let _ = child.kill();
    }
}

/// İzlemeyi duraklatır ya da sürdürür (tepsi menüsü). Duraklatmak sidecar'ı
/// durdurur: klasör izleme, kuyruk ve sunucu bağlantısı birlikte durur.
fn set_paused(app: &AppHandle, paused: bool) -> Result<(), String> {
    if paused {
        stop_sidecar(app, Mode::Paused);
        let _ = app.emit("shell://paused", true);
        return Ok(());
    }
    let state = app.state::<SidecarState>();
    *state.mode.lock().unwrap() = Mode::Running;
    state.backoff.lock().unwrap().reset();
    let _ = app.emit("shell://paused", false);
    start_sidecar(app)
}

fn is_paused(app: &AppHandle) -> bool {
    *app.state::<SidecarState>().mode.lock().unwrap() == Mode::Paused
}

/// Ana pencereyi gösterir ve öne getirir.
fn show_main(app: &AppHandle) {
    if let Some(w) = app.get_webview_window(MAIN_WINDOW) {
        let _ = w.unminimize();
        let _ = w.show();
        let _ = w.set_focus();
    }
}

/// Uygulamadan gerçekten çıkar: sidecar durur, yeniden başlatılmaz.
fn quit(app: &AppHandle) {
    stop_sidecar(app, Mode::ShuttingDown);
    app.exit(0);
}

/// Tepsi simgesi ve menüsü: Aç, İzlemeyi duraklat/sürdür, Oturum açılışında
/// başlat, Çıkış (#204).
fn build_tray(app: &AppHandle) -> tauri::Result<()> {
    let open = MenuItem::with_id(app, "open", "Aç", true, None::<&str>)?;
    let pause = MenuItem::with_id(app, "pause", "İzlemeyi duraklat", true, None::<&str>)?;
    let autostart_on = app.autolaunch().is_enabled().unwrap_or(false);
    let autostart = CheckMenuItem::with_id(
        app,
        "autostart",
        "Oturum açılışında başlat",
        true,
        autostart_on,
        None::<&str>,
    )?;
    let quit_item = MenuItem::with_id(app, "quit", "Çıkış", true, None::<&str>)?;
    let menu = Menu::with_items(
        app,
        &[
            &open,
            &pause,
            &PredefinedMenuItem::separator(app)?,
            &autostart,
            &PredefinedMenuItem::separator(app)?,
            &quit_item,
        ],
    )?;

    let pause_item = pause.clone();
    let autostart_item = autostart.clone();
    let mut tray = TrayIconBuilder::with_id("main")
        .tooltip("Hakan")
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(move |app, event| match event.id().as_ref() {
            "open" => show_main(app),
            "pause" => {
                let paused = !is_paused(app);
                if let Err(e) = set_paused(app, paused) {
                    log::error!("izleme durumu değiştirilemedi: {e}");
                }
                let _ = pause_item.set_text(if is_paused(app) {
                    "İzlemeyi sürdür"
                } else {
                    "İzlemeyi duraklat"
                });
            }
            "autostart" => {
                let want = !app.autolaunch().is_enabled().unwrap_or(false);
                if let Err(e) = set_autostart(app, want) {
                    log::error!("otomatik başlatma ayarlanamadı: {e}");
                }
                let _ = autostart_item.set_checked(app.autolaunch().is_enabled().unwrap_or(false));
            }
            "quit" => quit(app),
            _ => {}
        })
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                show_main(tray.app_handle());
            }
        });
    if let Some(icon) = app.default_window_icon() {
        tray = tray.icon(icon.clone());
    }
    tray.build(app)?;
    Ok(())
}

fn set_autostart(app: &AppHandle, enabled: bool) -> Result<(), String> {
    let launcher = app.autolaunch();
    let res = if enabled {
        launcher.enable()
    } else {
        launcher.disable()
    };
    res.map_err(|e| e.to_string())
}

/// Oturum olayları kabukta kalır: yenileme belirteci ön yüze hiç gitmez.
/// `true` dönerse olay ön yüze iletilmez.
fn handle_private_event(ev: &bridge::Event) -> bool {
    match ev.event.as_str() {
        "auth.rotated" => {
            if let Some(t) = ev.data.get("refresh_token").and_then(Value::as_str) {
                if let Err(e) = secrets::store(REFRESH_KEY, t) {
                    log::error!("oturum belirteci anahtar zincirine yazılamadı: {e}");
                }
            }
            true
        }
        "auth.signed_out" => {
            let _ = secrets::remove(REFRESH_KEY);
            false
        }
        _ => false,
    }
}

/// Sidecar hazır olunca anahtar zincirindeki belirteçle oturumu sürdürür.
fn resume_session(bridge: Arc<Bridge>) {
    let Ok(Some(token)) = secrets::load(REFRESH_KEY) else {
        return;
    };
    tauri::async_runtime::spawn(async move {
        let params = serde_json::json!({ "refresh_token": token });
        if let Err(e) = bridge.call("auth.resume", params, CALL_TIMEOUT).await {
            log::warn!("oturum sürdürülemedi: {e}");
            if e.code == "SIGNED_OUT" {
                let _ = secrets::remove(REFRESH_KEY);
            }
        }
    });
}

fn current_bridge(state: &SidecarState) -> Result<Arc<Bridge>, RpcError> {
    state
        .bridge
        .read()
        .unwrap()
        .clone()
        .filter(|b| b.is_alive())
        .ok_or_else(|| RpcError::new("SIDECAR_DOWN", "sidecar çalışmıyor"))
}

#[tauri::command]
async fn sidecar_call(
    state: State<'_, SidecarState>,
    method: String,
    params: Option<Value>,
) -> Result<Value, RpcError> {
    if !ALLOWED_METHODS.contains(&method.as_str()) {
        return Err(RpcError::new(
            "FORBIDDEN_METHOD",
            format!("izin verilmeyen yöntem: {method}"),
        ));
    }
    let bridge = current_bridge(&state)?;
    bridge
        .call(
            &method,
            params.unwrap_or(Value::Object(Default::default())),
            CALL_TIMEOUT,
        )
        .await
}

#[derive(serde::Serialize)]
struct SidecarStatus {
    alive: bool,
    ready: Option<Value>,
}

#[tauri::command]
fn sidecar_status(state: State<'_, SidecarState>) -> SidecarStatus {
    SidecarStatus {
        alive: current_bridge(&state).is_ok(),
        ready: state.ready.lock().unwrap().clone(),
    }
}

#[tauri::command]
fn sidecar_restart(app: AppHandle) -> Result<(), String> {
    set_paused(&app, false)
}

/// İşletim sisteminin klasör seçicisi (#206): kullanıcı yol yazmaz. İptal
/// edilirse `None`. Diyalog yalnızca kabuktan açılır; ön yüze dosya sistemi
/// izni verilmez.
#[tauri::command]
async fn pick_folder(app: AppHandle) -> Result<Option<String>, String> {
    use tauri_plugin_dialog::DialogExt;
    let (tx, rx) = tokio::sync::oneshot::channel();
    let mut dialog = app.dialog().file().set_title("İzlenecek klasörü seçin");
    if let Some(w) = app.get_webview_window(MAIN_WINDOW) {
        dialog = dialog.set_parent(&w);
    }
    dialog.pick_folder(move |p| {
        let _ = tx.send(p);
    });
    let picked = rx.await.map_err(|_| "Klasör seçici kapandı.".to_string())?;
    Ok(picked
        .and_then(|p| p.into_path().ok())
        .map(|p| p.to_string_lossy().into_owned()))
}

#[derive(serde::Serialize)]
struct ShellSettings {
    autostart: bool,
    paused: bool,
}

/// Arka plan ayarları (#204): otomatik başlatma ve izleme durumu.
#[tauri::command]
fn shell_settings(app: AppHandle) -> ShellSettings {
    ShellSettings {
        autostart: app.autolaunch().is_enabled().unwrap_or(false),
        paused: is_paused(&app),
    }
}

#[tauri::command]
fn set_autostart_enabled(app: AppHandle, enabled: bool) -> Result<bool, String> {
    set_autostart(&app, enabled)?;
    app.autolaunch().is_enabled().map_err(|e| e.to_string())
}

#[tauri::command]
fn set_watch_paused(app: AppHandle, paused: bool) -> Result<bool, String> {
    set_paused(&app, paused)?;
    Ok(is_paused(&app))
}

fn update_settings_dir(app: &AppHandle) -> Option<std::path::PathBuf> {
    app.path().app_config_dir().ok()
}

/// Durumu değiştirir ve ön yüze `updater://status` olarak bildirir.
fn set_update_status(app: &AppHandle, f: impl FnOnce(&mut updater::Status)) -> updater::Status {
    let state = app.state::<UpdaterState>();
    let snapshot = {
        let mut st = state.status.lock().unwrap();
        f(&mut st);
        st.clone()
    };
    let _ = app.emit("updater://status", &snapshot);
    snapshot
}

/// Seçili kanalın manifestini denetler. Kapalıysa ağa çıkmaz.
async fn run_update_check(app: &AppHandle) -> updater::Status {
    let state = app.state::<UpdaterState>();
    let Ok(cfg) = state.cfg.clone() else {
        return state.status.lock().unwrap().clone();
    };
    if state.installing.load(Ordering::SeqCst) {
        return state.status.lock().unwrap().clone();
    }
    let channel = state.status.lock().unwrap().channel;
    set_update_status(app, |s| {
        s.state = "checking";
        s.error = None;
    });
    let result = match updater::updater_for(app, &cfg, channel, None) {
        Ok(u) => u.check().await.map_err(|e| e.to_string()),
        Err(e) => Err(e),
    };
    let now = chrono_now();
    match result {
        Ok(Some(u)) => {
            log::info!("güncelleme var: {} ({})", u.version, channel.as_str());
            set_update_status(app, |s| {
                s.state = "available";
                s.available = Some(u.version.clone());
                s.notes = u.body.clone();
                s.checked_at = Some(now);
            })
        }
        Ok(None) => set_update_status(app, |s| {
            s.state = "up_to_date";
            s.available = None;
            s.notes = None;
            s.checked_at = Some(now);
        }),
        Err(e) => {
            log::warn!("güncelleme denetlenemedi: {e}");
            set_update_status(app, |s| {
                s.state = "error";
                s.error = Some(updater::user_error(&e));
            })
        }
    }
}

/// Açılıştan kısa süre sonra ve 6 saatte bir denetler (#205).
fn spawn_update_loop(app: AppHandle) {
    tauri::async_runtime::spawn(async move {
        tokio::time::sleep(updater::FIRST_CHECK_DELAY).await;
        loop {
            run_update_check(&app).await;
            tokio::time::sleep(updater::CHECK_INTERVAL).await;
        }
    });
}

/// Güncel durum (ağa çıkmaz).
#[tauri::command]
fn update_status(state: State<'_, UpdaterState>) -> updater::Status {
    state.status.lock().unwrap().clone()
}

/// Şimdi denetle.
#[tauri::command]
async fn update_check(app: AppHandle) -> Result<updater::Status, String> {
    Ok(run_update_check(&app).await)
}

/// Kanalı (stable / beta) değiştirir, kaydeder ve yeniden denetler.
#[tauri::command]
async fn update_set_channel(app: AppHandle, channel: String) -> Result<updater::Status, String> {
    let ch = updater::Channel::parse(&channel).ok_or("Bilinmeyen güncelleme kanalı.")?;
    if let Some(dir) = update_settings_dir(&app) {
        if let Err(e) = updater::save_channel(&dir, ch) {
            log::error!("güncelleme kanalı kaydedilemedi: {e}");
            return Err("Kanal seçimi kaydedilemedi.".into());
        }
    }
    set_update_status(&app, |s| {
        s.channel = ch;
        s.available = None;
        s.notes = None;
    });
    Ok(run_update_check(&app).await)
}

/// Güncellemeyi indirir (ilerleme `updater://progress`), kurar ve uygulamayı
/// yeniden başlatır. Güncelleme yoksa hiçbir şey yapmaz.
#[tauri::command]
async fn update_install(app: AppHandle) -> Result<(), String> {
    let state = app.state::<UpdaterState>();
    let Ok(cfg) = state.cfg.clone() else {
        return Err("Bu derlemede otomatik güncelleme kapalı.".into());
    };
    if state.installing.swap(true, Ordering::SeqCst) {
        return Err("Güncelleme zaten indiriliyor.".into());
    }
    let channel = state.status.lock().unwrap().channel;
    let result = install_update(&app, &cfg, channel).await;
    state.installing.store(false, Ordering::SeqCst);
    match result {
        Ok(true) => {
            log::info!("güncelleme kuruldu, yeniden başlatılıyor");
            stop_sidecar(&app, Mode::ShuttingDown);
            app.restart();
        }
        Ok(false) => {
            set_update_status(&app, |s| {
                s.state = "up_to_date";
                s.available = None;
            });
            Ok(())
        }
        Err(e) => {
            log::error!("güncelleme kurulamadı: {e}");
            let msg = updater::user_error(&e);
            set_update_status(&app, |s| {
                s.state = "error";
                s.error = Some(msg.clone());
            });
            Err(msg)
        }
    }
}

async fn install_update(
    app: &AppHandle,
    cfg: &updater::Config,
    channel: updater::Channel,
) -> Result<bool, String> {
    let u = updater::updater_for(app, cfg, channel, None)?;
    let Some(update) = u.check().await.map_err(|e| e.to_string())? else {
        return Ok(false);
    };
    set_update_status(app, |s| {
        s.state = "downloading";
        s.available = Some(update.version.clone());
        s.error = None;
    });
    let mut progress = updater::Progress::default();
    let progress_app = app.clone();
    let done_app = app.clone();
    update
        .download_and_install(
            move |chunk, total| {
                progress.add(chunk, total);
                let _ = progress_app.emit("updater://progress", progress);
            },
            move || {
                set_update_status(&done_app, |s| s.state = "installing");
            },
        )
        .await
        .map_err(|e| e.to_string())?;
    Ok(true)
}

/// `--self-test`: uygulama açılır, sidecar'a ping atar, sonucu stdout'a tek
/// satır JSON olarak yazar ve çıkar (0 başarı, 1 hata). Sürüm iş akışı imzalı
/// paketi bununla dener: "açılıyor ve sidecar ile konuşuyor" (#50).
fn self_test(app: AppHandle) {
    tauri::async_runtime::spawn(async move {
        let started = std::time::Instant::now();
        let mut result = Err(RpcError::new("SIDECAR_DOWN", "sidecar başlamadı"));
        while started.elapsed() < Duration::from_secs(20) {
            let bridge = current_bridge(&app.state::<SidecarState>());
            if let Ok(b) = bridge {
                result = b
                    .call(
                        "version",
                        Value::Object(Default::default()),
                        Duration::from_secs(5),
                    )
                    .await;
                if result.is_ok() {
                    break;
                }
            }
            tokio::time::sleep(Duration::from_millis(200)).await;
        }
        let line = match &result {
            Ok(v) => serde_json::json!({
                "ok": true,
                "app_version": app.package_info().version.to_string(),
                "sidecar": v,
                "elapsed_ms": started.elapsed().as_millis() as u64,
            }),
            Err(e) => serde_json::json!({ "ok": false, "error": e }),
        };
        println!("{line}");
        app.exit(if result.is_ok() { 0 } else { 1 });
    });
}

/// Kalıcı günlük (#204): uygulamanın günlük klasörüne, dönerek (5 × 2 MB)
/// ve maskelenerek (`logmask`) yazılır; geliştirmede stdout'a da gider.
fn log_plugin() -> tauri::plugin::TauriPlugin<tauri::Wry> {
    use tauri_plugin_log::{RotationStrategy, Target, TargetKind};
    tauri_plugin_log::Builder::new()
        .targets([
            Target::new(TargetKind::LogDir {
                file_name: Some("hakan".into()),
            }),
            Target::new(TargetKind::Stdout),
        ])
        .level(log::LevelFilter::Info)
        .max_file_size(2 * 1024 * 1024)
        .rotation_strategy(RotationStrategy::KeepSome(5))
        .format(|out, message, record| {
            out.finish(format_args!(
                "{} {} {} {}",
                chrono_now(),
                record.level(),
                record.target(),
                logmask::mask(&message.to_string())
            ))
        })
        .build()
}

/// Günlük satırı için UTC zaman damgası (RFC 3339).
fn chrono_now() -> String {
    time::OffsetDateTime::now_utc()
        .format(&time::format_description::well_known::Rfc3339)
        .unwrap_or_default()
}

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    let background = std::env::args().any(|a| a == BACKGROUND_ARG);
    let self_test_mode = std::env::args().any(|a| a == "--self-test");

    let app = tauri::Builder::default()
        // Tek örnek: ikinci açılış yeni süreç başlatmaz, açık pencereyi öne getirir.
        .plugin(tauri_plugin_single_instance::init(|app, _args, _cwd| {
            show_main(app);
        }))
        .plugin(log_plugin())
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_dialog::init())
        .plugin(tauri_plugin_process::init())
        .plugin(tauri_plugin_autostart::init(
            MacosLauncher::LaunchAgent,
            Some(vec![BACKGROUND_ARG]),
        ))
        .manage(SidecarState::default())
        .setup(move |app| {
            // Güncelleyici (#205): anahtar ve adres derleme zamanında gelir.
            let cfg = updater::Config::from_build();
            let version = app.package_info().version.to_string();
            let channel = app
                .path()
                .app_config_dir()
                .map(|d| updater::load_channel(&d, &version))
                .unwrap_or_else(|_| updater::Channel::default_for(&version));
            if let Err(d) = &cfg {
                log::info!("güncelleyici kapalı: {d:?}");
            }
            let status = updater::Status::initial(version, channel, &cfg);
            let enabled = cfg.is_ok();
            if let Ok(c) = &cfg {
                app.handle().plugin(
                    tauri_plugin_updater::Builder::new()
                        .pubkey(c.pubkey.clone())
                        .build(),
                )?;
            }
            app.manage(UpdaterState {
                cfg,
                status: Mutex::new(status),
                installing: AtomicBool::new(false),
            });
            if let Err(e) = start_sidecar(app.handle()) {
                log::error!("sidecar başlatılamadı: {e}");
            }
            if self_test_mode {
                self_test(app.handle().clone());
                return Ok(());
            }
            if enabled {
                spawn_update_loop(app.handle().clone());
            }
            // Tepsi yoksa (ör. masaüstü ortamı desteklemiyor) pencere kapanınca
            // uygulama görünmez kalmasın diye gizleme yerine çıkış yapılır.
            let tray_ok = match build_tray(app.handle()) {
                Ok(()) => true,
                Err(e) => {
                    log::error!("tepsi simgesi kurulamadı: {e}");
                    false
                }
            };
            app.manage(TrayReady(tray_ok));
            if background {
                if let Some(w) = app.get_webview_window(MAIN_WINDOW) {
                    let _ = w.hide();
                }
            }
            Ok(())
        })
        .on_window_event(|window, event| {
            // Pencere kapanınca uygulama tepside kalır; sidecar ve klasör
            // izleme sürer (#204). Çıkış tepsi menüsünden.
            if let WindowEvent::CloseRequested { api, .. } = event {
                let tray = window
                    .try_state::<TrayReady>()
                    .map(|t| t.0)
                    .unwrap_or(false);
                if tray {
                    api.prevent_close();
                    let _ = window.hide();
                }
            }
        })
        .invoke_handler(tauri::generate_handler![
            sidecar_call,
            sidecar_status,
            sidecar_restart,
            shell_settings,
            set_autostart_enabled,
            set_watch_paused,
            pick_folder,
            update_status,
            update_check,
            update_set_channel,
            update_install,
            secrets::secret_set,
            secrets::secret_get,
            secrets::secret_exists,
            secrets::secret_delete,
        ])
        .build(tauri::generate_context!())
        .expect("HAKAN masaüstü başlatılamadı");

    app.run(|app, event| {
        if let RunEvent::Exit = event {
            // Hangi yoldan çıkılırsa çıkılsın sidecar arkada kalmaz.
            stop_sidecar(app, Mode::ShuttingDown);
        }
    });
}

/// Tepsi simgesi kuruldu mu? Kurulamadıysa pencere kapanınca çıkılır.
struct TrayReady(bool);
