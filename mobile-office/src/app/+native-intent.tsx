/**
 * Sistem bağlantılarını expo-router çözmeden önce yeniden yazar (#241; kaynak
 * mobile/src/app/+native-intent.tsx, #201): evrensel bağlantılar ofis
 * panelinin yollarını taşır (`/t/{slug}/warnings`), bunlar uygulama yollarına
 * eşlenir; `hakanofis://` bağlantıları aynı eşlemeden geçer. Bilinmeyen
 * bağlantı ana sayfayı açar.
 */
import { systemPath } from '@/core/push/links';

export function redirectSystemPath({ path }: { path: string; initial: boolean }) {
  try {
    return systemPath(path);
  } catch {
    return '/';
  }
}
