"use client";

import { useMemo } from "react";

import { ClickFixWidget } from "@/components/support";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { compactAttributes } from "@/lib/support/clickfix";
import { useAuth } from "@/providers/auth-provider";

/**
 * Ofis paneli destek kanalı (#387): ofisin talebi yazılım sağlayıcısına
 * gider (platform widget'ı). Kullanıcı adı/e-postası ve ofis bilgisi
 * ziyaretçi verisi olarak geçer.
 */
export function OfficeSupportWidget() {
  const { user } = useAuth();
  const { organization, role } = useTenant();
  const visitor = useMemo(
    () => ({
      name: user?.fullName || undefined,
      email: user?.email || undefined,
      attributes: compactAttributes({
        portal: "ofis",
        ofis: organization.name,
        ofis_kodu: organization.slug,
        ofis_id: organization.uuid,
        rol: role.name,
      }),
    }),
    [user?.fullName, user?.email, organization, role.name],
  );
  if (!user) return null;
  return <ClickFixWidget channel="provider" visitor={visitor} />;
}
