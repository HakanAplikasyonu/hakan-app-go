//! Sidecar'ın yeniden başlatma politikası (#204). Tauri'den bağımsızdır;
//! kabuk (`lib.rs`) sidecar kapanınca `on_exit` ile bir sonraki beklemeyi
//! alır.
//!
//! Kural: kapanan sidecar katlanarak artan beklemeyle (1, 2, 4, 8, 16 sn,
//! en çok 16 sn) yeniden başlatılır. Böylece zorla öldürülen sidecar en geç
//! 16 sn içinde kalkar (bitti tanımı: 30 sn). Bir süre (`STABLE_AFTER`)
//! sorunsuz çalışan sidecar tekrar kapanırsa sayaç sıfırlanır: ara sıra
//! düşen sidecar hep 1 sn'de, sürekli çöken sidecar seyrek denenir.

use std::time::Duration;

/// İlk yeniden başlatma beklemesi.
pub const BASE: Duration = Duration::from_secs(1);
/// En uzun bekleme.
pub const MAX: Duration = Duration::from_secs(16);
/// Bu kadar çalışan sidecar "kararlı" sayılır; sayaç sıfırlanır.
pub const STABLE_AFTER: Duration = Duration::from_secs(60);

/// Kabuğun sidecar için istediği durum.
#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum Mode {
    /// Çalışsın; kapanırsa yeniden başlatılsın.
    Running,
    /// Kullanıcı izlemeyi duraklattı: kapalı kalsın.
    Paused,
    /// Uygulama kapanıyor: yeniden başlatma yok.
    ShuttingDown,
}

#[derive(Debug, Default)]
pub struct Backoff {
    failures: u32,
}

impl Backoff {
    /// Sidecar `ran` süre çalışıp kapandı: bir sonraki başlatmaya kadar
    /// beklenecek süre.
    pub fn on_exit(&mut self, ran: Duration) -> Duration {
        if ran >= STABLE_AFTER {
            self.failures = 0;
        }
        let shift = self.failures.min(4);
        self.failures = self.failures.saturating_add(1);
        (BASE * (1u32 << shift)).min(MAX)
    }

    /// Kullanıcı elle başlattı (ör. "İzlemeyi sürdür"): sayaç sıfırlanır.
    pub fn reset(&mut self) {
        self.failures = 0;
    }
}

/// Kapanan sidecar yeniden başlatılsın mı? Yalnızca kabuk hâlâ çalışmasını
/// istiyorsa ve kapanan süreç en son başlatılan süreçse (eski bir sürecin
/// geç gelen kapanma olayı yenisini ikiye katlamasın).
pub fn should_restart(mode: Mode, exited_generation: u64, current_generation: u64) -> bool {
    mode == Mode::Running && exited_generation == current_generation
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn backoff_grows_and_caps() {
        let mut b = Backoff::default();
        let quick = Duration::from_secs(1);
        let delays: Vec<u64> = (0..7).map(|_| b.on_exit(quick).as_secs()).collect();
        assert_eq!(delays, vec![1, 2, 4, 8, 16, 16, 16]);
    }

    #[test]
    fn worst_case_restart_is_within_30s() {
        let mut b = Backoff::default();
        for _ in 0..100 {
            assert!(b.on_exit(Duration::ZERO) <= Duration::from_secs(30));
        }
    }

    #[test]
    fn stable_run_resets_the_counter() {
        let mut b = Backoff::default();
        for _ in 0..5 {
            b.on_exit(Duration::from_secs(2));
        }
        assert_eq!(b.on_exit(STABLE_AFTER), BASE);
        assert_eq!(b.on_exit(Duration::from_secs(2)), Duration::from_secs(2));
    }

    #[test]
    fn manual_reset() {
        let mut b = Backoff::default();
        b.on_exit(Duration::ZERO);
        b.on_exit(Duration::ZERO);
        b.reset();
        assert_eq!(b.on_exit(Duration::ZERO), BASE);
    }

    #[test]
    fn restart_only_when_running_and_current() {
        assert!(should_restart(Mode::Running, 3, 3));
        assert!(
            !should_restart(Mode::Running, 2, 3),
            "eski sürecin kapanması"
        );
        assert!(!should_restart(Mode::Paused, 3, 3));
        assert!(!should_restart(Mode::ShuttingDown, 3, 3));
    }
}
