"use client";

import Link from "next/link";

import { YetkiYok } from "@/components/hakan";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

import { useTenant } from "../providers/tenant-provider";

/**
 * Ofis panelinde yetki 403 (#332, Durumlar.dc.html): rolün ve kimden
 * istemeli; tek eylem genel bakışa döner.
 */
export function TenantForbidden({ aciklama }: { aciklama?: string }) {
  const { t } = useLocale();
  const tenant = useTenant();
  return (
    <YetkiYok
      rol={tenant.role.name}
      aciklama={aciklama}
      eylem={
        <Button asChild variant="outline">
          <Link href={routes.tenant.home(tenant.slug)}>
            {t("common.state_back_home")}
          </Link>
        </Button>
      }
    />
  );
}
