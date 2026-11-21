"use client";

import { EntitySectionCard } from "@/components/entity";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  usePatchTenantSettings,
  useTenantSettings,
} from "@/features/tenant/hooks/use-tenant-admin";
import { useLocale } from "@/providers/locale-provider";

/** organizations.settings anahtarı; yoksa açık (backend ile aynı). */
export const TAXPAYER_VAT_PANEL_SETTING = "taxpayer_vat_sales_panel";

/**
 * Ayarlar → Mükellef paneli (#780): mükellef kendi panelinde bu ayın anlık
 * KDV durumunu ve satış raporunu görsün mü. Varsayılan açık; kapalıyken
 * bölüm mükellef panelinde görünmez.
 */
export function TaxpayerVatPanelSetting({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const query = useTenantSettings(slug);
  const patch = usePatchTenantSettings(slug);
  const settings = (query.data?.settings ?? {}) as Record<string, unknown>;
  const on = settings[TAXPAYER_VAT_PANEL_SETTING] !== false;
  return (
    <EntitySectionCard title={t("tenant.taxpayer_vat_panel_title")}>
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="taxpayer-vat-panel">
            {t("tenant.taxpayer_vat_panel_label")}
          </Label>
          <p className="text-text-2 text-sm">
            {on
              ? t("tenant.taxpayer_vat_panel_on")
              : t("tenant.taxpayer_vat_panel_off")}
          </p>
        </div>
        <Switch
          id="taxpayer-vat-panel"
          checked={on}
          disabled={!canWrite || !query.data || patch.isPending}
          onCheckedChange={(v) =>
            patch.mutate({
              settings: { ...settings, [TAXPAYER_VAT_PANEL_SETTING]: v },
            })
          }
        />
      </div>
    </EntitySectionCard>
  );
}
