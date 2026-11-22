"use client";

import { toast } from "sonner";

import { EntitySectionCard } from "@/components/entity";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Switch } from "@/components/ui/switch";
import {
  useExchangeRateSettings,
  useSaveExchangeRateSettings,
  type ExchangeRateKind,
} from "@/features/office/hooks/use-exchange-rates";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

const KINDS: ExchangeRateKind[] = [
  "forex_buying",
  "forex_selling",
  "banknote_buying",
  "banknote_selling",
];

/**
 * Ayarlar → Döviz kurları (#780): kuru eksik dövizli faturalar ve yabancı
 * hizmet faturaları, faturanın tarihinden önceki son TCMB bülteninden
 * otomatik kur alır. Ofis kur türünü seçer (varsayılan döviz alış) ya da
 * otomatik kuru kapatır. Elle girilen kur hiçbir zaman değişmez.
 */
export function ExchangeRateSetting({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const query = useExchangeRateSettings(slug);
  const save = useSaveExchangeRateSettings(slug);
  const on = query.data?.auto_fill !== false;
  const kind = query.data?.rate_kind ?? "forex_buying";
  const busy = !canWrite || !query.data || save.isPending;
  const fail = (err: unknown) =>
    toast.error(isApiError(err) ? err.message : t("tenant.fx_failed"));
  const done = () => toast.success(t("tenant.fx_saved"));

  return (
    <EntitySectionCard title={t("tenant.fx_title")}>
      <div className="space-y-4">
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="fx-auto-fill">{t("tenant.fx_label")}</Label>
            <p className="text-text-2 text-sm">
              {on ? t("tenant.fx_on") : t("tenant.fx_off")}
            </p>
          </div>
          <Switch
            id="fx-auto-fill"
            checked={on}
            disabled={busy}
            onCheckedChange={(v) =>
              save.mutate(
                { rate_kind: kind, auto_fill: v },
                { onSuccess: done, onError: fail },
              )
            }
          />
        </div>
        <div className="max-w-xs space-y-1.5">
          <Label htmlFor="fx-rate-kind">{t("tenant.fx_kind_label")}</Label>
          <NativeSelect
            id="fx-rate-kind"
            value={kind}
            disabled={busy || !on}
            onChange={(e) =>
              save.mutate(
                {
                  rate_kind: e.target.value as ExchangeRateKind,
                  auto_fill: on,
                },
                { onSuccess: done, onError: fail },
              )
            }
          >
            {KINDS.map((k) => (
              <NativeSelectOption key={k} value={k}>
                {t(`tenant.fx_kind_${k}`)}
              </NativeSelectOption>
            ))}
          </NativeSelect>
          <p className="text-text-3 text-xs">{t("tenant.fx_kind_hint")}</p>
        </div>
      </div>
    </EntitySectionCard>
  );
}
