/**
 * The installed build's real version (#203): store version and build number
 * from the native binary (expo-application), plus the OTA update in use
 * (expo-updates) when the build runs one.
 */
import * as Application from 'expo-application';
import * as Updates from 'expo-updates';

export function versionLabel(): string {
  const version = Application.nativeApplicationVersion ?? '?';
  const build = Application.nativeBuildVersion;
  const parts = [`Sürüm ${version}${build ? ` (${build})` : ''}`];
  if (Updates.isEnabled && Updates.channel) parts.push(`kanal ${Updates.channel}`);
  if (Updates.isEnabled && Updates.updateId && !Updates.isEmbeddedLaunch) parts.push(`güncelleme ${Updates.updateId.slice(0, 8)}`);
  return parts.join(' · ');
}
