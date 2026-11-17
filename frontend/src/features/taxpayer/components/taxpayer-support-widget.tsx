"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { ClickFixWidget } from "@/components/support";
import {
  fetchTaxpayerSupport,
  taxpayerSupportKeys,
  type TaxpayerContextResponse,
} from "@/features/taxpayer/services/taxpayer.service";
import { compactAttributes } from "@/lib/support/clickfix";
import { useAuth } from "@/providers/auth-provider";

/**
 * Mükellef portalı destek kanalı (#387): mükellefin talebi kendi mali
 * müşavirlik ofisine gider. Ofis ayarında widget kimliği yoksa platform
 * varsayılanı (NEXT_PUBLIC_CLICKFIX_TAXPAYER_WIDGET_ID); o da boşsa yüklenmez.
 */
export function TaxpayerSupportWidget({
  context,
}: {
  context: TaxpayerContextResponse;
}) {
  const { user } = useAuth();
  const { taxpayer } = context;
  const support = useQuery({
    queryKey: taxpayerSupportKeys.support(taxpayer.uuid),
    queryFn: () => fetchTaxpayerSupport(taxpayer.uuid),
    staleTime: 5 * 60_000,
    retry: false,
    meta: { silent: true },
  });
  const office = support.data?.office ?? null;
  const visitor = useMemo(
    () => ({
      name: user?.fullName || undefined,
      email: user?.email || undefined,
      attributes: compactAttributes({
        portal: "mukellef",
        mukellef: taxpayer.title,
        mukellef_id: taxpayer.uuid,
        ofis: office?.name,
        rol: context.role,
      }),
    }),
    [user?.fullName, user?.email, taxpayer, office?.name, context.role],
  );
  // Ofis kimliği gelmeden platform varsayılanıyla açıp kapatmamak için
  // sorgunun bitmesi beklenir (hata olursa varsayılan kullanılır).
  if (!user || support.isPending) return null;
  return (
    <ClickFixWidget
      channel="taxpayer"
      officeWidgetId={office?.clickfix_widget_id}
      visitor={visitor}
    />
  );
}
