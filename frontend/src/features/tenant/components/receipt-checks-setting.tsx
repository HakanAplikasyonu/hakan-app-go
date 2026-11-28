"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";

import { EntitySectionCard } from "@/components/entity";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { useWarningRules } from "@/features/office/hooks/use-warnings";
import {
  warningKeys,
  warningService,
} from "@/features/office/services/warnings.service";
import { useLocale } from "@/providers/locale-provider";

/** Fiş okunurken açılan uyarılar (backend receipts usecase/checks.go). */
export const RECEIPT_CHECK_CODES = [
  "RECEIPT_BUYER_MISMATCH",
  "PLATE_NOT_REGISTERED",
  "PLATE_UNREADABLE",
] as const;

/**
 * Ayarlar → Belgeler: fiş yapay zekâ kontrolleri (Oğuz'un geri bildirimi).
 * Açıkken fiş okunurken alıcı ve plaka denetlenir; mükellef önizlemede,
 * ofis onay ekranında rozet görür. Anahtar üç uyarı kodunu ofis katmanında
 * açıp kapatır (uyarı kuralları); mükellef bazında ayrı kural kalabilir.
 * Varsayılan açık.
 */
export function ReceiptChecksSetting({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const rules = useWarningRules(slug);
  const mine = (rules.data?.items ?? []).filter((r) =>
    (RECEIPT_CHECK_CODES as readonly string[]).includes(r.code),
  );
  const save = useMutation({
    mutationFn: (enabled: boolean) =>
      Promise.all(
        mine.map((r) => warningService.setRuleEnabled(slug, r, enabled)),
      ),
    onSettled: () =>
      qc.invalidateQueries({ queryKey: warningKeys.rules(slug) }),
  });
  const on =
    mine.length === 0 || mine.some((r) => r.office?.enabled !== false);
  return (
    <EntitySectionCard title={t("tenant.receipt_checks_title")}>
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="receipt-checks-enabled">
            {t("tenant.receipt_checks_label")}
          </Label>
          <p className="text-text-2 text-sm">
            {on
              ? t("tenant.receipt_checks_on")
              : t("tenant.receipt_checks_off")}
          </p>
        </div>
        <Switch
          id="receipt-checks-enabled"
          data-testid="receipt-checks-setting"
          checked={on}
          disabled={!canWrite || !mine.length || save.isPending}
          onCheckedChange={(v) => save.mutate(v)}
        />
      </div>
    </EntitySectionCard>
  );
}
