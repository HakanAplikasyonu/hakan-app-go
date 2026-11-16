// Pure helper (no React Native imports) so node --test can import it.

/** The `url` of a push payload (data.url), if it is a string. */
export function notificationUrl(data: unknown): string | null {
  if (!data || typeof data !== 'object') return null;
  const url = (data as { url?: unknown }).url;
  return typeof url === 'string' && url ? url : null;
}
