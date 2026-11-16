//! Otomatik güncelleme (#205).
//!
//! - İmza açık anahtarı (`TAURI_UPDATER_PUBKEY`) ve manifest kök adresi
//!   (`TAURI_UPDATER_BASE_URL`) **derleme zamanında** ortamdan okunur. Biri
//!   yoksa güncelleyici bu derlemede kapalıdır; ön yüz nedenini gösterir.
//! - Manifest adresi `<base>/<kanal>/latest.json` (bkz.
//!   `scripts/updater-manifest.mjs` `endpointFor`). Kanal `stable` ya da
//!   `beta`; seçim uygulama yapılandırma klasöründe `updater.json`'da tutulur.
//! - Açılışta ve 6 saatte bir denetlenir (`lib.rs`); kurulum ön yüzün
//!   onayıyla `update_install` komutundan yapılır.
//!
//! Bu modülün Tauri'ye bağlı olmayan kısmı birim testli; indirme, imza
//! doğrulama ve kurulum `tests/updater_e2e.rs`'te geçici anahtar çifti ve
//! yerel manifestle uçtan uca denenir.

use std::path::{Path, PathBuf};
use std::time::Duration;

use serde::{Deserialize, Serialize};
use tauri::{Manager, Runtime, Url};
use tauri_plugin_updater::{Updater, UpdaterExt};

/// Arka plan denetim aralığı.
pub const CHECK_INTERVAL: Duration = Duration::from_secs(6 * 60 * 60);

/// Açılıştan sonra ilk denetime kadar beklenen süre (sidecar ve pencere
/// önce açılsın).
pub const FIRST_CHECK_DELAY: Duration = Duration::from_secs(15);

/// Kanal seçiminin yazıldığı dosya (uygulama yapılandırma klasöründe).
const SETTINGS_FILE: &str = "updater.json";

/// Derleme zamanı değerleri. Boş ya da tanımsızsa güncelleyici kapalı.
pub const BUILD_PUBKEY: Option<&str> = option_env!("TAURI_UPDATER_PUBKEY");
pub const BUILD_BASE_URL: Option<&str> = option_env!("TAURI_UPDATER_BASE_URL");

#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize, Deserialize)]
#[serde(rename_all = "lowercase")]
pub enum Channel {
    Stable,
    Beta,
}

impl Channel {
    pub fn as_str(self) -> &'static str {
        match self {
            Channel::Stable => "stable",
            Channel::Beta => "beta",
        }
    }

    pub fn parse(s: &str) -> Option<Self> {
        match s {
            "stable" => Some(Channel::Stable),
            "beta" => Some(Channel::Beta),
            _ => None,
        }
    }

    /// Kullanıcı seçim yapmadıysa: beta sürüm (`-beta.N`) beta kanalında,
    /// diğerleri kararlı kanalda kalır.
    pub fn default_for(version: &str) -> Self {
        if version.contains('-') {
            Channel::Beta
        } else {
            Channel::Stable
        }
    }
}

/// Güncelleyicinin bu derlemede neden kapalı olduğu.
#[derive(Debug, Clone, Copy, PartialEq, Eq, Serialize)]
#[serde(rename_all = "snake_case")]
pub enum Disabled {
    NoPubkey,
    NoBaseUrl,
    InvalidBaseUrl,
    InsecureBaseUrl,
}

impl Disabled {
    /// Ön yüzde gösterilen açıklama.
    pub fn message(self) -> &'static str {
        match self {
            Disabled::NoPubkey => {
                "Bu derlemede otomatik güncelleme kapalı: imza anahtarı tanımlanmamış."
            }
            Disabled::NoBaseUrl => {
                "Bu derlemede otomatik güncelleme kapalı: güncelleme adresi tanımlanmamış."
            }
            Disabled::InvalidBaseUrl => {
                "Bu derlemede otomatik güncelleme kapalı: güncelleme adresi geçersiz."
            }
            Disabled::InsecureBaseUrl => {
                "Bu derlemede otomatik güncelleme kapalı: güncelleme adresi https değil."
            }
        }
    }
}

/// Güncelleyici yapılandırması: açık anahtar ve manifest kökü.
#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Config {
    pub pubkey: String,
    pub base_url: Url,
}

impl Config {
    /// Derleme zamanı değerlerinden yapılandırma. Sürüm derlemesinde
    /// (`release == true`) adres https olmalıdır; yerel denemede http'ye izin
    /// verilir.
    pub fn resolve(
        pubkey: Option<&str>,
        base_url: Option<&str>,
        release: bool,
    ) -> Result<Self, Disabled> {
        let pubkey = pubkey.map(str::trim).unwrap_or("");
        if pubkey.is_empty() {
            return Err(Disabled::NoPubkey);
        }
        let base = base_url.map(str::trim).unwrap_or("");
        if base.is_empty() {
            return Err(Disabled::NoBaseUrl);
        }
        let url = Url::parse(base).map_err(|_| Disabled::InvalidBaseUrl)?;
        match url.scheme() {
            "https" => {}
            "http" if !release => {}
            "http" => return Err(Disabled::InsecureBaseUrl),
            _ => return Err(Disabled::InvalidBaseUrl),
        }
        Ok(Self {
            pubkey: pubkey.to_string(),
            base_url: url,
        })
    }

    /// Bu ikilinin derlendiği ortamın değerleri.
    pub fn from_build() -> Result<Self, Disabled> {
        Self::resolve(BUILD_PUBKEY, BUILD_BASE_URL, !cfg!(debug_assertions))
    }

    /// Kanalın manifest adresi: `<base>/<kanal>/latest.json`.
    pub fn endpoint(&self, channel: Channel) -> Url {
        endpoint(&self.base_url, channel)
    }
}

pub fn endpoint(base: &Url, channel: Channel) -> Url {
    let root = base.as_str().trim_end_matches('/');
    Url::parse(&format!("{root}/{}/latest.json", channel.as_str()))
        .expect("geçerli kökten türetilen adres geçerlidir")
}

#[derive(Debug, Default, Serialize, Deserialize)]
struct SettingsFile {
    #[serde(default)]
    channel: Option<Channel>,
}

/// Kayıtlı kanal; dosya yoksa ya da bozuksa sürüme göre varsayılan.
pub fn load_channel(dir: &Path, version: &str) -> Channel {
    std::fs::read(dir.join(SETTINGS_FILE))
        .ok()
        .and_then(|b| serde_json::from_slice::<SettingsFile>(&b).ok())
        .and_then(|s| s.channel)
        .unwrap_or_else(|| Channel::default_for(version))
}

pub fn save_channel(dir: &Path, channel: Channel) -> std::io::Result<()> {
    std::fs::create_dir_all(dir)?;
    let body = serde_json::to_vec_pretty(&SettingsFile {
        channel: Some(channel),
    })?;
    let tmp = dir.join(format!("{SETTINGS_FILE}.tmp"));
    std::fs::write(&tmp, body)?;
    std::fs::rename(tmp, dir.join(SETTINGS_FILE))
}

/// Ön yüze giden güncelleme durumu (`updater://status` olayı ve
/// `update_check` yanıtı).
#[derive(Debug, Clone, Serialize, PartialEq)]
pub struct Status {
    pub enabled: bool,
    /// Kapalıysa ön yüzde gösterilecek açıklama.
    pub disabled_reason: Option<String>,
    pub current: String,
    pub channel: Channel,
    /// idle | checking | up_to_date | available | downloading | installing | error | disabled
    pub state: &'static str,
    pub available: Option<String>,
    pub notes: Option<String>,
    /// Son başarılı denetim (RFC 3339, UTC).
    pub checked_at: Option<String>,
    /// Son hata (kullanıcıya gösterilecek Türkçe metin).
    pub error: Option<String>,
}

impl Status {
    pub fn initial(current: String, channel: Channel, cfg: &Result<Config, Disabled>) -> Self {
        let (enabled, reason, state) = match cfg {
            Ok(_) => (true, None, "idle"),
            Err(d) => (false, Some(d.message().to_string()), "disabled"),
        };
        Self {
            enabled,
            disabled_reason: reason,
            current,
            channel,
            state,
            available: None,
            notes: None,
            checked_at: None,
            error: None,
        }
    }
}

/// İndirme ilerlemesi (`updater://progress`).
#[derive(Debug, Clone, Copy, Default, Serialize, PartialEq, Eq)]
pub struct Progress {
    pub downloaded: u64,
    pub total: Option<u64>,
}

impl Progress {
    pub fn add(&mut self, chunk: usize, total: Option<u64>) {
        self.downloaded += chunk as u64;
        if total.is_some() {
            self.total = total;
        }
    }
}

/// Belirli bir kanal için güncelleyici. `executable` yalnızca testte verilir
/// (kurulumun değiştireceği dosya); uygulamada çalışan ikili kullanılır.
pub fn updater_for<R: Runtime, M: Manager<R>>(
    app: &M,
    cfg: &Config,
    channel: Channel,
    executable: Option<&PathBuf>,
) -> Result<Updater, String> {
    let mut b = app
        .updater_builder()
        .pubkey(cfg.pubkey.clone())
        .endpoints(vec![cfg.endpoint(channel)])
        .map_err(|e| e.to_string())?
        .timeout(Duration::from_secs(60));
    if let Some(p) = executable {
        b = b.executable_path(p);
    }
    b.build().map_err(|e| e.to_string())
}

/// Güncelleyici hatasını kullanıcıya gösterilecek metne çevirir. Ayrıntı
/// günlüğe yazılır.
pub fn user_error(e: &str) -> String {
    let lower = e.to_lowercase();
    if lower.contains("signature") {
        "Güncelleme paketinin imzası doğrulanamadı; kurulmadı.".into()
    } else if lower.contains("timed out")
        || lower.contains("connect")
        || lower.contains("dns")
        || lower.contains("network")
        || lower.contains("request")
    {
        "Güncelleme sunucusuna ulaşılamadı. İnternet bağlantınızı denetleyin.".into()
    } else {
        "Güncelleme denetlenemedi. Daha sonra yeniden denenecek.".into()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn disabled_without_pubkey_or_base() {
        assert_eq!(
            Config::resolve(None, Some("https://x"), true),
            Err(Disabled::NoPubkey)
        );
        assert_eq!(
            Config::resolve(Some("  "), Some("https://x"), true),
            Err(Disabled::NoPubkey)
        );
        assert_eq!(
            Config::resolve(Some("k"), None, true),
            Err(Disabled::NoBaseUrl)
        );
        assert_eq!(
            Config::resolve(Some("k"), Some(""), true),
            Err(Disabled::NoBaseUrl)
        );
        assert_eq!(
            Config::resolve(Some("k"), Some("nope"), true),
            Err(Disabled::InvalidBaseUrl)
        );
        assert_eq!(
            Config::resolve(Some("k"), Some("ftp://x/y"), true),
            Err(Disabled::InvalidBaseUrl)
        );
    }

    #[test]
    fn http_only_outside_release() {
        assert_eq!(
            Config::resolve(Some("k"), Some("http://127.0.0.1:9/u"), true),
            Err(Disabled::InsecureBaseUrl)
        );
        assert!(Config::resolve(Some("k"), Some("http://127.0.0.1:9/u"), false).is_ok());
    }

    #[test]
    fn endpoint_matches_manifest_script() {
        // scripts/updater-manifest.mjs endpointFor ile aynı biçim.
        let cfg = Config::resolve(Some("k"), Some("https://cdn.example.com/hakan/"), true).unwrap();
        assert_eq!(
            cfg.endpoint(Channel::Stable).as_str(),
            "https://cdn.example.com/hakan/stable/latest.json"
        );
        assert_eq!(
            cfg.endpoint(Channel::Beta).as_str(),
            "https://cdn.example.com/hakan/beta/latest.json"
        );
        let cfg = Config::resolve(Some("k"), Some("https://cdn.example.com"), true).unwrap();
        assert_eq!(
            cfg.endpoint(Channel::Stable).as_str(),
            "https://cdn.example.com/stable/latest.json"
        );
    }

    #[test]
    fn channel_default_and_persistence() {
        assert_eq!(Channel::default_for("1.2.3"), Channel::Stable);
        assert_eq!(Channel::default_for("1.2.3-beta.1"), Channel::Beta);
        let dir = std::env::temp_dir().join(format!("hakan-upd-{}", std::process::id()));
        let _ = std::fs::remove_dir_all(&dir);
        assert_eq!(load_channel(&dir, "1.0.0"), Channel::Stable);
        save_channel(&dir, Channel::Beta).unwrap();
        assert_eq!(load_channel(&dir, "1.0.0"), Channel::Beta);
        std::fs::write(dir.join(SETTINGS_FILE), b"{bozuk").unwrap();
        assert_eq!(load_channel(&dir, "1.0.0-beta.2"), Channel::Beta);
        let _ = std::fs::remove_dir_all(&dir);
    }

    #[test]
    fn status_reports_disabled_reason() {
        let s = Status::initial("0.1.0".into(), Channel::Stable, &Err(Disabled::NoPubkey));
        assert!(!s.enabled);
        assert_eq!(s.state, "disabled");
        assert!(s.disabled_reason.unwrap().contains("kapalı"));
        let cfg = Config::resolve(Some("k"), Some("https://x"), true);
        let s = Status::initial("0.1.0".into(), Channel::Beta, &cfg);
        assert!(s.enabled);
        assert_eq!(s.state, "idle");
        assert_eq!(
            serde_json::to_value(&s).unwrap()["channel"],
            serde_json::json!("beta")
        );
    }

    #[test]
    fn progress_accumulates() {
        let mut p = Progress::default();
        p.add(10, Some(100));
        p.add(5, None);
        assert_eq!(
            p,
            Progress {
                downloaded: 15,
                total: Some(100)
            }
        );
    }

    #[test]
    fn user_errors_are_turkish_and_raw_free() {
        let m = user_error("the signature verification failed: xyz");
        assert!(m.contains("imza"));
        assert!(!m.contains("xyz"));
        assert!(user_error("error sending request for url").contains("ulaşılamadı"));
        assert!(user_error("weird").contains("denetlenemedi"));
    }
}
