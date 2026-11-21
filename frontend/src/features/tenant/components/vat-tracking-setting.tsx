"use client";

import { EntitySectionCard } from "@/components/entity";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  useSaveVATTrackingSettings,
  useVATTrackingSettings,
} from "@/features/office/hooks/use-vat-tracking";
import { useLocale } from "@/providers/locale-provider";

const DAYS = Array.from({ length: 31 }, (_, i) => i + 1);

/**
 * Ayarlar → KDV takibi (#780 B): ay sonu KDV raporu (tüm mükellefler;
 * alınan, satılan, devreden; varsayılan ayın 29'u) ve KDV nöbetçisi
 * (her sabah "durum değişti" / "eşik aşıldı"). İkisi de varsayılan açık;
 * ofis kapatabilir.
 */
export function VATTrackingSetting({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const query = useVATTrackingSettings(slug);
  const save = useSaveVATTrackingSettings(slug);
  const on = query.data?.report_enabled !== false;
  const day = query.data?.report_day ?? 29;
  const watch = query.data?.watch_enabled !== false;
  const busy = !canWrite || !query.data || save.isPending;
  return (
    <EntitySectionCard title={t("tenant.vat_report_title")}>
      <div className="space-y-4">
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="vat-report-enabled">
              {t("tenant.vat_report_label")}
            </Label>
            <p className="text-text-2 text-sm">
              {on ? t("tenant.vat_report_on") : t("tenant.vat_report_off")}
            </p>
          </div>
          <Switch
            id="vat-report-enabled"
            checked={on}
            disabled={busy}
            onCheckedChange={(v) =>
              save.mutate({
                report_enabled: v,
                report_day: day,
                watch_enabled: watch,
              })
            }
          />
        </div>
        <div className="max-w-xs space-y-1.5">
          <Label htmlFor="vat-report-day">{t("tenant.vat_report_day")}</Label>
          <Select
            value={String(day)}
            onValueChange={(v) =>
              save.mutate({
                report_enabled: on,
                report_day: Number(v),
                watch_enabled: watch,
              })
            }
            disabled={busy || !on}
          >
            <SelectTrigger id="vat-report-day">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {DAYS.map((d) => (
                <SelectItem key={d} value={String(d)}>
                  {t(
                    d > 28
                      ? "tenant.vat_report_day_option_late"
                      : "tenant.vat_report_day_option",
                    { day: d },
                  )}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="flex items-start justify-between gap-4 border-t pt-4">
          <div className="space-y-1">
            <Label htmlFor="vat-watch-enabled">
              {t("tenant.vat_watch_label")}
            </Label>
            <p className="text-text-2 text-sm">
              {watch ? t("tenant.vat_watch_on") : t("tenant.vat_watch_off")}
            </p>
          </div>
          <Switch
            id="vat-watch-enabled"
            checked={watch}
            disabled={busy}
            onCheckedChange={(v) =>
              save.mutate({
                report_enabled: on,
                report_day: day,
                watch_enabled: v,
              })
            }
          />
        </div>
      </div>
    </EntitySectionCard>
  );
}
