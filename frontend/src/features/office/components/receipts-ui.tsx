"use client";

import type { LucideIcon } from "lucide-react";
import { X } from "lucide-react";
import { type ReactNode, useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type { ReceiptImportResult } from "../services/receipts.service";

/** Masraf ve Z ekranlarının üst özet kartları. */
export function SummaryCards({
  cards,
}: {
  cards: { label: string; value: ReactNode; icon: LucideIcon; box: string }[];
}) {
  return (
    <div
      className={cn(
        "grid gap-3",
        cards.length > 3 ? "sm:grid-cols-2 lg:grid-cols-4" : "sm:grid-cols-3",
      )}
    >
      {cards.map((c) => (
        <div
          key={c.label}
          className="bg-surface border-line flex items-center gap-3.5 rounded-xl border p-4"
        >
          <span
            aria-hidden
            className={cn(
              "flex size-10 shrink-0 items-center justify-center rounded-lg",
              c.box,
            )}
          >
            <c.icon className="size-5" />
          </span>
          <span className="flex min-w-0 flex-col">
            <span className="font-display truncate text-xl font-bold tabular-nums">
              {c.value}
            </span>
            <span className="text-text-2 text-sm">{c.label}</span>
          </span>
        </div>
      ))}
    </div>
  );
}

/** Gizli dosya seçici + yükleme durumu (Excel / CSV içe aktarma). */
export function useImportFile(
  upload: (file: File) => Promise<ReceiptImportResult>,
  onDone: () => void,
) {
  const { t } = useLocale();
  const ref = useRef<HTMLInputElement>(null);
  const [pending, setPending] = useState(false);
  const [result, setResult] = useState<ReceiptImportResult | null>(null);
  const input = (
    <input
      ref={ref}
      type="file"
      accept=".xlsx,.csv,text/csv,application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
      className="hidden"
      aria-hidden
      tabIndex={-1}
      onChange={(e) => {
        const file = e.target.files?.[0];
        e.target.value = "";
        if (!file) return;
        setPending(true);
        upload(file)
          .then((r) => {
            setResult(r);
            onDone();
            toast.success(
              t("office.receipts.imported", {
                inserted: r.inserted,
                skipped: r.skipped,
              }),
            );
          })
          .catch((err) =>
            toast.error(
              isApiError(err)
                ? err.message
                : t("office.receipts.import_failed"),
            ),
          )
          .finally(() => setPending(false));
      }}
    />
  );
  return {
    input,
    open: () => ref.current?.click(),
    pending,
    result,
    clear: () => setResult(null),
  };
}

/** İçe aktarma sonucu: eklenen / atlanan ve satır numaralı hatalar. */
export function ImportResultNote({
  result,
  onClose,
}: {
  result: ReceiptImportResult | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  if (!result) return null;
  return (
    <section
      aria-live="polite"
      className={cn(
        "rounded-xl border p-4 text-sm",
        result.errors.length
          ? "border-warn/40 bg-warn-tint"
          : "border-line bg-surface",
      )}
    >
      <div className="flex items-start justify-between gap-3">
        <p className="font-medium">
          {t("office.receipts.import_summary", {
            rows: result.rows,
            inserted: result.inserted,
            skipped: result.skipped,
            errors: result.errors.length,
          })}
        </p>
        <Button
          size="sm"
          variant="ghost"
          aria-label={t("common.close")}
          onClick={onClose}
          className="max-md:min-h-11"
        >
          <X className="size-4" aria-hidden />
        </Button>
      </div>
      {result.errors.length ? (
        <ul className="text-text-2 mt-2 max-h-40 space-y-1 overflow-y-auto">
          {result.errors.map((e) => (
            <li key={`${e.line}-${e.message}`}>
              <span className="font-mono">
                {t("office.receipts.import_line", { line: e.line })}
              </span>{" "}
              {e.message}
            </li>
          ))}
        </ul>
      ) : null}
    </section>
  );
}
