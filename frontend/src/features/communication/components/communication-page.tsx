"use client";

import { useRouter, useSearchParams } from "next/navigation";
import { useState } from "react";

import { EntityPage } from "@/components/entity";
import { TabStrip, TabStripItem } from "@/components/ui/tab-strip";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { TenantForbidden } from "@/features/tenant/components/tenant-forbidden";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import type { BulkSendTarget } from "../services/communication.service";
import { BulkSendTab } from "./bulk-send-tab";
import { GroupsTab } from "./groups-tab";

/**
 * Sekmeler: diğer bölümler (otomasyonlar, gönderim geçmişi) kendi
 * sekmelerini buraya ekler. Varsayılan sekme adreste yazmaz.
 */
const TABS = ["bulk", "groups"] as const;
type Tab = (typeof TABS)[number];

/**
 * `/t/{slug}/communication` (#780 B): İletişim sayfası. "Toplu gönderim"
 * sekmesinde bir grup, süzgeç ya da son gönderimlerden biri hedef seçilip
 * SMS / e-posta yazılır; "Gruplar" sekmesinde ofisin kayıtlı mükellef
 * listeleri yönetilir.
 */
export function CommunicationPage() {
  const { t } = useLocale();
  const tenant = useTenant();
  const router = useRouter();
  const params = useSearchParams();
  const raw = params.get("tab");
  const tab: Tab = TABS.includes(raw as Tab) ? (raw as Tab) : "bulk";
  const [preset, setPreset] = useState<BulkSendTarget | null>(null);

  const go = (next: Tab) => {
    const q = new URLSearchParams(params.toString());
    if (next === "bulk") q.delete("tab");
    else q.set("tab", next);
    const s = q.toString();
    router.replace(
      `${routes.tenant.communication(tenant.slug)}${s ? `?${s}` : ""}`,
    );
  };

  return (
    <EntityPage
      title={t("communication.title")}
      description={t("communication.description")}
      permission={permissions.tenant.engagements.read}
      forbiddenFallback={<TenantForbidden />}
      breadcrumbs={[
        {
          label: tenant.organization.name,
          href: routes.tenant.home(tenant.slug),
        },
        { label: t("communication.title") },
      ]}
    >
      <div className="flex min-w-0 flex-col gap-4">
        <TabStrip aria-label={t("communication.tabs_label")}>
          {TABS.map((x) => (
            <TabStripItem
              key={x}
              selected={tab === x}
              onClick={() => go(x)}
              data-testid={`communication-tab-${x}`}
            >
              {t(`communication.tab_${x}`)}
            </TabStripItem>
          ))}
        </TabStrip>
        {tab === "bulk" ? (
          <BulkSendTab preset={preset} />
        ) : (
          <GroupsTab
            onSend={(group) => {
              setPreset({ kind: "group", group_uuid: group });
              go("bulk");
            }}
          />
        )}
      </div>
    </EntityPage>
  );
}
