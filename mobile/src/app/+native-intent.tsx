/**
 * Rewrites incoming system links before expo-router resolves them (#201):
 * universal / app links carry the web panel's paths (`/m/{taxpayer}/tasks`),
 * which map to the app's own routes; unknown links open the home screen.
 * The scheme links (`hakan://tasks`, `hakan://receipts/{id}`) pass through the
 * same mapping. Taxpayer switching for web links happens for notification
 * taps (use-notification-routing); a cold link opens on the active company.
 */
import { systemPath } from '@/core/push/links';

export function redirectSystemPath({ path }: { path: string; initial: boolean }) {
  try {
    return systemPath(path);
  } catch {
    return '/';
  }
}
