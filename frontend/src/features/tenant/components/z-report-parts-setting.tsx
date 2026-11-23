"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { EntitySectionCard } from "@/components/entity";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  receiptsService,
  zPartsSettingsKey,
} from "@/features/office/services/receipts.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * Ayarlar → Belgeler: parçalı Z raporu (Oğuz'un demo geri bildirimi). Açıkken
 * uzun Z raporu birkaç fotoğrafla (parça 1/2/3) tek Z raporu olarak
 * yüklenir; kapalıyken yalnızca tek fotoğraf kabul edilir. Varsayılan açık.
 */
export function ZReportPartsSetting({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: zPartsSettingsKey(slug),
    queryFn: () => receiptsService.partsSettings(slug),
  });
  const save = useMutation({
    mutationFn: (enabled: boolean) =>
      receiptsService.savePartsSettings(slug, enabled),
    onSuccess: (data) => qc.setQueryData(zPartsSettingsKey(slug), data),
  });
  const on = query.data?.enabled !== false;
  return (
    <EntitySectionCard title={t("tenant.z_parts_title")}>
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="z-parts-enabled">{t("tenant.z_parts_label")}</Label>
          <p className="text-text-2 text-sm">
            {on ? t("tenant.z_parts_on") : t("tenant.z_parts_off")}
          </p>
        </div>
        <Switch
          id="z-parts-enabled"
          data-testid="z-parts-setting"
          checked={on}
          disabled={!canWrite || !query.data || save.isPending}
          onCheckedChange={(v) => save.mutate(v)}
        />
      </div>
    </EntitySectionCard>
  );
}
