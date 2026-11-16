/**
 * Values kept on the device. Secure storage itself is shared with the
 * mükellef app (@hakan/mobile-core, #295); the keys are this app's own.
 */
export { secure } from '@hakan/mobile-core/secure-store';

/** Keys of values kept on the device. */
export const StorageKeys = {
  session: 'hakanofis.session',
  organization: 'hakanofis.organization',
  pushToken: 'hakanofis.push-token',
} as const;
