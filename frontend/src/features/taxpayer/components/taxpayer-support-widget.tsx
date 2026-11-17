"use client";

import { useMemo } from "react";

import { ClickFixWidget } from "@/components/support";
import type { TaxpayerContextResponse } from "@/features/taxpayer/services/taxpayer.service";
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
  const visitor = useMemo(
    () => ({
      name: user?.fullName || undefined,
      email: user?.email || undefined,
      attributes: compactAttributes({
        portal: "mukellef",
        mukellef: taxpayer.title,
        mukellef_id: taxpayer.uuid,
        rol: context.role,
      }),
    }),
    [user?.fullName, user?.email, taxpayer, context.role],
  );
  if (!user) return null;
  return <ClickFixWidget channel="taxpayer" visitor={visitor} />;
}
