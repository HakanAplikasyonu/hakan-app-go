/**
 * Fiş kamerasında yakınlaştırma (saf; node --test ile denenir). Oğuz'un geri
 * bildirimi: iki parmakla yakınlaştırma ve 0,5× / 1× / 2× kısa düğmeleri.
 *
 * expo-camera `zoom` 0–1 arasıdır (cihazın en büyük yakınlaştırmasının
 * oranı). 1× = 0. 2× için cihazın en büyük çarpanı bilinmediğinden yaygın
 * değer (8×) varsayılır; yaklaşık 2× görünür. 0,5× yakınlaştırma değil,
 * geniş açı mercek seçimidir: yalnız cihazda ultra geniş mercek varsa
 * (iOS `selectedLens`) gösterilir.
 */

/** expo-camera's iOS device type of the ultra wide lens. */
export const ULTRA_WIDE = 'AVCaptureDeviceTypeBuiltInUltraWideCamera';
/** The standard back lens (1×). */
export const WIDE = 'AVCaptureDeviceTypeBuiltInWideAngleCamera';

/** Assumed max zoom factor of a phone's back camera (zoom = 1). */
export const MAX_FACTOR = 8;

/** `id` names the button (testID `capture-zoom-<id>`). */
export type ZoomStep = { factor: number; label: string; id: string };

const STEPS: ZoomStep[] = [
  { factor: 0.5, label: '0,5×', id: '05' },
  { factor: 1, label: '1×', id: '1' },
  { factor: 2, label: '2×', id: '2' },
];

/** Quick buttons to show: 0,5× only with an ultra wide lens. */
export function zoomSteps(hasUltraWide: boolean): ZoomStep[] {
  return STEPS.filter((s) => hasUltraWide || s.factor >= 1);
}

const clamp01 = (n: number) => Math.min(1, Math.max(0, n));

/** expo-camera zoom (0–1) for a factor ≥ 1 (1× → 0). */
export function zoomForFactor(factor: number, maxFactor = MAX_FACTOR): number {
  if (!(factor > 1) || !(maxFactor > 1)) return 0;
  return clamp01((factor - 1) / (maxFactor - 1));
}

/** Factor shown for an expo-camera zoom (inverse of zoomForFactor). */
export function factorForZoom(zoom: number, maxFactor = MAX_FACTOR): number {
  return 1 + clamp01(zoom) * (maxFactor - 1);
}

/** Lens and zoom to apply for a quick button. */
export function applyStep(factor: number, hasUltraWide: boolean): { lens: string | undefined; zoom: number } {
  if (factor < 1 && hasUltraWide) return { lens: ULTRA_WIDE, zoom: 0 };
  return { lens: undefined, zoom: zoomForFactor(Math.max(1, factor)) };
}

/** The quick button that matches the camera's state (null while pinched in between). */
export function activeStep(zoom: number, ultraWide: boolean): number | null {
  if (ultraWide) return zoom <= 0.001 ? 0.5 : null;
  for (const s of STEPS) {
    if (s.factor >= 1 && Math.abs(zoomForFactor(s.factor) - zoom) < 0.01) return s.factor;
  }
  return null;
}

/** Whether the device has an ultra wide lens (expo-camera `onAvailableLensesChanged`). */
export function hasUltraWideLens(lenses: readonly { deviceType: string }[] | undefined): boolean {
  return !!lenses?.some((l) => l.deviceType === ULTRA_WIDE);
}

export type Touch = { pageX: number; pageY: number };

/** Distance between the first two fingers; null with fewer than two. */
export function pinchDistance(touches: readonly Touch[]): number | null {
  if (touches.length < 2) return null;
  const [a, b] = touches;
  return Math.hypot(a.pageX - b.pageX, a.pageY - b.pageY);
}

/** Zoom change per pixel of finger spread (a full-screen pinch ≈ the whole range). */
export const PINCH_SENSITIVITY = 1 / 400;

/**
 * Zoom while pinching: the zoom when two fingers came down, the finger
 * distance then and now. Spreading zooms in, pinching zooms out, 0–1.
 */
export function pinchZoom(startZoom: number, startDistance: number, distance: number): number {
  return clamp01(startZoom + (distance - startDistance) * PINCH_SENSITIVITY);
}
