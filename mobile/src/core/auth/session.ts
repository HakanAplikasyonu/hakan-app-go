// Oturum ve yenileme ortak pakette (@hakan/mobile-core, #295).
import { createSession } from '@hakan/mobile-core/auth/session';

import { secure, StorageKeys } from '../storage';

export type { StoredSession, Tokens } from '@hakan/mobile-core/auth/session';

/** The signed-in user's tokens, kept in secure storage across launches. */
export const session = createSession(secure, StorageKeys.session);
