"use client";

import { AccountProfilePage } from "@/features/account/components/account-profile-page";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";

/** `/t/{slug}/profil`: profil ofis kenar çubuğu ve üst çubuğuyla (#890 C). */
export default function TenantProfileRoute() {
  const { slug } = useTenant();
  return <AccountProfilePage home={routes.tenant.home(slug)} />;
}
