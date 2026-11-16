// Kaynak: mobile/src/core/auth/launch.ts (#241'de kopyalandı).
/**
 * What a failed session check at launch means (#202). Only a refused session
 * (401) signs the user out; no network, a timeout or a server error keeps the
 * stored session and shows "no connection, try again".
 */
export type LaunchFailure = 'signedOut' | 'offline';

export function launchFailure(e: unknown): LaunchFailure {
  const status = typeof e === 'object' && e !== null && 'status' in e ? (e as { status: unknown }).status : undefined;
  return status === 401 ? 'signedOut' : 'offline';
}
