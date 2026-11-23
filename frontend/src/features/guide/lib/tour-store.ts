/**
 * Rehber turunun durumu: açık mı, hangi adımda, daha önce gösterildi mi.
 *
 * Tek bir modül deposu; kabuktaki `GuideTourHost` ile Rehber sayfasındaki
 * "Turu başlat" düğmesi aynı turu açıp kapatır. React tarafı
 * `useSyncExternalStore` ile okur (efekt içinde setState yok).
 *
 * İlk girişte tur kendiliğinden açılır; gösterildiği tarayıcıda
 * localStorage'a yazılır. Otomatik tarayıcıda (`navigator.webdriver`,
 * Playwright) kendiliğinden açılmaz: mevcut e2e ve görsel testler etkilenmez.
 */

const SEEN_KEY = "hakan.guide.tour_seen.v1";

export type TourState = {
  /** Rehber sayfasından elle başlatıldı. */
  manual: boolean;
  step: number;
  /** Bu oturumda kapatıldı (localStorage yazılamasa da yeniden açılmaz). */
  dismissed: boolean;
};

const INITIAL: TourState = { manual: false, step: 0, dismissed: false };

let state: TourState = INITIAL;
const listeners = new Set<() => void>();

function emit() {
  for (const l of listeners) l();
}

export function subscribeTour(listener: () => void): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

export function getTourState(): TourState {
  return state;
}

export function getServerTourState(): TourState {
  return INITIAL;
}

/** Tur bu tarayıcıda daha önce gösterildiyse ya da gösterilmemeliyse true. */
export function tourSeen(): boolean {
  if (typeof window === "undefined") return true;
  if (typeof navigator !== "undefined" && navigator.webdriver) return true;
  try {
    return window.localStorage.getItem(SEEN_KEY) === "1";
  } catch {
    // Depolama kapalıysa her sayfada açılmasın.
    return true;
  }
}

/** Sunucuda ve hidrasyonda tur kapalı başlar. */
export function tourSeenServer(): boolean {
  return true;
}

function markSeen() {
  try {
    window.localStorage.setItem(SEEN_KEY, "1");
  } catch {
    // Yazılamazsa `dismissed` bu oturumda yeniden açılmasını önler.
  }
}

export function startTour() {
  state = { manual: true, step: 0, dismissed: state.dismissed };
  emit();
}

export function closeTour() {
  markSeen();
  state = { manual: false, step: 0, dismissed: true };
  emit();
}

export function goToStep(step: number) {
  state = { ...state, step: Math.max(0, step) };
  emit();
}
