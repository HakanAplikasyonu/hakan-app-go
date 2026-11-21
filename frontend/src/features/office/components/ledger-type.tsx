"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useId, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { usePeriodMatrix } from "../hooks/use-office-data";
import {
  ENGAGEMENT_MONTHS,
  LEDGER_TYPES,
  isSingleEntry,
  type LedgerType,
} from "../lib/ledger-type";
import { bookkeepingService } from "../services/bookkeeping.service";

/**
 * Mükellefin defter türü (#780). Mükellef başlığıyla aynı matris sorgusundan
 * (önbellekten) okunur; satır yoksa bilanço sayılır.
 */
export function useLedgerType(engagement: string | null | undefined): {
  ledgerType: LedgerType;
  singleEntry: boolean;
  isLoading: boolean;
} {
  const matrix = usePeriodMatrix(ENGAGEMENT_MONTHS);
  const row = engagement
    ? matrix.data?.items.find((r) => r.engagement_uuid === engagement)
    : undefined;
  const ledgerType: LedgerType = row?.ledger_type ?? "bilanco";
  return {
    ledgerType,
    singleEntry: isSingleEntry(ledgerType),
    isLoading: matrix.isLoading,
  };
}

/** Başlıktaki defter türü rozeti. */
export function LedgerBadge({ ledgerType }: { ledgerType: LedgerType }) {
  const { t } = useLocale();
  return (
    <span
      data-testid="ledger-badge"
      className={cn(
        "rounded-md px-2 py-0.5 text-xs font-semibold whitespace-nowrap",
        isSingleEntry(ledgerType)
          ? "bg-warn-tint text-warn"
          : "bg-brand-tint text-brand-text",
      )}
    >
      {t(`office.bookkeeping.ledger_${ledgerType}`)}
    </span>
  );
}

/**
 * Mükellef ayarı: defter türü. Genel bakışta; yazma izni olmayan yalnız
 * görür.
 */
export function LedgerTypeCard({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const id = useId();
  const { ledgerType } = useLedgerType(engagement);
  const [draft, setDraft] = useState<LedgerType | null>(null);
  const value = draft ?? ledgerType;
  const canWrite = can(permissions.tenant.engagements.write);
  const save = useMutation({
    mutationFn: (v: LedgerType) =>
      bookkeepingService.setLedgerType(slug, engagement, v),
    onSuccess: async () => {
      setDraft(null);
      await qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase(), "periods"],
      });
      void qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase(), "operating-account"],
      });
      toast.success(t("office.bookkeeping.ledger_saved"));
    },
    onError: (e) =>
      toast.error(
        isApiError(e) ? e.message : t("office.bookkeeping.ledger_failed"),
      ),
  });
  return (
    <section
      aria-labelledby={`${id}-title`}
      className="bg-surface border-line rounded-xl border p-4 md:p-5"
      data-testid="ledger-type-card"
    >
      <h2 id={`${id}-title`} className="font-display mb-1 text-base font-bold">
        {t("office.bookkeeping.settings_title")}
      </h2>
      <p className="text-text-2 mb-3 text-sm">
        {t("office.bookkeeping.settings_hint")}
      </p>
      <div className="flex flex-wrap items-end gap-3">
        <div className="space-y-1">
          <Label htmlFor={`${id}-ledger`}>
            {t("office.bookkeeping.ledger_label")}
          </Label>
          <NativeSelect
            id={`${id}-ledger`}
            value={value}
            disabled={!canWrite || save.isPending}
            onChange={(e) => setDraft(e.target.value as LedgerType)}
          >
            {LEDGER_TYPES.map((x) => (
              <NativeSelectOption key={x} value={x}>
                {t(`office.bookkeeping.ledger_${x}`)}
              </NativeSelectOption>
            ))}
          </NativeSelect>
        </div>
        {canWrite ? (
          <Button
            onClick={() => save.mutate(value)}
            disabled={value === ledgerType || save.isPending}
          >
            {t("office.bookkeeping.ledger_save")}
          </Button>
        ) : null}
      </div>
      <p className="text-text-3 mt-2 text-xs">
        {t(`office.bookkeeping.ledger_${value}_hint`)}
      </p>
    </section>
  );
}
