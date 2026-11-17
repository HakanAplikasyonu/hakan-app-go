"use client";

import { useQuery } from "@tanstack/react-query";
import { useMemo } from "react";

import { ClickFixWidget, useClickFixEnv } from "@/components/support";
import {
  supportKeys,
  supportService,
} from "@/features/tenant/services/support.service";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { compactAttributes, resolveWidgetId } from "@/lib/support/clickfix";
import { useAuth } from "@/providers/auth-provider";

/**
 * Ofis paneli destek kanalı (#387): ofisin talebi yazılım sağlayıcısına
 * gider (platform widget'ı). Kullanıcı adı/e-postası ve ofis bilgisi
 * ziyaretçi verisi olarak geçer; CLICKFIX_IDENTITY_SECRET tanımlıysa
 * kullanıcı sunucuda imzalanır (user_hash) ve ClickFix'te doğrulanmış görünür.
 */
export function OfficeSupportWidget() {
  const { user } = useAuth();
  const { slug, organization, role } = useTenant();
  const env = useClickFixEnv();
  const enabled = Boolean(user && resolveWidgetId("provider", env));
  const identity = useQuery({
    queryKey: supportKeys.identity(slug),
    queryFn: () => supportService.identity(slug),
    enabled,
    staleTime: 30 * 60_000,
    retry: false,
    meta: { silent: true },
  });
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
  const verified = identity.data;
  const signed = useMemo(
    () =>
      verified?.available && verified.user_id && verified.user_hash
        ? {
            user_id: verified.user_id,
            user_hash: verified.user_hash,
            company: verified.company_id
              ? { id: verified.company_id, name: organization.name }
              : undefined,
          }
        : null,
    [verified, organization.name],
  );
  if (!enabled || identity.isPending) return null;
  return (
    <ClickFixWidget channel="provider" visitor={visitor} identity={signed} />
  );
}
