import { secure, StorageKeys } from '../storage';

/** Token pair as returned by `/v1/auth/login` and `/v1/auth/refresh` (schema `Tokens`). */
export type Tokens = {
  access_token: string;
  refresh_token: string;
  token_type: string;
  expires_in: number;
  refresh_expires_at: string;
};

export type StoredSession = {
  accessToken: string;
  refreshToken: string;
  /** epoch ms */
  accessExpiresAt: number;
  refreshExpiresAt: string;
};

let current: StoredSession | null = null;
const listeners = new Set<(s: StoredSession | null) => void>();

function emit() {
  for (const l of listeners) l(current);
}

/** The signed-in user's tokens, kept in secure storage across launches. */
export const session = {
  get: () => current,

  async load() {
    const raw = await secure.get(StorageKeys.session);
    if (!raw) return null;
    try {
      current = JSON.parse(raw) as StoredSession;
    } catch {
      current = null;
      await secure.remove(StorageKeys.session);
    }
    return current;
  },

  async setTokens(tokens: Tokens) {
    current = {
      accessToken: tokens.access_token,
      refreshToken: tokens.refresh_token,
      accessExpiresAt: Date.now() + tokens.expires_in * 1000,
      refreshExpiresAt: tokens.refresh_expires_at,
    };
    await secure.set(StorageKeys.session, JSON.stringify(current));
  },

  async clear() {
    if (!current) return;
    current = null;
    await secure.remove(StorageKeys.session);
    emit();
  },

  /** Fires when the session ends (logout, refresh rejected). */
  subscribe(listener: (s: StoredSession | null) => void) {
    listeners.add(listener);
    return () => {
      listeners.delete(listener);
    };
  },
};
