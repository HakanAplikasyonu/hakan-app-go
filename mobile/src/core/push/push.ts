/**
 * Expo push (#201; shared, @hakan/mobile-core #295): permission, token and
 * device registration (`/v1/push-devices`).
 */
import { createPush } from '@hakan/mobile-core/push/push';

import { api } from '../api/client';
import { secure, StorageKeys } from '../storage';

export { notificationsModule, getPushPermission, type PushPermission, type RegisterResult } from '@hakan/mobile-core/push/push';

export const { registerPushDevice, enablePush, unregisterPushDevice, installForegroundHandler, subscribeNotificationOpens } = createPush({
  api,
  store: secure,
  tokenKey: StorageKeys.pushToken,
  channelName: 'Hakan',
});
