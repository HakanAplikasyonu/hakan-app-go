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
export const BANK_STATEMENT_TRACKING_SETTING_KEY = "bank_statement_tracking";

/**
 * Ayarlar → Eksik ekstre takibi (Oğuz'un geri bildirimi): mükellefin banka
 * hesaplarında her kapanmış ay ekstre geldi mi, ana sayfadaki "Eksik
 * ekstreler" listesi ve ekstre yüklerken listede olmayan hesap için
 * "Listeye ekle" önerisi. Varsayılan açık; kapalıyken hesap listesi kalır.
 */
export function BankStatementTrackingSetting({
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
  const on = settings[BANK_STATEMENT_TRACKING_SETTING_KEY] !== false;
  return (
    <EntitySectionCard title={t("tenant.bank_statement_tracking_title")}>
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="bank-statement-tracking">
            {t("tenant.bank_statement_tracking_label")}
          </Label>
          <p className="text-text-2 text-sm">
            {on
              ? t("tenant.bank_statement_tracking_on")
              : t("tenant.bank_statement_tracking_off")}
          </p>
        </div>
        <Switch
          id="bank-statement-tracking"
          checked={on}
          disabled={!canWrite || !query.data || patch.isPending}
          onCheckedChange={(v) =>
            patch.mutate({
              settings: { ...settings, [BANK_STATEMENT_TRACKING_SETTING_KEY]: v },
            })
          }
        />
      </div>
    </EntitySectionCard>
  );
}
