import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';

import { api, request } from '../api/client';
import { isApiError, type Schemas } from '../api/envelope';
import { registerPushDevice, unregisterPushDevice } from '../push/push';
import { secure, StorageKeys } from '../storage';
import { session, type Tokens } from './session';
import { pickTaxpayer, type TaxpayerOption } from './taxpayers';

export type Me = Schemas['Me'];
export type TaxpayerContext = Schemas['TaxpayerContext'];

/**
 * booting → (no session) signedOut → login → selectTaxpayer → ready.
 * `selectTaxpayer` is also where a user without any company lands.
 */
export type AuthStatus = 'booting' | 'signedOut' | 'selectTaxpayer' | 'ready';

type AuthState = {
  status: AuthStatus;
  me: Me | null;
  taxpayers: TaxpayerOption[];
  /** Active company (resolved with `/v1/taxpayer/context`). */
  taxpayer: TaxpayerContext | null;
};

type AuthApi = AuthState & {
  signIn: (input: { email: string; password: string; totp_code?: string }) => Promise<void>;
  selectTaxpayer: (uuid: string) => Promise<void>;
  /** Back to the picker (keeps the current company until another is chosen). */
  switchTaxpayer: () => void;
  /** Re-reads memberships (after an invitation was accepted on the web). */
  reload: () => Promise<void>;
  signOut: () => Promise<void>;
};

const AuthContext = createContext<AuthApi | null>(null);

const initial: AuthState = { status: 'booting', me: null, taxpayers: [], taxpayer: null };

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(initial);
  const mounted = useRef(true);

  const resolve = useCallback(async (preferred: string | null) => {
    const me = await api.get<Me>('/v1/auth/me');
    const taxpayers = (me.taxpayers ?? []) as TaxpayerOption[];
    const lastUsed = preferred ?? (await secure.get(StorageKeys.taxpayer));
    const uuid = pickTaxpayer(taxpayers, lastUsed);
    if (uuid) {
      try {
        const taxpayer = await api.get<TaxpayerContext>('/v1/taxpayer/context', { taxpayer: uuid });
        await secure.set(StorageKeys.taxpayer, uuid);
        setState({ status: 'ready', me, taxpayers, taxpayer });
        void registerPushDevice();
        return;
      } catch (e) {
        if (!isApiError(e) || e.code !== 'NO_TAXPAYER_MEMBERSHIP') throw e;
        await secure.remove(StorageKeys.taxpayer);
      }
    }
    setState({ status: 'selectTaxpayer', me, taxpayers, taxpayer: null });
  }, []);

  useEffect(() => {
    mounted.current = true;
    const off = session.subscribe((s) => {
      if (!s && mounted.current) setState({ ...initial, status: 'signedOut' });
    });
    (async () => {
      const stored = await session.load();
      if (!stored) {
        setState({ ...initial, status: 'signedOut' });
        return;
      }
      try {
        await resolve(null);
      } catch (e) {
        // Offline at launch: keep the session, the user can retry by signing in again.
        if (isApiError(e) && e.status === 401) await session.clear();
        setState({ ...initial, status: 'signedOut' });
      }
    })();
    return () => {
      mounted.current = false;
      off();
    };
  }, [resolve]);

  const signIn = useCallback<AuthApi['signIn']>(
    async (input) => {
      const tokens = await request<Tokens>('POST', '/v1/auth/login', { body: input, anonymous: true });
      await session.setTokens(tokens);
      await resolve(null);
    },
    [resolve],
  );

  const selectTaxpayer = useCallback<AuthApi['selectTaxpayer']>(async (uuid) => resolve(uuid), [resolve]);

  const switchTaxpayer = useCallback(() => {
    setState((s) => (s.me ? { ...s, status: 'selectTaxpayer' } : s));
  }, []);

  const reload = useCallback(async () => resolve(state.taxpayer?.taxpayer.uuid ?? null), [resolve, state.taxpayer]);

  const signOut = useCallback(async () => {
    await unregisterPushDevice();
    const current = session.get();
    if (current) {
      await api.post('/v1/auth/logout', { refresh_token: current.refreshToken }).catch(() => {});
    }
    await secure.remove(StorageKeys.taxpayer);
    await session.clear();
    setState({ ...initial, status: 'signedOut' });
  }, []);

  const value = useMemo<AuthApi>(
    () => ({ ...state, signIn, selectTaxpayer, switchTaxpayer, reload, signOut }),
    [state, signIn, selectTaxpayer, switchTaxpayer, reload, signOut],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthApi {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth outside AuthProvider');
  return ctx;
}
