"use client";

import { EntitySectionCard } from "@/components/entity";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  useSaveTaxReturnSettings,
  useTaxReturnSettings,
} from "@/features/office/hooks/use-tax-return-settings";
import { useLocale } from "@/providers/locale-provider";

/**
 * Ayarlar → Beyan (#710, ofis kararı A.5): sorumlu müşavirin son kontrolü.
 * Varsayılan açık: "kontrol edildi" ve "verildi" yalnız sorumlu (sahip,
 * yönetici). Kapalıyken beyanı hazırlayan da işaretler; kim / ne zaman yine
 * kaydedilir.
 */
export function TaxReturnFinalCheckSetting({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const query = useTaxReturnSettings(slug);
  const save = useSaveTaxReturnSettings(slug);
  const on = query.data?.final_check_required !== false;
  return (
    <EntitySectionCard title={t("tenant.final_check_title")}>
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="tax-return-final-check">
            {t("tenant.final_check_label")}
          </Label>
          <p className="text-text-2 text-sm">
            {on ? t("tenant.final_check_on") : t("tenant.final_check_off")}
          </p>
        </div>
        <Switch
          id="tax-return-final-check"
          checked={on}
          disabled={!canWrite || !query.data || save.isPending}
          onCheckedChange={(v) => save.mutate(v)}
        />
      </div>
    </EntitySectionCard>
  );
}
