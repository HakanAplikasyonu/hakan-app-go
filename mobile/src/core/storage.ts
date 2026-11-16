/**
 * Values kept on the device. Secure storage itself is shared with the office
 * app (@hakan/mobile-core, #295); the keys are this app's own.
 */
export { secure } from '@hakan/mobile-core/secure-store';

/** Keys of values kept on the device. */
export const StorageKeys = {
  session: 'hakan.session',
  taxpayer: 'hakan.taxpayer',
  pushToken: 'hakan.push-token',
  /** Mükellef ilk açılışı (KVKK onayı) görüldü (#332). */
  onboarding: 'hakan.ilk-acilis',
} as const;
