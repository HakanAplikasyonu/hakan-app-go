import { cleanWidgetId, type SupportIdentity } from '@hakan/mobile-core/support/clickfix';
import { useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import { useAuth } from '@/core/auth/auth-provider';
import { env } from '@/core/config/env';

type ClickFixIdentity = { available: boolean; user_id?: string; user_hash?: string; company_id?: string };

/** Platform (yazılım sağlayıcısı) widget'ı; boşsa destek ekranı yok (#387). */
export const providerWidgetId = cleanWidgetId(env.clickfixWidgetId);

/**
 * Ofis kullanıcısının doğrulanmış ClickFix kimliği: sunucu
 * CLICKFIX_IDENTITY_SECRET ile imzalar; anahtar yoksa null (doğrulanmamış).
 */
export function useSupportIdentity() {
  const { officeSlug, office } = useAuth();
  const q = useQuery({
    queryKey: ['support', 'identity', officeSlug],
    queryFn: () => api.get<ClickFixIdentity>('/v1/tenant/support/clickfix-identity', { organization: officeSlug! }),
    enabled: !!officeSlug && !!providerWidgetId,
    staleTime: 30 * 60_000,
    retry: false,
  });
  const d = q.data;
  const identity: SupportIdentity | null =
    d?.available && d.user_id && d.user_hash
      ? {
          user_id: d.user_id,
          user_hash: d.user_hash,
          ...(d.company_id ? { company: { id: d.company_id, name: office?.organization.name } } : {}),
        }
      : null;
  return { loading: q.isPending && q.fetchStatus !== 'idle', identity };
}
