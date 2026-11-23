//! Ofis paneli (tek program): web ofis uygulaması aynı programda, ayrı bir
//! pencerede (`office`) açılır.
//!
//! Güvenlik:
//! - Bu pencereye hiçbir Tauri yeteneği verilmez: `capabilities/default.json`
//!   yalnızca `main`i hedefler, eklenti komutları (olay dinleme dahil) ACL'de
//!   reddedilir. Uygulama komutları ACL dışında kaldığı için ayrıca
//!   [`ipc_allowed`] ile yalnızca ana pencereden kabul edilir (lib.rs). Uzak
//!   sayfada `window.__TAURI__` nesnesi görünse de hiçbir çağrısı karşılık
//!   bulmaz.
//! - Gezinme yalnızca web origin'inde kalır ([`navigation`]); başka
//!   bağlantılar ve yeni pencereler sistem tarayıcısında açılır.
//! - İndirmeler yalnızca kendi origin'inden kabul edilir ve Ayarlar'da seçilen
//!   klasöre kaydedilir ("Luca dosyasını indir").
//! - Parola saklanmaz: kullanıcı panelde bir kez oturum açar, çerez webview
//!   profilinde kalır.

use std::collections::HashMap;
use std::ffi::OsStr;
use std::path::{Path, PathBuf};
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;
use std::time::{Duration, Instant};

use serde::{Deserialize, Serialize};
use serde_json::Value;
use tauri::webview::DownloadEvent;
use tauri::{AppHandle, Emitter, Manager, Url, WebviewUrl, WebviewWindowBuilder};

/// Ofis paneli penceresinin etiketi.
pub const OFFICE_WINDOW: &str = "office";

/// İndirme bitti: `{ name, folder, ok }` (yalnızca ana pencereye).
pub const DOWNLOAD_EVENT: &str = "office://download";

/// Yeni belgeler geldi ve ofis paneli açık: `{ count }` (yalnızca ana
/// pencereye). Ön yüz "Paneli yenile" şeridini gösterir.
pub const NEW_DOCUMENTS_EVENT: &str = "office://new-documents";

/// Derleme zamanı web adresi (`HAKAN_WEB_URL`; sürüm iş akışı verir).
const BUILD_WEB_URL: Option<&str> = option_env!("HAKAN_WEB_URL");
/// Derleme zamanı API adresi (`HAKAN_API_BASE_URL`, #829). Web adresi
/// verilmemişse ve kullanıcının kayıtlı adresi yoksa buradan türetilir.
const BUILD_API_URL: Option<&str> = option_env!("HAKAN_API_BASE_URL");

const NO_URL: &str = "Ofis paneli adresi bilinmiyor. Önce sunucu adresiyle oturum açın.";

/// Yeni belge bildirimi en çok bu aralıkta bir gönderilir.
pub const NOTIFY_EVERY: Duration = Duration::from_secs(120);
/// Art arda gelen yüklemeler bu süre beklenip tek bildirimde toplanır.
const NOTIFY_DEBOUNCE: Duration = Duration::from_secs(5);

/// Komutu çağıran pencere uygulama komutlarını kullanabilir mi? Yalnızca
/// yerel arayüzün ana penceresi; ofis paneli (uzak sayfa) hiçbir komutu
/// çağıramaz.
pub fn ipc_allowed(label: &str) -> bool {
    label == crate::MAIN_WINDOW
}

fn is_local_host(host: &str) -> bool {
    matches!(host, "localhost" | "127.0.0.1" | "[::1]" | "::1")
}

/// Web adresini doğrular: uzak sunucu için https, düz http yalnızca yerel
/// geliştirmede (sidecar'daki API adresi kuralıyla aynı).
pub fn parse_web_url(raw: &str) -> Option<Url> {
    let mut u = Url::parse(raw.trim()).ok()?;
    let local = is_local_host(u.host_str()?);
    match u.scheme() {
        "https" => {}
        "http" if local => {}
        _ => return None,
    }
    u.set_query(None);
    u.set_fragment(None);
    Some(u)
}

/// API adresinden web adresini türetir: baştaki `api.` → `app.`
/// (`https://api.ornek.com` → `https://app.ornek.com/`). `api.` ile
/// başlamıyorsa panel API ile aynı sunucudadır.
pub fn derive_web_url(api: &str) -> Option<Url> {
    let mut u = parse_web_url(api)?;
    let host = u.host_str()?.to_string();
    if let Some(rest) = host.strip_prefix("api.") {
        if rest.is_empty() {
            return None;
        }
        u.set_host(Some(&format!("app.{rest}"))).ok()?;
    }
    u.set_path("/");
    Some(u)
}

/// Kullanılacak web adresi: derleme değişkeni `HAKAN_WEB_URL`; yoksa
/// kullanıcının sunucu adresinden, o da yoksa derlemedeki API adresinden
/// türetilir.
pub fn resolve_web_url(
    build_web: Option<&str>,
    api: Option<&str>,
    build_api: Option<&str>,
) -> Option<Url> {
    let explicit = build_web.and_then(parse_web_url);
    if explicit.is_some() {
        return explicit;
    }
    let from_api = api.and_then(derive_web_url);
    from_api.or_else(|| build_api.and_then(derive_web_url))
}

/// İki adres aynı origin'de mi? `blob:` adresinin origin'i içindeki
/// adrestir; opak origin (ör. `data:`) hiçbir şeyle eşleşmez.
pub fn same_origin(base: &Url, target: &Url) -> bool {
    let o = target.origin();
    o.is_tuple() && o == base.origin()
}

/// Ofis panelinde gezinme kararı.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Nav {
    /// Panelde açılır.
    Allow,
    /// Sistem tarayıcısında (ya da e-posta / telefon uygulamasında) açılır.
    External,
    /// Hiç açılmaz.
    Block,
}

/// Yalnızca web origin'i panelde açılır; başka http(s) adresleri ve
/// `mailto:` / `tel:` sistemde, geri kalan şemalar (file, javascript, data…)
/// hiç açılmaz.
pub fn navigation(base: &Url, target: &Url) -> Nav {
    match target.scheme() {
        "http" | "https" if same_origin(base, target) => Nav::Allow,
        "http" | "https" => Nav::External,
        "blob" if same_origin(base, target) => Nav::Allow,
        "about" if matches!(target.path(), "blank" | "srcdoc") => Nav::Allow,
        "mailto" | "tel" => Nav::External,
        _ => Nav::Block,
    }
}

/// Yeni pencere isteği hiçbir zaman Tauri penceresi açmaz: http(s) ve
/// e-posta bağlantıları sistemde açılır, gerisi reddedilir.
pub fn new_window(target: &Url) -> Nav {
    match target.scheme() {
        "http" | "https" | "mailto" | "tel" => Nav::External,
        _ => Nav::Block,
    }
}

/// İndirme yalnızca panelin kendi origin'inden (sayfanın oluşturduğu `blob:`
/// dahil) kabul edilir.
pub fn download_allowed(base: &Url, url: &Url) -> bool {
    matches!(url.scheme(), "http" | "https" | "blob") && same_origin(base, url)
}

fn safe_char(c: char) -> char {
    if c.is_control() || matches!(c, ':' | '*' | '?' | '"' | '<' | '>' | '|') {
        '_'
    } else {
        c
    }
}

/// Dosya adını güvenli hâle getirir: yalnızca son bileşen, yol ayırıcı ve
/// denetim karakteri yok, boşsa varsayılan ad.
pub fn safe_file_name(name: &str) -> String {
    let last = name.rsplit(['/', '\\']).next().unwrap_or_default();
    let clean: String = last.chars().map(safe_char).collect();
    let clean = clean.trim().trim_start_matches('.').trim();
    if clean.is_empty() {
        return "hakan-indirme".into();
    }
    clean.to_string()
}

fn os_name(p: Option<&OsStr>) -> String {
    match p {
        Some(s) => s.to_string_lossy().into_owned(),
        None => String::new(),
    }
}

/// Klasörde aynı adlı dosya varsa üzerine yazılmaz: `ad (2).uzantı`.
pub fn unique_path(dir: &Path, name: &str) -> PathBuf {
    let first = dir.join(name);
    if !first.exists() {
        return first;
    }
    let p = Path::new(name);
    let stem = os_name(p.file_stem());
    let ext = match p.extension() {
        Some(e) => format!(".{}", e.to_string_lossy()),
        None => String::new(),
    };
    for i in 2..1000 {
        let candidate = dir.join(format!("{stem} ({i}){ext}"));
        if !candidate.exists() {
            return candidate;
        }
    }
    first
}

/// Yeni belge bildiriminin zamanlaması: dosya başına değil, en çok
/// [`NOTIFY_EVERY`] aralıkta bir özet.
#[derive(Debug, Default)]
pub struct NewDocs {
    pending: u32,
    last: Option<Instant>,
}

/// [`NewDocs::take`] sonucu.
#[derive(Debug, PartialEq, Eq)]
pub enum Take {
    /// Bekleyen yok.
    Nothing,
    /// Şimdi bildir (sayı).
    Notify(u32),
    /// Aralık dolmadı; bu kadar sonra yeniden bak.
    Wait(Duration),
}

impl NewDocs {
    /// Yüklenen bir belgeyi sayar.
    pub fn add(&mut self) {
        self.pending = self.pending.saturating_add(1);
    }

    /// Bildirim zamanı geldiyse bekleyen sayıyı verir ve sıfırlar.
    pub fn take(&mut self, now: Instant) -> Take {
        if self.pending == 0 {
            return Take::Nothing;
        }
        if let Some(last) = self.last {
            let since = now.saturating_duration_since(last);
            if since < NOTIFY_EVERY {
                return Take::Wait(NOTIFY_EVERY - since);
            }
        }
        let n = self.pending;
        self.pending = 0;
        self.last = Some(now);
        Take::Notify(n)
    }
}

/// Bildirim başlığı ve gövdesi.
pub fn new_docs_text(count: u32, office_open: bool) -> (String, String) {
    let title = if count == 1 {
        "HAKAN · yeni belge geldi".to_string()
    } else {
        format!("HAKAN · {count} yeni belge geldi")
    };
    let body = if office_open {
        "Ofis panelinde görmek için Hakan penceresinde Paneli yenile'yi seçin."
    } else {
        "Belgeler ofis paneline gönderildi."
    };
    (title, body.to_string())
}

#[derive(Debug, Default, Serialize, Deserialize)]
struct Settings {
    #[serde(default, skip_serializing_if = "Option::is_none")]
    download_dir: Option<PathBuf>,
}

const SETTINGS_FILE: &str = "office.json";

fn load_settings(dir: &Path) -> Settings {
    let Ok(body) = std::fs::read(dir.join(SETTINGS_FILE)) else {
        return Settings::default();
    };
    serde_json::from_slice(&body).unwrap_or_default()
}

fn save_settings(dir: &Path, s: &Settings) -> std::io::Result<()> {
    std::fs::create_dir_all(dir)?;
    let body = serde_json::to_vec_pretty(s)?;
    std::fs::write(dir.join(SETTINGS_FILE), body)
}

/// Ofis panelinin paylaşılan durumu (`app.manage`).
#[derive(Default)]
pub struct OfficeState {
    download_dir: Mutex<Option<PathBuf>>,
    /// İndirme adresi → kaydedileceği yol (bitince ad bildirilir; macOS
    /// bitişte yolu vermez).
    pending: Mutex<HashMap<String, PathBuf>>,
    new_docs: Mutex<NewDocs>,
    flush_scheduled: AtomicBool,
}

/// Ayarları yükler ve durumu kaydeder (`setup` içinde).
pub fn init(app: &AppHandle) {
    let state = OfficeState::default();
    if let Ok(dir) = app.path().app_config_dir() {
        *state.download_dir.lock().unwrap() = load_settings(&dir).download_dir;
    }
    app.manage(state);
}

fn office_state(app: &AppHandle) -> Option<tauri::State<'_, OfficeState>> {
    app.try_state::<OfficeState>()
}

/// Ayarlarda seçilen indirme klasörü.
fn configured_dir(app: &AppHandle) -> Option<PathBuf> {
    let st = office_state(app)?;
    let dir = st.download_dir.lock().unwrap();
    dir.clone()
}

/// Kullanıcının kayıtlı sunucu (API) adresi; sidecar'dan okunur.
async fn api_base(app: &AppHandle) -> Option<String> {
    let bridge = {
        let state = app.state::<crate::SidecarState>();
        crate::current_bridge(&state).ok()?
    };
    let st = bridge
        .call(
            "agent.status",
            Value::Object(Default::default()),
            Duration::from_secs(5),
        )
        .await
        .ok()?;
    let base = st.get("base_url")?.as_str()?.trim();
    if base.is_empty() {
        return None;
    }
    Some(base.to_string())
}

async fn web_url(app: &AppHandle) -> Option<Url> {
    let api = api_base(app).await;
    resolve_web_url(BUILD_WEB_URL, api.as_deref(), BUILD_API_URL)
}

/// Bağlantıyı sistemde (tarayıcı, e-posta) açar.
fn open_external(app: &AppHandle, url: &Url) {
    #[allow(deprecated)]
    {
        use tauri_plugin_shell::ShellExt;
        if let Err(e) = app.shell().open(url.as_str(), None) {
            log::warn!("bağlantı sistemde açılamadı: {e}");
        }
    }
}

/// Ofis paneli açık mı?
pub fn is_open(app: &AppHandle) -> bool {
    app.get_webview_window(OFFICE_WINDOW).is_some()
}

/// Ofis panelini açar ya da öne getirir.
pub async fn open(app: &AppHandle) -> Result<(), String> {
    if let Some(w) = app.get_webview_window(OFFICE_WINDOW) {
        let _ = w.unminimize();
        let _ = w.show();
        let _ = w.set_focus();
        return Ok(());
    }
    let Some(url) = web_url(app).await else {
        return Err(NO_URL.into());
    };
    log::info!("ofis paneli: {}", url.origin().ascii_serialization());

    let nav_app = app.clone();
    let nav_base = url.clone();
    let win_app = app.clone();
    let dl_app = app.clone();
    let dl_base = url.clone();
    WebviewWindowBuilder::new(app, OFFICE_WINDOW, WebviewUrl::External(url))
        .title("Hakan · Ofis paneli")
        .inner_size(1280.0, 820.0)
        .min_inner_size(900.0, 600.0)
        // Dosya bırakma web sayfasının kendisine kalsın (panelin kendi
        // yükleme alanları); kabuğun sürükle-bırak olayı yalnızca ana
        // pencerede.
        .disable_drag_drop_handler()
        .on_navigation(move |target| match navigation(&nav_base, target) {
            Nav::Allow => true,
            Nav::External => {
                open_external(&nav_app, target);
                false
            }
            Nav::Block => {
                log::warn!("ofis panelinde gezinme engellendi: {}", target.scheme());
                false
            }
        })
        .on_new_window(move |target, _features| {
            if new_window(&target) == Nav::External {
                open_external(&win_app, &target);
            }
            tauri::webview::NewWindowResponse::Deny
        })
        .on_download(move |_webview, event| {
            if let DownloadEvent::Requested { url, destination } = event {
                return download_requested(&dl_app, &dl_base, &url, destination);
            }
            if let DownloadEvent::Finished { url, path, success } = event {
                download_finished(&dl_app, &url, path, success);
            }
            true
        })
        .build()
        .map_err(|e| {
            log::error!("ofis paneli açılamadı: {e}");
            "Ofis paneli açılamadı.".to_string()
        })?;
    Ok(())
}

/// İndirme başlıyor: yabancı origin reddedilir; klasör seçiliyse dosya oraya
/// (aynı ad varsa numaralı) kaydedilir, değilse sistemin indirme klasörüne.
fn download_requested(app: &AppHandle, base: &Url, url: &Url, dest: &mut PathBuf) -> bool {
    if !download_allowed(base, url) {
        let origin = url.origin().ascii_serialization();
        log::warn!("ofis panelinde yabancı indirme reddedildi: {origin}");
        return false;
    }
    if let Some(dir) = configured_dir(app).filter(|d| d.is_dir()) {
        let suggested = os_name(dest.file_name());
        *dest = unique_path(&dir, &safe_file_name(&suggested));
    }
    if let Some(st) = office_state(app) {
        let mut pending = st.pending.lock().unwrap();
        pending.insert(url.to_string(), dest.clone());
    }
    true
}

#[derive(Clone, Serialize)]
struct DownloadDone {
    name: String,
    folder: String,
    ok: bool,
}

fn download_finished(app: &AppHandle, url: &Url, path: Option<PathBuf>, success: bool) {
    let planned = office_state(app).and_then(|st| {
        let mut pending = st.pending.lock().unwrap();
        pending.remove(url.as_str())
    });
    if let Some(path) = path.or(planned) {
        let done = DownloadDone {
            name: os_name(path.file_name()),
            folder: os_name(path.parent().and_then(Path::file_name)),
            ok: success,
        };
        let _ = app.emit_to(crate::MAIN_WINDOW, DOWNLOAD_EVENT, done);
    }
}

/// Sidecar `upload` olayı: yeni belge yüklendiyse bildirimi zamanlar.
pub fn on_upload(app: &AppHandle, data: &Value) {
    if data.get("outcome").and_then(Value::as_str) != Some("uploaded") {
        return;
    }
    if let Some(st) = office_state(app) {
        st.new_docs.lock().unwrap().add();
        schedule_flush(app, NOTIFY_DEBOUNCE);
    }
}

fn schedule_flush(app: &AppHandle, delay: Duration) {
    if let Some(st) = office_state(app) {
        if st.flush_scheduled.swap(true, Ordering::SeqCst) {
            return;
        }
    }
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        tokio::time::sleep(delay).await;
        let take = match office_state(&app) {
            Some(st) => {
                st.flush_scheduled.store(false, Ordering::SeqCst);
                let mut docs = st.new_docs.lock().unwrap();
                docs.take(Instant::now())
            }
            None => Take::Nothing,
        };
        match take {
            Take::Notify(n) => notify_new_documents(&app, n),
            Take::Wait(d) => schedule_flush(&app, d),
            Take::Nothing => {}
        }
    });
}

/// Tepsi bildirimi; ofis paneli açıksa ana pencereye yenileme sinyali.
fn notify_new_documents(app: &AppHandle, count: u32) {
    use tauri_plugin_notification::NotificationExt;
    let open = is_open(app);
    let (title, body) = new_docs_text(count, open);
    let n = app.notification().builder().title(title).body(body);
    if let Err(e) = n.show() {
        log::warn!("bildirim gösterilemedi: {e}");
    }
    if open {
        let payload = serde_json::json!({ "count": count });
        let _ = app.emit_to(crate::MAIN_WINDOW, NEW_DOCUMENTS_EVENT, payload);
    }
}

/// Ön yüzün gördüğü durum.
#[derive(Serialize)]
pub struct OfficeStatus {
    /// Panelin adresi (yoksa `null`: sunucu adresi bilinmiyor).
    web_url: Option<String>,
    open: bool,
    /// İndirilen Luca dosyalarının klasörü (yoksa sistemin indirme klasörü).
    download_dir: Option<String>,
}

async fn status(app: &AppHandle) -> OfficeStatus {
    let dir = configured_dir(app);
    OfficeStatus {
        web_url: web_url(app).await.map(|u| u.to_string()),
        open: is_open(app),
        download_dir: dir.map(|d| d.to_string_lossy().into_owned()),
    }
}

fn set_download_dir(app: &AppHandle, dir: Option<PathBuf>) -> Result<(), String> {
    let cfg = app.path().app_config_dir().map_err(|e| e.to_string())?;
    let settings = Settings {
        download_dir: dir.clone(),
    };
    if let Err(e) = save_settings(&cfg, &settings) {
        log::error!("ofis ayarı kaydedilemedi: {e}");
        return Err("Ayar kaydedilemedi.".into());
    }
    if let Some(st) = office_state(app) {
        *st.download_dir.lock().unwrap() = dir;
    }
    Ok(())
}

#[tauri::command]
pub async fn office_status(app: AppHandle) -> Result<OfficeStatus, String> {
    Ok(status(&app).await)
}

#[tauri::command]
pub async fn office_open(app: AppHandle) -> Result<OfficeStatus, String> {
    open(&app).await?;
    Ok(status(&app).await)
}

/// Açık paneli yeniler ve öne getirir; kapalıysa açar.
#[tauri::command]
pub async fn office_reload(app: AppHandle) -> Result<OfficeStatus, String> {
    if let Some(w) = app.get_webview_window(OFFICE_WINDOW) {
        if let Err(e) = w.eval("window.location.reload()") {
            log::warn!("ofis paneli yenilenemedi: {e}");
        }
    }
    open(&app).await?;
    Ok(status(&app).await)
}

/// İndirme klasörünü işletim sisteminin seçicisiyle seçer (iptal: değişmez).
#[tauri::command]
pub async fn office_pick_download_dir(app: AppHandle) -> Result<OfficeStatus, String> {
    use tauri_plugin_dialog::DialogExt;
    let (tx, rx) = tokio::sync::oneshot::channel();
    let mut dialog = app
        .dialog()
        .file()
        .set_title("Luca dosyalarının kaydedileceği klasörü seçin");
    if let Some(w) = app.get_webview_window(crate::MAIN_WINDOW) {
        dialog = dialog.set_parent(&w);
    }
    dialog.pick_folder(move |p| {
        let _ = tx.send(p);
    });
    let picked = rx.await.map_err(|_| "Klasör seçici kapandı.".to_string())?;
    if let Some(p) = picked.and_then(|p| p.into_path().ok()) {
        set_download_dir(&app, Some(p))?;
    }
    Ok(status(&app).await)
}

/// Seçimi kaldırır: indirmeler sistemin indirme klasörüne gider.
#[tauri::command]
pub async fn office_clear_download_dir(app: AppHandle) -> Result<OfficeStatus, String> {
    set_download_dir(&app, None)?;
    Ok(status(&app).await)
}

#[cfg(test)]
mod tests {
    use super::*;

    fn u(s: &str) -> Url {
        Url::parse(s).unwrap()
    }

    fn web(api: &str) -> String {
        match derive_web_url(api) {
            Some(u) => u.to_string(),
            None => String::new(),
        }
    }

    #[test]
    fn web_adresi_api_den_turetilir() {
        let cases = [
            ("https://api.ornek.com", "https://app.ornek.com/"),
            (" https://API.ornek.com/v1/ ", "https://app.ornek.com/"),
            // api. ile başlamıyorsa panel aynı sunucuda.
            ("https://hakan.ornek.com", "https://hakan.ornek.com/"),
            // Ortadaki "api." değişmez.
            ("https://x.api.ornek.com", "https://x.api.ornek.com/"),
            ("http://localhost:8080", "http://localhost:8080/"),
            // Uzak sunucuda düz http, başka şema ya da boş adres yok.
            ("http://api.ornek.com", ""),
            ("ftp://api.ornek.com", ""),
            ("", ""),
        ];
        for (api, want) in cases {
            assert_eq!(web(api), want, "{api}");
        }
    }

    #[test]
    fn web_adresi_sirasi() {
        let api = Some("https://api.ornek.com");
        let build_api = Some("https://api.derleme.com");
        let r = |w: Option<&str>, a: Option<&str>| match resolve_web_url(w, a, build_api) {
            Some(u) => u.to_string(),
            None => String::new(),
        };
        let panel = Some("https://panel.ornek.com/?x=1#a");
        assert_eq!(r(panel, api), "https://panel.ornek.com/");
        assert_eq!(r(Some("  "), api), "https://app.ornek.com/");
        // Geçersiz derleme adresi atlanır.
        let bad = Some("http://panel.ornek.com");
        assert_eq!(r(bad, api), "https://app.ornek.com/");
        assert_eq!(r(None, None), "https://app.derleme.com/");
        assert!(resolve_web_url(None, None, None).is_none());
    }

    #[test]
    fn gezinme_yalniz_kendi_origininde() {
        let base = u("https://app.ornek.com/");
        let cases = [
            ("https://app.ornek.com/ofis/yevmiye?x=1", Nav::Allow),
            ("https://app.ornek.com:443/giris", Nav::Allow),
            ("blob:https://app.ornek.com/1f2e", Nav::Allow),
            ("about:blank", Nav::Allow),
            // Başka origin: alt alan adı, şema ya da port farkı dahil.
            ("https://api.ornek.com/v1", Nav::External),
            ("http://app.ornek.com/", Nav::External),
            ("https://app.ornek.com:8443/", Nav::External),
            ("https://app.ornek.com.kotu.com/", Nav::External),
            ("https://www.gib.gov.tr/", Nav::External),
            ("mailto:destek@ornek.com", Nav::External),
            // Yerel dosya, betik, veri ve yabancı blob hiç açılmaz.
            ("file:///etc/passwd", Nav::Block),
            ("javascript:alert(1)", Nav::Block),
            ("data:text/html,x", Nav::Block),
            ("blob:https://kotu.com/1f2e", Nav::Block),
            ("tauri://localhost/", Nav::Block),
        ];
        for (target, want) in cases {
            assert_eq!(navigation(&base, &u(target)), want, "{target}");
        }
    }

    #[test]
    fn yeni_pencere_hep_sistemde() {
        let cases = [
            ("https://app.ornek.com/rapor", Nav::External),
            ("https://www.gib.gov.tr/", Nav::External),
            ("file:///tmp/x", Nav::Block),
            ("blob:https://app.ornek.com/1", Nav::Block),
        ];
        for (target, want) in cases {
            assert_eq!(new_window(&u(target)), want, "{target}");
        }
    }

    #[test]
    fn indirme_yalniz_kendi_origininden() {
        let base = u("https://app.ornek.com/");
        let cases = [
            ("https://app.ornek.com/luca/fis.xlsx", true),
            ("blob:https://app.ornek.com/9a8b", true),
            ("https://api.ornek.com/files/x", false),
            ("blob:https://kotu.com/9a8b", false),
            ("data:text/csv,a", false),
            ("file:///tmp/a.xlsx", false),
        ];
        for (url, want) in cases {
            assert_eq!(download_allowed(&base, &u(url)), want, "{url}");
        }
    }

    #[test]
    fn dosya_adi_guvenli_ve_ustune_yazilmaz() {
        assert_eq!(safe_file_name("luca_2026-10.xlsx"), "luca_2026-10.xlsx");
        assert_eq!(safe_file_name("../../etc/passwd"), "passwd");
        assert_eq!(safe_file_name("C:\\x\\fis?.txt"), "fis_.txt");
        assert_eq!(safe_file_name(".gizli"), "gizli");
        assert_eq!(safe_file_name(""), "hakan-indirme");

        let name = format!("hakan-office-test-{}", std::process::id());
        let dir = std::env::temp_dir().join(name);
        std::fs::create_dir_all(&dir).unwrap();
        let first = unique_path(&dir, "fis.xlsx");
        assert_eq!(first, dir.join("fis.xlsx"));
        std::fs::write(&first, b"1").unwrap();
        assert_eq!(unique_path(&dir, "fis.xlsx"), dir.join("fis (2).xlsx"));
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn yeni_belge_bildirimi_toplanir() {
        let t0 = Instant::now();
        let later = t0 + Duration::from_secs(20);
        let mut n = NewDocs::default();
        assert_eq!(n.take(t0), Take::Nothing);
        n.add();
        n.add();
        assert_eq!(n.take(t0), Take::Notify(2));
        n.add();
        let wait = NOTIFY_EVERY - Duration::from_secs(20);
        assert_eq!(n.take(later), Take::Wait(wait));
        n.add();
        assert_eq!(n.take(t0 + NOTIFY_EVERY), Take::Notify(2));
        assert_eq!(new_docs_text(1, false).0, "HAKAN · yeni belge geldi");
        let (title, body) = new_docs_text(3, true);
        assert!(title.contains("3 yeni belge"));
        assert!(body.contains("Paneli yenile"));
    }

    #[test]
    fn komutlar_yalniz_ana_pencereden() {
        assert!(ipc_allowed("main"));
        assert!(!ipc_allowed(OFFICE_WINDOW));
        assert!(!ipc_allowed(""));
    }
}
