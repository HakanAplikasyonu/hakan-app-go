// Kaynak: mobile/src/core/auth/auth-provider.tsx (#236). Mükellef seçimi yerine
// ofis seçimi (`Me.organizations` + `/v1/tenant/context`) ve ofis rolü denetimi.
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';

import { api, request } from '../api/client';
import { isApiError, type Schemas } from '../api/envelope';
import { setReportUser } from '../observability/sentry';
import { registerPushDevice, unregisterPushDevice } from '../push/push';
import { launchFailure } from './launch';
import { secure, StorageKeys } from '../storage';
import { officeAccess, pickOffice, type OfficeAccess, type OfficeOption } from './offices';
import { session, type Tokens } from './session';

export type Me = Schemas['Me'];
export type TenantContext = Schemas['TenantContext'];

/**
 * booting → (oturum yok) signedOut → giriş → selectOffice → ready.
 * Ofis üyeliği olmayan hesap `denied` durumuna düşer (mükellef hesabı ya da davetsiz).
 */
// `offline`: kayıtlı oturum açılışta denetlenemedi (bağlantı yok, zaman aşımı,
// sunucu hatası). Oturum korunur; `retry` yeniden dener (#241, kaynak #202).
export type AuthStatus = 'booting' | 'offline' | 'signedOut' | 'denied' | 'selectOffice' | 'ready';

type AuthState = {
  status: AuthStatus;
  me: Me | null;
  offices: OfficeOption[];
  /** Açık ofis (`/v1/tenant/context` ile doğrulanır). */
  office: TenantContext | null;
  /** Açık ofisin slug'ı: `/v1/tenant/*` isteklerinin `X-Organization` başlığı. */
  officeSlug: string | null;
  access: OfficeAccess | null;
};

type AuthApi = AuthState & {
  signIn: (input: { email: string; password: string; totp_code?: string }) => Promise<void>;
  selectOffice: (slug: string) => Promise<void>;
  /** Seçiciye dön (başka ofis seçilene kadar açık ofis korunur). */
  switchOffice: () => void;
  reload: () => Promise<void>;
  /** Çevrimdışı açılıştan sonra kayıtlı oturumu yeniden denetler. */
  retry: () => Promise<void>;
  signOut: () => Promise<void>;
  /** Açık ofiste izin var mı (`tenant.*`). */
  can: (permission: string) => boolean;
};

const AuthContext = createContext<AuthApi | null>(null);

const initial: AuthState = { status: 'booting', me: null, offices: [], office: null, officeSlug: null, access: null };

export function AuthProvider({ children }: { children: ReactNode }) {
  const [state, setState] = useState<AuthState>(initial);
  const mounted = useRef(true);

  const resolve = useCallback(async (preferred: string | null) => {
    const me = await api.get<Me>('/v1/auth/me');
    const access = officeAccess(me);
    const offices = (me.organizations ?? []) as OfficeOption[];
    if (access !== 'office') {
      setState({ ...initial, status: 'denied', me, access });
      return;
    }
    const lastUsed = preferred ?? (await secure.get(StorageKeys.organization));
    const slug = preferred ?? pickOffice(offices, lastUsed);
    if (slug) {
      try {
        const office = await api.get<TenantContext>('/v1/tenant/context', { organization: slug });
        await secure.set(StorageKeys.organization, slug);
        setState({ status: 'ready', me, offices, office, officeSlug: slug, access });
        setReportUser(me.user?.uuid ?? null);
        void registerPushDevice();
        return;
      } catch (e) {
        const gone = isApiError(e) && ['NO_TENANT_MEMBERSHIP', 'ORGANIZATION_SUSPENDED', 'ORGANIZATION_ACCESS_EXPIRED'].includes(e.code);
        if (!gone) throw e;
        await secure.remove(StorageKeys.organization);
        if (preferred) throw e;
      }
    }
    setState({ status: 'selectOffice', me, offices, office: null, officeSlug: null, access });
  }, []);

  // Açılışta kayıtlı oturum: yalnızca reddedilen oturum (401) çıkış yapar;
  // bağlantı yoksa oturum korunur ve uygulama yeniden denemeyi önerir.
  const check = useCallback(async () => {
    try {
      await resolve(null);
    } catch (e) {
      if (launchFailure(e) === 'signedOut') {
        await session.clear();
        setState({ ...initial, status: 'signedOut' });
        return;
      }
      if (mounted.current) setState({ ...initial, status: 'offline' });
    }
  }, [resolve]);

  const retry = useCallback(async () => {
    setState({ ...initial, status: 'booting' });
    await check();
  }, [check]);

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
      await check();
    })();
    return () => {
      mounted.current = false;
      off();
    };
  }, [check]);

  const signIn = useCallback<AuthApi['signIn']>(
    async (input) => {
      const tokens = await request<Tokens>('POST', '/v1/auth/login', { body: input, anonymous: true });
      await session.setTokens(tokens);
      await resolve(null);
    },
    [resolve],
  );

  const selectOffice = useCallback<AuthApi['selectOffice']>(async (slug) => resolve(slug), [resolve]);

  const switchOffice = useCallback(() => {
    setState((s) => (s.me && s.offices.length ? { ...s, status: 'selectOffice' } : s));
  }, []);

  const reload = useCallback(async () => resolve(state.officeSlug), [resolve, state.officeSlug]);

  const signOut = useCallback(async () => {
    await unregisterPushDevice();
    const current = session.get();
    if (current) {
      await api.post('/v1/auth/logout', { refresh_token: current.refreshToken }).catch(() => {});
    }
    await secure.remove(StorageKeys.organization);
    await session.clear();
    setReportUser(null);
    setState({ ...initial, status: 'signedOut' });
  }, []);

  const can = useCallback((permission: string) => !!state.office?.permissions.includes(permission), [state.office]);

  const value = useMemo<AuthApi>(
    () => ({ ...state, signIn, selectOffice, switchOffice, reload, retry, signOut, can }),
    [state, signIn, selectOffice, switchOffice, reload, retry, signOut, can],
  );
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthApi {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error('useAuth outside AuthProvider');
  return ctx;
}

/** Açık ofisin `X-Organization` değeri; yalnızca `ready` ekranlarında çağrılır. */
export function useOrganization(): string {
  const { officeSlug } = useAuth();
  if (!officeSlug) throw new Error('useOrganization before an office is selected');
  return officeSlug;
}
