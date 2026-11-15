//! HAKAN masaüstü kabuğu (Tauri v2). Bilinçli olarak ince tutuldu
//! (docs/01 §4.2, D5): pencere, OS anahtar zinciri, güncelleyici ve Go
//! sidecar'ın yaşam döngüsü. İş mantığı Go sidecar'dadır
//! (`desktop/sidecar`).

pub mod bridge;
mod secrets;

use std::sync::{Arc, Mutex, RwLock};
use std::time::Duration;

use serde_json::Value;
use tauri::{AppHandle, Emitter, Manager, State};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;

use bridge::{Bridge, LineBuffer, RpcError};

/// Sidecar'ın ikili adı (`bundle.externalBin` ile aynı).
const SIDECAR: &str = "hakan-sidecar";

/// Ön yüzün çağırabileceği sidecar yöntemleri. Yeni yöntem (#51 yerel ajan)
/// eklenince buraya da yazılır; liste dışı istek sidecar'a ulaşmaz.
const ALLOWED_METHODS: &[&str] = &["ping", "version", "ubl.inspect", "api.health"];

const CALL_TIMEOUT: Duration = Duration::from_secs(30);

#[derive(Default)]
struct SidecarState {
    bridge: RwLock<Option<Arc<Bridge>>>,
    child: Arc<Mutex<Option<CommandChild>>>,
    /// Son `ready` olayının verisi (sürüm, pid…).
    ready: Mutex<Option<Value>>,
}

struct UpdaterEnabled(bool);

fn start_sidecar(app: &AppHandle) -> Result<(), String> {
    let state = app.state::<SidecarState>();
    if let Some(old) = state.child.lock().unwrap().take() {
        let _ = old.kill();
    }
    *state.ready.lock().unwrap() = None;

    let (mut rx, child) = app
        .shell()
        .sidecar(SIDECAR)
        .map_err(|e| e.to_string())?
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
                            }
                            let _ = app.emit("sidecar://event", ev);
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
                    break;
                }
                _ => {}
            }
        }
    });
    Ok(())
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
    start_sidecar(&app)
}

#[derive(serde::Serialize)]
struct UpdateInfo {
    enabled: bool,
    current: String,
    available: Option<String>,
    notes: Option<String>,
}

#[tauri::command]
async fn update_check(
    app: AppHandle,
    enabled: State<'_, UpdaterEnabled>,
) -> Result<UpdateInfo, String> {
    let current = app.package_info().version.to_string();
    if !enabled.0 {
        return Ok(UpdateInfo {
            enabled: false,
            current,
            available: None,
            notes: None,
        });
    }
    use tauri_plugin_updater::UpdaterExt;
    let update = app
        .updater()
        .map_err(|e| e.to_string())?
        .check()
        .await
        .map_err(|e| e.to_string())?;
    Ok(UpdateInfo {
        enabled: true,
        current,
        available: update.as_ref().map(|u| u.version.clone()),
        notes: update.and_then(|u| u.body),
    })
}

#[tauri::command]
async fn update_install(app: AppHandle, enabled: State<'_, UpdaterEnabled>) -> Result<(), String> {
    if !enabled.0 {
        return Err("güncelleyici bu derlemede kapalı".into());
    }
    use tauri_plugin_updater::UpdaterExt;
    let Some(update) = app
        .updater()
        .map_err(|e| e.to_string())?
        .check()
        .await
        .map_err(|e| e.to_string())?
    else {
        return Ok(());
    };
    update
        .download_and_install(|_, _| {}, || {})
        .await
        .map_err(|e| e.to_string())?;
    app.restart();
}

/// Güncelleyici yalnızca yapılandırmada açık anahtar ve uç tanımlıysa
/// açılır (sürüm derlemesi `tauri.release.conf.json` ile verir). Yerel ve CI
/// derlemeleri anahtarsız çalışır.
fn updater_configured(app: &tauri::App) -> bool {
    app.config()
        .plugins
        .0
        .get("updater")
        .map(|u| {
            let key = u.get("pubkey").and_then(Value::as_str).unwrap_or("");
            let has_endpoint = u
                .get("endpoints")
                .and_then(Value::as_array)
                .is_some_and(|e| !e.is_empty());
            !key.trim().is_empty() && has_endpoint
        })
        .unwrap_or(false)
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

#[cfg_attr(mobile, tauri::mobile_entry_point)]
pub fn run() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_process::init())
        .manage(SidecarState::default())
        .setup(|app| {
            let updater = updater_configured(app);
            if updater {
                app.handle()
                    .plugin(tauri_plugin_updater::Builder::new().build())?;
            }
            app.manage(UpdaterEnabled(updater));
            if let Err(e) = start_sidecar(app.handle()) {
                log::error!("sidecar başlatılamadı: {e}");
            }
            if std::env::args().any(|a| a == "--self-test") {
                self_test(app.handle().clone());
            }
            Ok(())
        })
        .on_window_event(|window, event| {
            if let tauri::WindowEvent::Destroyed = event {
                if let Some(child) = window.state::<SidecarState>().child.lock().unwrap().take() {
                    let _ = child.kill();
                }
            }
        })
        .invoke_handler(tauri::generate_handler![
            sidecar_call,
            sidecar_status,
            sidecar_restart,
            update_check,
            update_install,
            secrets::secret_set,
            secrets::secret_get,
            secrets::secret_exists,
            secrets::secret_delete,
        ])
        .run(tauri::generate_context!())
        .expect("HAKAN masaüstü başlatılamadı");
}
