//! Tepsi simgesi ve tepsi menüsü (#325, tuval: `docs/tasarim/tuval/AjanTepsi.dc.html`).
//!
//! Ajan çoğu zaman görünmezdir; tepsi simgesi durumu söyler, menü tek tıkla
//! çözüm sunar. Simgenin beş durumu (tuvaldeki adlarla): **Bağlı**,
//! **Gönderiyor** (kristaller sırayla yanar), **Duraklatıldı**, **Sorun var**
//! (kırmızı rozet), **Güncelleme hazır** (mavi rozet).
//!
//! Durum yalnızca mevcut kaynaklardan okunur: sidecar olayları (`ready`,
//! `upload`, `agent.heartbeat`, `auth.signed_out`, `agent.revoked`), sidecar
//! kapanması, `agent.status` yanıtı, duraklatma ve güncelleyici durumu.
//! Sidecar davranışı ve ön yüz IPC komutları değişmez.
//!
//! Bu dosyanın üst yarısı saf mantıktır (model → simge durumu, menü tarifi,
//! piksel çizimi) ve birim testleriyle denenir; alt yarısı Tauri yapıştırıcısı.

use std::collections::VecDeque;
use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::{Arc, Mutex, OnceLock};
use std::time::{Duration, Instant};

use serde_json::Value;
use time::{Date, OffsetDateTime, UtcOffset};

/// Bir yüklemeden sonra simge bu süre "Gönderiyor" kalır.
pub const SENDING_WINDOW: Duration = Duration::from_secs(8);
/// Sidecar bu süreden uzun kapalı kalırsa "Sorun var" (yeniden başlatma
/// genellikle bundan kısa sürer; kısa kesintide simge yanıp sönmesin).
pub const DOWN_GRACE: Duration = Duration::from_secs(15);
/// "Gönderiyor" canlandırmasında bir karenin süresi.
pub const FRAME: Duration = Duration::from_millis(400);
/// Durum yoklaması (son tarama metni ve bağlantı hatası güncel kalsın).
pub const POLL_INTERVAL: Duration = Duration::from_secs(60);
/// Menüdeki son dosya ve sorunlu dosya sayısı.
pub const LIST_MAX: usize = 3;
/// Simge kenarı (piksel). İşletim sistemi tepsi boyutuna ölçekler.
pub const ICON_SIZE: u32 = 32;

/// Tepsi simgesinin durumu.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub enum IconState {
    /// Bağlı: her şey yolunda.
    Connected,
    /// Gönderiyor: kristaller sırayla yanar.
    Sending,
    /// Duraklatıldı: soluk simge.
    Paused,
    /// Sorun var: kırmızı rozet.
    Problem,
    /// Güncelleme hazır: mavi rozet.
    UpdateReady,
}

impl IconState {
    /// Tuvaldeki ad (ipucu metni ve günlük için).
    pub fn label(self) -> &'static str {
        match self {
            IconState::Connected => "Bağlı",
            IconState::Sending => "Gönderiyor",
            IconState::Paused => "Duraklatıldı",
            IconState::Problem => "Sorun var",
            IconState::UpdateReady => "Güncelleme hazır",
        }
    }
}

/// Menüde gösterilen son gönderim.
#[derive(Clone, Debug, PartialEq, Eq)]
pub struct Recent {
    pub name: String,
    pub taxpayer: String,
}

/// Tepsinin bildiği her şey. Yalnızca kabuk içinde tutulur.
#[derive(Debug)]
pub struct Model {
    pub version: String,
    /// İzleme kullanıcı tarafından duraklatıldı.
    pub paused: bool,
    /// Süreli duraklatmada kendiliğinden sürdürme zamanı (yerel saat).
    pub resume_at: Option<OffsetDateTime>,
    /// Sidecar hazır mı (`ready` geldi, kapanmadı).
    pub alive: bool,
    /// Sidecar ne zamandır kapalı (duraklatılmadan).
    pub down_since: Option<Instant>,
    /// Son `agent.status` yanıtı.
    pub status: Option<Value>,
    /// Son başarılı yükleme olayı.
    pub last_send: Option<Instant>,
    /// Bugün (yerel gün) gönderilen dosya sayısı; uygulama açıkken sayılır.
    pub today: Option<(Date, u32)>,
    /// En yeni başta.
    pub recent: VecDeque<Recent>,
    /// Kurulmaya hazır sürüm (`updater://status` → `available`).
    pub update: Option<String>,
    /// Güncelleyici "güncel" dedi mi (sürüm satırında "· güncel").
    pub up_to_date: bool,
    pub autostart: bool,
}

impl Model {
    pub fn new(version: impl Into<String>, now: Instant) -> Self {
        Self {
            version: version.into(),
            paused: false,
            resume_at: None,
            alive: false,
            down_since: Some(now),
            status: None,
            last_send: None,
            today: None,
            recent: VecDeque::new(),
            update: None,
            up_to_date: false,
            autostart: false,
        }
    }

    /// Sidecar olayını modele işler. Durumun yeniden okunması gerekiyorsa
    /// `true` döner.
    pub fn on_event(&mut self, event: &str, data: &Value, now: Instant, today: Date) -> bool {
        match event {
            "ready" => {
                self.alive = true;
                self.down_since = None;
                true
            }
            "upload" => {
                let outcome = data.get("outcome").and_then(Value::as_str).unwrap_or("");
                if outcome == "uploaded" {
                    self.last_send = Some(now);
                    let count = match self.today {
                        Some((d, n)) if d == today => n + 1,
                        _ => 1,
                    };
                    self.today = Some((today, count));
                    let name = str_at(data, "name").to_string();
                    let taxpayer = self.taxpayer_of(str_at(data, "engagement_uuid"));
                    self.recent.retain(|r| r.name != name);
                    self.recent.push_front(Recent { name, taxpayer });
                    self.recent.truncate(LIST_MAX);
                }
                true
            }
            "agent.heartbeat" | "auth.signed_out" | "agent.revoked" => true,
            _ => false,
        }
    }

    /// Sidecar kapandı.
    pub fn on_exit(&mut self, now: Instant) {
        self.alive = false;
        if self.down_since.is_none() {
            self.down_since = Some(now);
        }
    }

    /// Duraklatma değişti. Sürdürülünce kapalı kalma süresi sıfırdan sayılır
    /// (sidecar yeniden başlıyor).
    pub fn set_paused(&mut self, paused: bool, resume_at: Option<OffsetDateTime>, now: Instant) {
        self.paused = paused;
        self.resume_at = if paused { resume_at } else { None };
        if paused {
            self.alive = false;
            self.down_since = None;
        } else if !self.alive {
            self.down_since = Some(now);
        }
    }

    /// Güncelleyici durumunu (`updater::Status` JSON) işler.
    pub fn on_update(&mut self, status: &Value) {
        let state = str_at(status, "state");
        self.update = match state {
            "available" | "downloading" | "installing" => status
                .get("available")
                .and_then(Value::as_str)
                .map(str::to_string),
            _ => None,
        };
        self.up_to_date = state == "up_to_date";
    }

    fn taxpayer_of(&self, engagement: &str) -> String {
        watches(self.status.as_ref())
            .find(|w| str_at(w, "engagement_uuid") == engagement && !engagement.is_empty())
            .map(|w| str_at(w, "taxpayer_title").to_string())
            .unwrap_or_default()
    }

    fn sending(&self, now: Instant) -> bool {
        self.last_send
            .is_some_and(|t| now.saturating_duration_since(t) < SENDING_WINDOW)
    }

    fn down_too_long(&self, now: Instant) -> bool {
        !self.alive
            && !self.paused
            && self
                .down_since
                .is_some_and(|t| now.saturating_duration_since(t) >= DOWN_GRACE)
    }

    /// Kullanıcının bakması gereken sorun: başlık ve açıklama.
    pub fn problem(&self, now: Instant) -> Option<(String, String)> {
        if self.paused {
            return None;
        }
        if self.down_too_long(now) {
            return Some((
                "Yerel ajan çalışmıyor".into(),
                "Yeniden başlatılıyor. Dosyalar bilgisayarda bekliyor.".into(),
            ));
        }
        let st = self.status.as_ref().filter(|_| self.alive)?;
        if !st
            .get("signed_in")
            .and_then(Value::as_bool)
            .unwrap_or(false)
        {
            return Some(("Oturum açılmadı".into(), "HAKAN'ı açıp giriş yapın.".into()));
        }
        if matches!(st.get("organization"), None | Some(Value::Null)) {
            return Some((
                "Kurulum tamamlanmadı".into(),
                "HAKAN'ı açıp ofisi seçin.".into(),
            ));
        }
        if !str_at(st, "heartbeat_error").is_empty() {
            return Some((
                "Sunucuya ulaşılamıyor".into(),
                "Dosyalar bilgisayarda bekliyor, kaybolmaz.".into(),
            ));
        }
        let folders = watches(Some(st))
            .filter(|w| !str_at(&w["activity"], "folder_error").is_empty())
            .count();
        if folders > 0 {
            return Some((
                format!("{folders} klasöre erişilemiyor"),
                "Klasör taşınmış ya da silinmiş olabilir.".into(),
            ));
        }
        let files = self.problem_files();
        if !files.is_empty() {
            let retry = files.iter().filter(|f| f.2).count();
            let detail = if retry > 0 {
                format!("{retry} tanesi sonraki taramada yeniden denenecek")
            } else {
                "Bu dosyalar gönderilmeyecek.".to_string()
            };
            return Some((format!("{} dosya gönderilemedi", files.len()), detail));
        }
        None
    }

    /// Sorunlu dosyalar: (ad, Türkçe neden, yeniden denenecek mi).
    pub fn problem_files(&self) -> Vec<(String, String, bool)> {
        watches(self.status.as_ref())
            .flat_map(|w| {
                w["activity"]["errors"]
                    .as_array()
                    .cloned()
                    .unwrap_or_default()
            })
            .map(|e| {
                (
                    str_at(&e, "name").to_string(),
                    str_at(&e, "message").to_string(),
                    e.get("retry").and_then(Value::as_bool).unwrap_or(false),
                )
            })
            .collect()
    }

    /// Simgenin durumu. Öncelik: duraklatıldı > sorun > gönderiyor >
    /// güncelleme > bağlı.
    pub fn icon_state(&self, now: Instant) -> IconState {
        if self.paused {
            IconState::Paused
        } else if self.problem(now).is_some() {
            IconState::Problem
        } else if self.sending(now) {
            IconState::Sending
        } else if self.update.is_some() {
            IconState::UpdateReady
        } else {
            IconState::Connected
        }
    }

    /// Kurulum bitti mi (oturum ve ofis var).
    fn configured(&self) -> bool {
        self.status.as_ref().is_some_and(|st| {
            st.get("signed_in")
                .and_then(Value::as_bool)
                .unwrap_or(false)
                && !matches!(st.get("organization"), None | Some(Value::Null))
        })
    }

    /// Ofis · bilgisayar satırı (tuval: "Demir Mali Müşavirlik · Ofis-PC").
    fn org_line(&self) -> Option<String> {
        let st = self.status.as_ref()?;
        let org = st["organization"]["name"].as_str().unwrap_or("");
        let agent = str_at(st, "agent_name");
        let parts: Vec<&str> = [org, agent].into_iter().filter(|s| !s.is_empty()).collect();
        (!parts.is_empty()).then(|| parts.join(" · "))
    }

    /// Menünün tarifi.
    pub fn menu(&self, now: Instant, wall: OffsetDateTime, today: Date) -> Vec<Entry> {
        let mut m = Vec::new();
        let state = self.icon_state(now);
        let problem = self.problem(now);

        // Durum satırı.
        let (head, detail) = match (&problem, state) {
            (Some((h, d)), _) => (h.clone(), Some(d.clone())),
            (None, IconState::Paused) => (
                "İzleme duraklatıldı".to_string(),
                Some(match self.resume_at {
                    Some(t) => format!("Kendiliğinden sürer: {}", hhmm(t)),
                    None => "Dosyalar gönderilmiyor.".to_string(),
                }),
            ),
            (None, IconState::Sending) => ("Gönderiliyor…".to_string(), None),
            (None, _) if !self.alive => ("Başlatılıyor…".to_string(), None),
            (None, _) => ("Her şey yolunda".to_string(), None),
        };
        m.push(Entry::info(head));
        if let Some(d) = detail {
            m.push(Entry::info(d));
        }
        if let Some(org) = self.org_line() {
            m.push(Entry::info(org));
        }

        // Üç sayaç.
        if self.configured() {
            m.push(Entry::Separator);
            let sent = match self.today {
                Some((d, n)) if d == today => n,
                _ => 0,
            };
            m.push(Entry::info(format!("{sent} bugün gönderildi")));
            let folders = watches(self.status.as_ref()).count();
            m.push(Entry::info(format!("{folders} klasör izleniyor")));
            let scan = self
                .status
                .as_ref()
                .and_then(|st| st["stats"]["last_scan"].as_str())
                .and_then(|s| {
                    OffsetDateTime::parse(s, &time::format_description::well_known::Rfc3339).ok()
                });
            m.push(Entry::info(match scan {
                Some(t) => format!("Son tarama: {}", ago(wall - t)),
                None => "Henüz tarama yapılmadı".to_string(),
            }));
        }

        // Sorunlu dosyalar (Windows sorun menüsü) ya da son gönderilenler.
        let files = self.problem_files();
        if state == IconState::Problem && !files.is_empty() {
            m.push(Entry::Separator);
            for (i, (name, why, _)) in files.iter().take(LIST_MAX).enumerate() {
                m.push(Entry::action(
                    format!("problem.{i}"),
                    format!("{} — {}", short(name, 40), short(why, 60)),
                ));
            }
            if files.iter().any(|f| f.2) {
                m.push(Entry::action("retry", "Şimdi yeniden dene"));
            }
            m.push(Entry::action("details", "Ayrıntılar"));
        } else if !self.recent.is_empty() {
            m.push(Entry::Separator);
            for (i, r) in self.recent.iter().enumerate() {
                let text = if r.taxpayer.is_empty() {
                    short(&r.name, 40)
                } else {
                    format!("{} · {}", short(&r.name, 40), short(&r.taxpayer, 40))
                };
                m.push(Entry::action(format!("recent.{i}"), text));
            }
        }

        // Eylemler.
        m.push(Entry::Separator);
        m.push(Entry::Action {
            id: "open".into(),
            text: "HAKAN'ı aç".into(),
            enabled: true,
            accel: Some("CmdOrCtrl+O"),
        });
        let can_scan = self.alive && !self.paused && watches(self.status.as_ref()).count() > 0;
        m.push(Entry::Action {
            id: "scan".into(),
            text: "Şimdi tara".into(),
            enabled: can_scan,
            accel: None,
        });
        if self.paused {
            m.push(Entry::action("resume", "İzlemeyi sürdür"));
        } else {
            m.push(Entry::Submenu {
                text: "İzlemeyi duraklat".into(),
                items: vec![
                    Entry::action("pause.30m", "30 dakika"),
                    Entry::action("pause.2h", "2 saat"),
                    Entry::action("pause.manual", "Ben sürdürene kadar"),
                ],
            });
        }
        let has_panel = self
            .status
            .as_ref()
            .is_some_and(|st| !str_at(st, "base_url").is_empty());
        m.push(Entry::Action {
            id: "panel".into(),
            text: "Panelde aç ↗".into(),
            enabled: has_panel,
            accel: None,
        });
        m.push(Entry::action("settings", "Ayarlar"));
        m.push(Entry::Separator);
        m.push(Entry::Check {
            id: "autostart".into(),
            text: "Oturum açılışında başlat".into(),
            checked: self.autostart,
        });

        // Sürüm ve çıkış.
        m.push(Entry::Separator);
        match &self.update {
            Some(v) => m.push(Entry::action(
                "update",
                format!("Güncelle ve yeniden başlat · {v}"),
            )),
            None if self.up_to_date => {
                m.push(Entry::info(format!("Sürüm {} · güncel", self.version)))
            }
            None => m.push(Entry::info(format!("Sürüm {}", self.version))),
        }
        m.push(Entry::action("quit", "Çıkış"));
        m
    }

    /// Fare üstündeki ipucu.
    pub fn tooltip(&self, now: Instant) -> String {
        match self.problem(now) {
            Some((h, _)) => format!("HAKAN · {h}"),
            None => format!("HAKAN · {}", self.icon_state(now).label()),
        }
    }
}

/// Menü tarifinin bir satırı. Tarif değişmedikçe menü yeniden kurulmaz.
#[derive(Clone, Debug, PartialEq, Eq)]
pub enum Entry {
    /// Tıklanmayan bilgi satırı.
    Info(String),
    Action {
        id: String,
        text: String,
        enabled: bool,
        accel: Option<&'static str>,
    },
    Check {
        id: String,
        text: String,
        checked: bool,
    },
    Submenu {
        text: String,
        items: Vec<Entry>,
    },
    Separator,
}

impl Entry {
    fn info(text: impl Into<String>) -> Self {
        Entry::Info(text.into())
    }
    fn action(id: impl Into<String>, text: impl Into<String>) -> Self {
        Entry::Action {
            id: id.into(),
            text: text.into(),
            enabled: true,
            accel: None,
        }
    }
}

fn str_at<'a>(v: &'a Value, key: &str) -> &'a str {
    v.get(key).and_then(Value::as_str).unwrap_or("")
}

fn watches(status: Option<&Value>) -> impl Iterator<Item = &Value> {
    status
        .and_then(|st| st.get("watches"))
        .and_then(Value::as_array)
        .into_iter()
        .flatten()
}

fn hhmm(t: OffsetDateTime) -> String {
    format!("{:02}:{:02}", t.hour(), t.minute())
}

/// "az önce", "12 dk önce", "3 sa önce", "2 gün önce".
pub fn ago(d: time::Duration) -> String {
    let mins = d.whole_minutes().max(0);
    match mins {
        0 => "az önce".into(),
        1..=59 => format!("{mins} dk önce"),
        60..=1439 => format!("{} sa önce", mins / 60),
        _ => format!("{} gün önce", mins / 1440),
    }
}

/// Uzun dosya ve mükellef adlarını menü için kısaltır.
fn short(s: &str, max: usize) -> String {
    if s.chars().count() <= max {
        return s.to_string();
    }
    let mut out: String = s.chars().take(max.saturating_sub(1)).collect();
    out.push('…');
    out
}

// --- Simge çizimi -------------------------------------------------------------

/// Simgenin nasıl boyanacağı.
#[derive(Clone, Copy, Debug, PartialEq, Eq, Hash)]
pub struct Style {
    /// macOS şablon simgesi: tek renk siyah; sistem menü çubuğuna göre boyar.
    pub template: bool,
    /// Koyu tema (şablon değilse taç açık renk çizilir).
    pub dark: bool,
}

const RED: [u8; 3] = [0xE5, 0x48, 0x4D];
const BLUE: [u8; 3] = [0x24, 0x48, 0xFF];
/// Açık temada lacivert (M3 marka `#102290`), koyu temada açık kristal.
const GLYPH_LIGHT: [u8; 3] = [0x10, 0x22, 0x90];
const GLYPH_DARK: [u8; 3] = [0xE6, 0xEA, 0xFF];

/// M3 logosunun üç kristalli taç türevi (tuvaldeki `.tray`: 4 px genişlik,
/// 1,5 px aralık, 10/14/10 px yükseklik, 2 px sivri tepe; 16 px'lik ölçünün
/// iki katı). Dönen dizi `ICON_SIZE × ICON_SIZE` RGBA'dır.
///
/// `frame` yalnızca "Gönderiyor"da anlamlıdır: o karede yanan kristal.
pub fn render(state: IconState, frame: usize, style: Style) -> Vec<u8> {
    const SS: usize = 4; // kenar başına alt örnek (kenar yumuşatma)
    let n = ICON_SIZE as usize;
    let glyph = if style.template {
        [0, 0, 0]
    } else if style.dark {
        GLYPH_DARK
    } else {
        GLYPH_LIGHT
    };
    let badge = match state {
        IconState::Problem => Some(if style.template { [0, 0, 0] } else { RED }),
        IconState::UpdateReady => Some(if style.template { [0, 0, 0] } else { BLUE }),
        _ => None,
    };
    // Kristaller: (sol x, tepe y); genişlik 8, sivri 4, taban 30.
    const BARS: [(f32, f32); 3] = [(1.0, 10.0), (12.0, 2.0), (23.0, 10.0)];
    const W: f32 = 8.0;
    const POINT: f32 = 4.0;
    const BOTTOM: f32 = 30.0;
    // Rozet: sağ üstte; çevresindeki halka taçtan oyulur ki seçilsin.
    const BC: (f32, f32) = (26.0, 6.0);
    const BR: f32 = 5.0;
    const RING: f32 = 7.0;

    let lit = |i: usize| -> f32 {
        match state {
            IconState::Paused => 0.4,
            IconState::Sending if i != frame % 3 => 0.35,
            _ => 1.0,
        }
    };

    let mut out = vec![0u8; n * n * 4];
    for py in 0..n {
        for px in 0..n {
            let (mut g, mut b) = (0.0f32, 0.0f32);
            for sy in 0..SS {
                for sx in 0..SS {
                    let x = px as f32 + (sx as f32 + 0.5) / SS as f32;
                    let y = py as f32 + (sy as f32 + 0.5) / SS as f32;
                    let d = ((x - BC.0).powi(2) + (y - BC.1).powi(2)).sqrt();
                    if badge.is_some() && d <= BR {
                        b += 1.0;
                        continue;
                    }
                    if badge.is_some() && d <= RING {
                        continue;
                    }
                    for (i, (x0, top)) in BARS.iter().enumerate() {
                        if x < *x0 || x > x0 + W || y > BOTTOM {
                            continue;
                        }
                        let cx = x0 + W / 2.0;
                        if y >= top + POINT * (x - cx).abs() / (W / 2.0) {
                            g += lit(i);
                            break;
                        }
                    }
                }
            }
            let total = (SS * SS) as f32;
            let (g, b) = (g / total, b / total);
            let i = (py * n + px) * 4;
            let (rgb, a) = match badge {
                Some(c) if b > 0.0 => (c, b + g * (1.0 - b)),
                _ => (glyph, g),
            };
            out[i..i + 3].copy_from_slice(&rgb);
            out[i + 3] = (a.clamp(0.0, 1.0) * 255.0).round() as u8;
        }
    }
    out
}

// --- Tauri yapıştırıcısı ---------------------------------------------------------

use tauri::image::Image;
use tauri::menu::{
    CheckMenuItem, IsMenuItem, Menu, MenuEvent, MenuItem, PredefinedMenuItem, Submenu,
};
use tauri::tray::{MouseButton, MouseButtonState, TrayIcon, TrayIconBuilder, TrayIconEvent};
use tauri::{AppHandle, Emitter, Manager, Wry};
use tauri_plugin_autostart::ManagerExt;

use crate::bridge::Bridge;

/// Ön yüze "şu sekmeyi aç" olayı (yalnızca ekleme; ön yüz dinlemezse
/// pencere yine açılır).
pub const NAVIGATE_EVENT: &str = "shell://navigate";

const TRAY_ID: &str = "main";

/// Yerel saat dilimi. Linux'ta çok iş parçacıklı süreçte okunamadığından
/// `run()` başında bir kez alınır; alınamazsa Türkiye saati (UTC+3).
static LOCAL_OFFSET: OnceLock<UtcOffset> = OnceLock::new();

/// `run()` başında, iş parçacıkları açılmadan çağrılır.
pub fn init_local_offset() {
    let off = UtcOffset::current_local_offset()
        .unwrap_or_else(|_| UtcOffset::from_hms(3, 0, 0).expect("geçerli ofset"));
    let _ = LOCAL_OFFSET.set(off);
}

fn local_now() -> OffsetDateTime {
    let off = LOCAL_OFFSET
        .get()
        .copied()
        .unwrap_or_else(|| UtcOffset::from_hms(3, 0, 0).expect("geçerli ofset"));
    OffsetDateTime::now_utc().to_offset(off)
}

/// Tepsinin paylaşılan durumu (`app.manage`).
pub struct Hub {
    model: Mutex<Model>,
    /// Son kurulan menünün tarifi; aynıysa menü yeniden kurulmaz.
    spec: Mutex<Option<Vec<Entry>>>,
    /// Son çizilen simge; aynıysa yeniden çizilmez.
    icon: Mutex<Option<(IconState, usize, Style)>>,
    animating: AtomicBool,
    frame: AtomicU64,
    /// Süreli duraklatma belirteci: elle sürdürme ya da yeni duraklatma
    /// eski zamanlayıcıyı geçersiz kılar.
    pause_token: AtomicU64,
    /// `pause_for`'un istediği sürdürme zamanı; `on_paused` alır.
    pending_resume: Mutex<Option<OffsetDateTime>>,
    polling: AtomicBool,
}

impl Hub {
    pub fn new(version: String) -> Self {
        Self {
            model: Mutex::new(Model::new(version, Instant::now())),
            spec: Mutex::default(),
            icon: Mutex::default(),
            animating: AtomicBool::new(false),
            frame: AtomicU64::new(0),
            pause_token: AtomicU64::new(0),
            pending_resume: Mutex::default(),
            polling: AtomicBool::new(false),
        }
    }
}

fn hub(app: &AppHandle) -> Option<tauri::State<'_, Hub>> {
    app.try_state::<Hub>()
}

fn style(app: &AppHandle) -> Style {
    let dark = app
        .get_webview_window(super::MAIN_WINDOW)
        .and_then(|w| w.theme().ok())
        .is_some_and(|t| t == tauri::Theme::Dark);
    Style {
        template: cfg!(target_os = "macos"),
        dark,
    }
}

/// Tepsi simgesini kurar. Menü ve simge ilk durumdan üretilir.
pub fn build(app: &AppHandle) -> tauri::Result<()> {
    let version = app.package_info().version.to_string();
    app.manage(Hub::new(version));
    if let Some(h) = hub(app) {
        h.model.lock().unwrap().autostart = app.autolaunch().is_enabled().unwrap_or(false);
    }
    let st = style(app);
    let icon = Image::new_owned(render(IconState::Connected, 0, st), ICON_SIZE, ICON_SIZE);
    TrayIconBuilder::with_id(TRAY_ID)
        .icon(icon)
        .icon_as_template(st.template)
        .tooltip("HAKAN")
        .show_menu_on_left_click(false)
        .on_menu_event(on_menu)
        .on_tray_icon_event(|tray, event| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = event
            {
                super::show_main(tray.app_handle());
            }
        })
        .build(app)?;
    refresh(app);
    spawn_poll_loop(app.clone());
    Ok(())
}

fn tray(app: &AppHandle) -> Option<TrayIcon<Wry>> {
    app.tray_by_id(TRAY_ID)
}

/// Modeli simgeye, ipucuna ve menüye yansıtır. Kilitler tutulurken Tauri
/// çağrısı yapılmaz (ana iş parçacığına geçen çağrılar kilitlenmesin).
pub fn refresh(app: &AppHandle) {
    let Some(h) = hub(app) else { return };
    let Some(tray) = tray(app) else { return };
    let now = Instant::now();
    let wall = local_now();
    let (state, spec, tip) = {
        let m = h.model.lock().unwrap();
        (
            m.icon_state(now),
            m.menu(now, wall, wall.date()),
            m.tooltip(now),
        )
    };
    let st = style(app);
    let frame = h.frame.load(Ordering::SeqCst) as usize % 3;
    set_icon(&h, &tray, state, frame, st);
    let _ = tray.set_tooltip(Some(&tip));

    let changed = {
        let mut cur = h.spec.lock().unwrap();
        if cur.as_ref() == Some(&spec) {
            false
        } else {
            *cur = Some(spec.clone());
            true
        }
    };
    if changed {
        match build_menu(app, &spec) {
            Ok(menu) => {
                let _ = tray.set_menu(Some(menu));
            }
            Err(e) => log::error!("tepsi menüsü kurulamadı: {e}"),
        }
    }
    if state == IconState::Sending && !h.animating.swap(true, Ordering::SeqCst) {
        spawn_animation(app.clone());
    }
}

fn set_icon(h: &Hub, tray: &TrayIcon<Wry>, state: IconState, frame: usize, st: Style) {
    let key = (
        state,
        if state == IconState::Sending {
            frame
        } else {
            0
        },
        st,
    );
    {
        let mut cur = h.icon.lock().unwrap();
        if *cur == Some(key) {
            return;
        }
        *cur = Some(key);
    }
    let img = Image::new_owned(render(state, key.1, st), ICON_SIZE, ICON_SIZE);
    let _ = tray.set_icon(Some(img));
    let _ = tray.set_icon_as_template(st.template);
}

/// "Gönderiyor" sürdükçe kristalleri sırayla yakar; bitince durur ve son
/// durumu çizer.
fn spawn_animation(app: AppHandle) {
    tauri::async_runtime::spawn(async move {
        loop {
            tokio::time::sleep(FRAME).await;
            let Some(h) = hub(&app) else { return };
            h.frame.fetch_add(1, Ordering::SeqCst);
            let sending = h.model.lock().unwrap().icon_state(Instant::now()) == IconState::Sending;
            if !sending {
                h.animating.store(false, Ordering::SeqCst);
                refresh(&app);
                return;
            }
            refresh(&app);
        }
    });
}

/// Dakikada bir durumu okur (son tarama, bağlantı hatası, sidecar kapalı
/// kalma süresi menüde güncel kalsın).
fn spawn_poll_loop(app: AppHandle) {
    tauri::async_runtime::spawn(async move {
        loop {
            tokio::time::sleep(POLL_INTERVAL).await;
            poll_status(&app);
            refresh(&app);
        }
    });
}

/// `agent.status`'u kabuk içinden okur (ön yüz izin listesinden bağımsız;
/// salt okunur).
pub fn poll_status(app: &AppHandle) {
    let Ok(bridge) = super::current_bridge(&app.state::<super::SidecarState>()) else {
        return;
    };
    let Some(h) = hub(app) else { return };
    if h.polling.swap(true, Ordering::SeqCst) {
        return;
    }
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let res = bridge
            .call(
                "agent.status",
                Value::Object(Default::default()),
                Duration::from_secs(10),
            )
            .await;
        if let Some(h) = hub(&app) {
            h.polling.store(false, Ordering::SeqCst);
            if let Ok(st) = res {
                h.model.lock().unwrap().status = Some(st);
            }
        }
        refresh(&app);
    });
}

/// Sidecar olayı (start_sidecar döngüsünden; ön yüze iletilmeden önce de
/// sonra da aynı veri).
pub fn on_sidecar_event(app: &AppHandle, ev: &crate::bridge::Event) {
    let Some(h) = hub(app) else { return };
    let wall = local_now();
    let poll = h
        .model
        .lock()
        .unwrap()
        .on_event(&ev.event, &ev.data, Instant::now(), wall.date());
    if poll {
        poll_status(app);
    }
    refresh(app);
}

pub fn on_sidecar_exit(app: &AppHandle) {
    if let Some(h) = hub(app) {
        h.model.lock().unwrap().on_exit(Instant::now());
    }
    refresh(app);
}

/// Duraklatma değişti (tepsiden ya da pencereden; `set_paused` çağırır).
/// Her değişiklik bekleyen süreli sürdürmeyi geçersiz kılar.
pub fn on_paused(app: &AppHandle, paused: bool) {
    if let Some(h) = hub(app) {
        h.pause_token.fetch_add(1, Ordering::SeqCst);
        let until = h.pending_resume.lock().unwrap().take();
        let until = if paused { until } else { None };
        h.model
            .lock()
            .unwrap()
            .set_paused(paused, until, Instant::now());
    }
    refresh(app);
}

pub fn on_update_status(app: &AppHandle, status: &Value) {
    if let Some(h) = hub(app) {
        h.model.lock().unwrap().on_update(status);
    }
    refresh(app);
}

/// Pencereyi açar ve ön yüze sekmeyi söyler.
fn open_tab(app: &AppHandle, tab: &str) {
    super::show_main(app);
    let _ = app.emit(NAVIGATE_EVENT, tab);
}

/// Süreli ya da süresiz duraklatır. Süre dolunca, arada elle sürdürülmediyse
/// ya da yeniden duraklatılmadıysa izleme kendiliğinden sürer.
fn pause_for(app: &AppHandle, dur: Option<Duration>) {
    let Some(h) = hub(app) else { return };
    *h.pending_resume.lock().unwrap() = dur.map(|d| local_now() + d);
    // set_paused → on_paused: model güncellenir, belirteç artar.
    if let Err(e) = super::set_paused(app, true) {
        log::error!("izleme duraklatılamadı: {e}");
    }
    let token = h.pause_token.load(Ordering::SeqCst);
    let Some(d) = dur else { return };
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        tokio::time::sleep(d).await;
        let Some(h) = hub(&app) else { return };
        if h.pause_token.load(Ordering::SeqCst) == token && super::is_paused(&app) {
            log::info!("süreli duraklatma bitti, izleme sürüyor");
            resume(&app);
        }
    });
}

fn resume(app: &AppHandle) {
    if let Err(e) = super::set_paused(app, false) {
        log::error!("izleme sürdürülemedi: {e}");
    }
}

/// Sidecar'a hemen tarattırır. Ayrı bir RPC yok (sidecar değişmez): izlenen
/// ilk klasör kendi değerleriyle yeniden eklenir; `watch.add` aynı yol için
/// kaydı olduğu gibi günceller ve taramayı hemen tetikler.
fn scan_now(app: &AppHandle) {
    let Ok(bridge) = super::current_bridge(&app.state::<super::SidecarState>()) else {
        return;
    };
    let first = hub(app).and_then(|h| {
        let m = h.model.lock().unwrap();
        let params = watches(m.status.as_ref()).next().map(|w| {
            serde_json::json!({
                "path": str_at(w, "path"),
                "engagement_uuid": str_at(w, "engagement_uuid"),
                "label": str_at(w, "label"),
                "taxpayer_title": str_at(w, "taxpayer_title"),
            })
        });
        params
    });
    let Some(params) = first else { return };
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        rescan(&bridge, params).await;
        poll_status(&app);
    });
}

async fn rescan(bridge: &Arc<Bridge>, params: Value) {
    if let Err(e) = bridge
        .call("watch.add", params, Duration::from_secs(10))
        .await
    {
        log::warn!("tarama tetiklenemedi: {e}");
    }
}

/// Web panelini varsayılan tarayıcıda açar (API ve panel aynı adreste).
fn open_panel(app: &AppHandle) {
    let base = hub(app).and_then(|h| {
        let m = h.model.lock().unwrap();
        m.status
            .as_ref()
            .map(|st| str_at(st, "base_url").to_string())
    });
    let Some(base) = base.filter(|b| b.starts_with("https://") || b.starts_with("http://")) else {
        return;
    };
    #[allow(deprecated)]
    {
        use tauri_plugin_shell::ShellExt;
        if let Err(e) = app.shell().open(format!("{base}/"), None) {
            log::warn!("panel açılamadı: {e}");
        }
    }
}

fn on_menu(app: &AppHandle, event: MenuEvent) {
    let id = event.id().as_ref();
    match id {
        "open" => super::show_main(app),
        "scan" | "retry" => scan_now(app),
        "pause.30m" => pause_for(app, Some(Duration::from_secs(30 * 60))),
        "pause.2h" => pause_for(app, Some(Duration::from_secs(2 * 60 * 60))),
        "pause.manual" | "pause" => pause_for(app, None),
        "resume" => resume(app),
        "panel" => open_panel(app),
        "settings" => open_tab(app, "settings"),
        "details" => open_tab(app, "problems"),
        "update" => {
            let app = app.clone();
            tauri::async_runtime::spawn(async move {
                if let Err(e) = super::update_install(app.clone()).await {
                    log::error!("güncelleme tepsiden kurulamadı: {e}");
                    open_tab(&app, "settings");
                }
            });
        }
        "autostart" => {
            let want = !app.autolaunch().is_enabled().unwrap_or(false);
            if let Err(e) = super::set_autostart(app, want) {
                log::error!("otomatik başlatma ayarlanamadı: {e}");
            }
            if let Some(h) = hub(app) {
                h.model.lock().unwrap().autostart = app.autolaunch().is_enabled().unwrap_or(false);
                // İşaret kutusu tıklamayla kendiliğinden döner; başarısızsa
                // da doğru değeri göstermek için menü yeniden kurulur.
                *h.spec.lock().unwrap() = None;
            }
            refresh(app);
        }
        "quit" => super::quit(app),
        _ if id.starts_with("problem.") => open_tab(app, "problems"),
        _ if id.starts_with("recent.") => open_tab(app, "history"),
        _ => {}
    }
}

fn build_menu(app: &AppHandle, spec: &[Entry]) -> tauri::Result<Menu<Wry>> {
    let items = build_items(app, spec)?;
    let refs: Vec<&dyn IsMenuItem<Wry>> = items.iter().map(|b| b.as_ref()).collect();
    Menu::with_items(app, &refs)
}

fn build_items(app: &AppHandle, spec: &[Entry]) -> tauri::Result<Vec<Box<dyn IsMenuItem<Wry>>>> {
    let mut out: Vec<Box<dyn IsMenuItem<Wry>>> = Vec::with_capacity(spec.len());
    for (i, e) in spec.iter().enumerate() {
        match e {
            Entry::Info(text) => out.push(Box::new(MenuItem::with_id(
                app,
                format!("info.{i}"),
                text,
                false,
                None::<&str>,
            )?)),
            Entry::Action {
                id,
                text,
                enabled,
                accel,
            } => out.push(Box::new(MenuItem::with_id(
                app,
                id.as_str(),
                text,
                *enabled,
                *accel,
            )?)),
            Entry::Check { id, text, checked } => out.push(Box::new(CheckMenuItem::with_id(
                app,
                id.as_str(),
                text,
                true,
                *checked,
                None::<&str>,
            )?)),
            Entry::Submenu { text, items } => {
                let sub = build_items(app, items)?;
                let refs: Vec<&dyn IsMenuItem<Wry>> = sub.iter().map(|b| b.as_ref()).collect();
                out.push(Box::new(Submenu::with_items(app, text, true, &refs)?));
            }
            Entry::Separator => out.push(Box::new(PredefinedMenuItem::separator(app)?)),
        }
    }
    Ok(out)
}

#[cfg(test)]
mod tests {
    use super::*;
    use serde_json::json;

    fn day() -> Date {
        Date::from_calendar_date(2026, time::Month::November, 16).unwrap()
    }

    fn wall() -> OffsetDateTime {
        day()
            .with_hms(10, 42, 0)
            .unwrap()
            .assume_offset(UtcOffset::from_hms(3, 0, 0).unwrap())
    }

    fn status(extra: Value) -> Value {
        let mut st = json!({
            "base_url": "https://hakan.ornek.com",
            "signed_in": true,
            "organization": { "uuid": "o", "slug": "demir", "name": "Demir Mali Müşavirlik" },
            "agent_name": "Ofis-PC",
            "running": true,
            "watches": [
                { "path": "/e/atlas", "engagement_uuid": "e1", "label": "", "taxpayer_title": "Atlas Gıda",
                  "activity": { "uploaded": 1, "errors": [] } },
                { "path": "/e/kuzey", "engagement_uuid": "e2", "label": "", "taxpayer_title": "Kuzey Lojistik",
                  "activity": { "uploaded": 0, "errors": [] } }
            ],
            "stats": { "watched_folders": 2, "uploaded": 1, "failed": 0, "last_scan": "2026-11-16T07:40:00Z" },
        });
        if let (Some(o), Some(e)) = (st.as_object_mut(), extra.as_object()) {
            for (k, v) in e {
                o.insert(k.clone(), v.clone());
            }
        }
        st
    }

    fn ready(now: Instant) -> Model {
        let mut m = Model::new("1.4.2", now);
        m.on_event("ready", &json!({}), now, day());
        m.status = Some(status(json!({})));
        m
    }

    fn infos(spec: &[Entry]) -> Vec<String> {
        spec.iter()
            .filter_map(|e| match e {
                Entry::Info(t) => Some(t.clone()),
                _ => None,
            })
            .collect()
    }

    fn ids(spec: &[Entry]) -> Vec<String> {
        spec.iter()
            .filter_map(|e| match e {
                Entry::Action { id, .. } | Entry::Check { id, .. } => Some(id.clone()),
                Entry::Submenu { items, .. } => Some(format!("sub[{}]", ids(items).join(","))),
                _ => None,
            })
            .collect()
    }

    #[test]
    fn bagli_menu_tuvaldeki_gibi() {
        let now = Instant::now();
        let m = ready(now);
        assert_eq!(m.icon_state(now), IconState::Connected);
        let spec = m.menu(now, wall(), day());
        assert_eq!(
            infos(&spec),
            vec![
                "Her şey yolunda",
                "Demir Mali Müşavirlik · Ofis-PC",
                "0 bugün gönderildi",
                "2 klasör izleniyor",
                "Son tarama: 2 dk önce",
                "Sürüm 1.4.2",
            ]
        );
        assert_eq!(
            ids(&spec),
            vec![
                "open",
                "scan",
                "sub[pause.30m,pause.2h,pause.manual]",
                "panel",
                "settings",
                "autostart",
                "quit"
            ]
        );
    }

    #[test]
    fn yukleme_gonderiyor_ve_son_dosyalar() {
        let t0 = Instant::now();
        let mut m = ready(t0);
        for name in ["a.xml", "KLM2026000184213.xml"] {
            let ev = json!({ "name": name, "engagement_uuid": "e1", "outcome": "uploaded" });
            assert!(m.on_event("upload", &ev, t0, day()));
        }
        // Reddedilen dosya sayılmaz, son dosyalara girmez.
        m.on_event(
            "upload",
            &json!({ "name": "x.pdf", "outcome": "rejected" }),
            t0,
            day(),
        );
        assert_eq!(m.icon_state(t0), IconState::Sending);
        let spec = m.menu(t0, wall(), day());
        assert!(infos(&spec).contains(&"2 bugün gönderildi".to_string()));
        assert_eq!(infos(&spec)[0], "Gönderiliyor…");
        assert!(spec.contains(&Entry::action(
            "recent.0",
            "KLM2026000184213.xml · Atlas Gıda"
        )));
        assert!(spec.contains(&Entry::action("recent.1", "a.xml · Atlas Gıda")));
        // Pencere geçince yeniden "Bağlı".
        let later = t0 + SENDING_WINDOW + Duration::from_secs(1);
        assert_eq!(m.icon_state(later), IconState::Connected);
        // Ertesi gün sayaç sıfırlanır.
        let tomorrow = day().next_day().unwrap();
        assert!(infos(&m.menu(later, wall(), tomorrow)).contains(&"0 bugün gönderildi".to_string()));
    }

    #[test]
    fn sorunlu_dosyalar_windows_menusu() {
        let now = Instant::now();
        let mut m = ready(now);
        m.status = Some(status(json!({ "watches": [
            { "path": "/e/a", "engagement_uuid": "e1", "taxpayer_title": "Atlas Gıda", "activity": { "errors": [
                { "name": "TKN2026000391827.xml", "message": "Sunucuya ulaşılamadı", "retry": true },
                { "name": "tarama_0091.pdf", "message": "UBL değil", "retry": false },
                { "name": "c.xml", "message": "Okunamadı", "retry": true }
            ] } }
        ] })));
        assert_eq!(m.icon_state(now), IconState::Problem);
        let spec = m.menu(now, wall(), day());
        let inf = infos(&spec);
        assert_eq!(inf[0], "3 dosya gönderilemedi");
        assert_eq!(inf[1], "2 tanesi sonraki taramada yeniden denenecek");
        let ids = ids(&spec);
        assert_eq!(
            &ids[..5],
            ["problem.0", "problem.1", "problem.2", "retry", "details"]
        );
        assert!(m.tooltip(now).contains("3 dosya gönderilemedi"));
    }

    #[test]
    fn sorun_oncelikleri() {
        let now = Instant::now();
        let mut m = ready(now);
        m.status = Some(status(json!({ "heartbeat_error": "dial tcp: timeout" })));
        assert_eq!(m.problem(now).unwrap().0, "Sunucuya ulaşılamıyor");
        m.status = Some(status(json!({ "signed_in": false })));
        assert_eq!(m.problem(now).unwrap().0, "Oturum açılmadı");
        m.status = Some(status(json!({ "organization": null })));
        assert_eq!(m.problem(now).unwrap().0, "Kurulum tamamlanmadı");
        // Kurulum bitmeden sayaçlar gösterilmez.
        assert!(!infos(&m.menu(now, wall(), day()))
            .iter()
            .any(|s| s.contains("klasör")));
    }

    #[test]
    fn sidecar_kapali_kisa_kesinti_sorun_degil() {
        let t0 = Instant::now();
        let mut m = ready(t0);
        m.on_exit(t0);
        assert_eq!(
            m.icon_state(t0 + Duration::from_secs(3)),
            IconState::Connected
        );
        assert_eq!(infos(&m.menu(t0, wall(), day()))[0], "Başlatılıyor…");
        let late = t0 + DOWN_GRACE;
        assert_eq!(m.icon_state(late), IconState::Problem);
        assert_eq!(m.problem(late).unwrap().0, "Yerel ajan çalışmıyor");
        // Tarama düğmesi sidecar yokken kapalı.
        let spec = m.menu(late, wall(), day());
        assert!(spec
            .iter()
            .any(|e| matches!(e, Entry::Action { id, enabled: false, .. } if id == "scan")));
    }

    #[test]
    fn duraklatma_sorunu_bastirir() {
        let t0 = Instant::now();
        let mut m = ready(t0);
        m.status = Some(status(json!({ "heartbeat_error": "x" })));
        let at = wall() + Duration::from_secs(30 * 60);
        m.set_paused(true, Some(at), t0);
        let late = t0 + DOWN_GRACE * 4;
        assert_eq!(m.icon_state(late), IconState::Paused);
        let spec = m.menu(late, wall(), day());
        assert_eq!(
            &infos(&spec)[..2],
            ["İzleme duraklatıldı", "Kendiliğinden sürer: 11:12"]
        );
        assert!(ids(&spec).contains(&"resume".to_string()));
        m.set_paused(false, None, late);
        assert!(m.resume_at.is_none());
        assert_eq!(m.icon_state(late), IconState::Connected);
    }

    #[test]
    fn guncelleme_hazir() {
        let now = Instant::now();
        let mut m = ready(now);
        m.on_update(&json!({ "state": "up_to_date", "current": "1.4.2" }));
        assert!(infos(&m.menu(now, wall(), day())).contains(&"Sürüm 1.4.2 · güncel".to_string()));
        m.on_update(&json!({ "state": "available", "available": "1.4.3" }));
        assert_eq!(m.icon_state(now), IconState::UpdateReady);
        assert!(m.menu(now, wall(), day()).contains(&Entry::action(
            "update",
            "Güncelle ve yeniden başlat · 1.4.3"
        )));
        m.on_update(&json!({ "state": "error" }));
        assert_eq!(m.icon_state(now), IconState::Connected);
    }

    #[test]
    fn once_metinleri() {
        assert_eq!(ago(time::Duration::seconds(20)), "az önce");
        assert_eq!(ago(time::Duration::minutes(12)), "12 dk önce");
        assert_eq!(ago(time::Duration::hours(3)), "3 sa önce");
        assert_eq!(ago(time::Duration::days(2)), "2 gün önce");
        assert_eq!(ago(time::Duration::minutes(-5)), "az önce");
        assert_eq!(short("abcdef", 4), "abc…");
    }

    fn alpha(px: &[u8], x: usize, y: usize) -> u8 {
        px[(y * ICON_SIZE as usize + x) * 4 + 3]
    }

    #[test]
    fn simge_bes_durum_farkli() {
        let st = Style {
            template: false,
            dark: false,
        };
        let all = [
            render(IconState::Connected, 0, st),
            render(IconState::Sending, 0, st),
            render(IconState::Paused, 0, st),
            render(IconState::Problem, 0, st),
            render(IconState::UpdateReady, 0, st),
        ];
        for (i, a) in all.iter().enumerate() {
            assert_eq!(a.len(), (ICON_SIZE * ICON_SIZE * 4) as usize);
            for b in &all[i + 1..] {
                assert_ne!(a, b);
            }
        }
        let c = &all[0];
        // Orta kristalin içi dolu, köşe boş; tepe sivri (kenarda boş).
        assert_eq!(alpha(c, 16, 20), 255);
        assert_eq!(alpha(c, 0, 0), 0);
        assert_eq!(alpha(c, 12, 3), 0);
        // Duraklatıldı: %40.
        assert_eq!(alpha(&all[2], 16, 20), 102);
        // Rozet rengi.
        let p = &all[3];
        let i = (6 * ICON_SIZE as usize + 26) * 4;
        assert_eq!(&p[i..i + 4], &[0xE5, 0x48, 0x4D, 255]);
        let u = &all[4];
        assert_eq!(&u[i..i + 4], &[0x24, 0x48, 0xFF, 255]);
    }

    #[test]
    fn gonderiyor_kristaller_sirayla() {
        let st = Style {
            template: true,
            dark: false,
        };
        let centers = [5usize, 16, 27];
        for f in 0..3 {
            let px = render(IconState::Sending, f, st);
            for (i, x) in centers.iter().enumerate() {
                let a = alpha(&px, *x, 26);
                if i == f {
                    assert_eq!(a, 255, "kare {f} kristal {i}");
                } else {
                    assert_eq!(a, 89, "kare {f} kristal {i}");
                }
            }
            // Şablon simge siyah.
            assert_eq!(&px[(26 * 32 + 16) * 4..(26 * 32 + 16) * 4 + 3], &[0, 0, 0]);
        }
    }
}
