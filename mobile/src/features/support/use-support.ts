import { cleanWidgetId } from '@hakan/mobile-core/support/clickfix';
import { useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import { useAuth } from '@/core/auth/auth-provider';
import { env } from '@/core/config/env';

type TaxpayerSupport = { office: { name: string; clickfix_widget_id: string } | null };

/**
 * Destek kanalı (#387): mükellefin talebi kendi mali müşavirlik ofisine
 * gider. Ofisin ClickFix widget kimliği (ofis ayarı), yoksa platform
 * varsayılanı; ikisi de yoksa widgetId null.
 */
export function useSupportChannel() {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid;
  const q = useQuery({
    queryKey: ['support', uuid],
    queryFn: () => api.get<TaxpayerSupport>('/v1/taxpayer/support', { taxpayer: uuid! }),
    enabled: !!uuid,
    staleTime: 5 * 60_000,
    retry: false,
  });
  const office = q.data?.office ?? null;
  const widgetId = cleanWidgetId(office?.clickfix_widget_id) ?? cleanWidgetId(env.clickfixTaxpayerWidgetId);
  return { loading: q.isPending && !!uuid, widgetId, officeName: office?.name ?? null };
}
