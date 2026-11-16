/**
 * The signed-in user's tokens, kept in secure storage across launches (#295).
 * Pure: the store is passed in, so `node --test` runs it with a map.
 */

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

/** Key/value store (secure storage on the device). */
export type KeyValueStore = {
  get(key: string): Promise<string | null>;
  set(key: string, value: string): Promise<void>;
  remove(key: string): Promise<void>;
};

export type Session = ReturnType<typeof createSession>;

export function createSession(store: KeyValueStore, key: string, now: () => number = Date.now) {
  let current: StoredSession | null = null;
  const listeners = new Set<(s: StoredSession | null) => void>();

  function emit() {
    for (const l of listeners) l(current);
  }

  return {
    get: () => current,

    async load() {
      const raw = await store.get(key);
      if (!raw) return null;
      try {
        current = JSON.parse(raw) as StoredSession;
      } catch {
        current = null;
        await store.remove(key);
      }
      return current;
    },

    async setTokens(tokens: Tokens) {
      current = {
        accessToken: tokens.access_token,
        refreshToken: tokens.refresh_token,
        accessExpiresAt: now() + tokens.expires_in * 1000,
        refreshExpiresAt: tokens.refresh_expires_at,
      };
      await store.set(key, JSON.stringify(current));
    },

    async clear() {
      if (!current) return;
      current = null;
      await store.remove(key);
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
}
