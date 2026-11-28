"use client";

import { AccountProfilePage } from "@/features/account/components/account-profile-page";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";

/**
 * `/t/{slug}/profil`: profil ofis kenar çubuğu ve üst çubuğuyla (#890 C);
 * özet kartında ofis ve üyelik rolü, "Yetkilerim"de üyeliğin izinleri (#890 D).
 */
export default function TenantProfileRoute() {
  const { slug, organization, role, permissions } = useTenant();
  return (
    <AccountProfilePage
      home={routes.tenant.home(slug)}
      scope={{
        kind: "office",
        name: organization.name,
        role: role.name,
        permissions,
      }}
    />
  );
}
