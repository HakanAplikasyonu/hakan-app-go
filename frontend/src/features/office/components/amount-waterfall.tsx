"use client";

import { motion } from "motion/react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { absDiff, formatAmount, fromMinor, toMinor } from "../lib/decimal";
import type { Invoice } from "../types";
import { Figure } from "./office-ui";

type Step = {
  label: string;
  delta: bigint;
  kind: "base" | "plus" | "minus" | "total";
};

/**
 * Tutar şelalesi: Matrah + vergiler − tevkifat = hesaplanan ödenecek, yanında
 * faturada yazan ödenecek. Tutmuyorsa son sütun farkıyla kırmızı yanar;
 * TAX/PAYABLE_MISMATCH uyarıları kendini açıklar.
 */
export function AmountWaterfall({ invoice }: { invoice: Invoice }) {
  const { t } = useLocale();
  const steps: Step[] = [
    {
      label: t("office.col_base"),
      delta: toMinor(invoice.totals.taxExclusive),
      kind: "base",
    },
  ];
  for (const tx of invoice.taxes) {
    const isWithholding = tx.code.startsWith("9") || tx.code === "4171";
    const name = t(`office.tax_code.${tx.code}`);
    steps.push({
      label: `${name === `office.tax_code.${tx.code}` ? tx.code : name} %${tx.percent}`,
      delta: isWithholding ? -toMinor(tx.amount) : toMinor(tx.amount),
      kind: isWithholding ? "minus" : "plus",
    });
  }
  const computed = steps.reduce((a, s) => a + s.delta, BigInt(0));
  const declared = toMinor(invoice.totals.payable);
  const off = absDiff(fromMinor(computed), invoice.totals.payable) > BigInt(5);

  const bars: {
    label: string;
    start: bigint;
    end: bigint;
    kind: Step["kind"];
    value: bigint;
    alert?: boolean;
  }[] = [];
  let run = BigInt(0);
  for (const s of steps) {
    const start = s.kind === "base" ? BigInt(0) : run;
    run += s.delta;
    bars.push({
      label: s.label,
      start,
      end: run,
      kind: s.kind,
      value: s.delta,
    });
  }
  bars.push({
    label: t("office.waterfall.computed"),
    start: BigInt(0),
    end: computed,
    kind: "total",
    value: computed,
  });
  bars.push({
    label: t("office.waterfall.declared"),
    start: BigInt(0),
    end: declared,
    kind: "total",
    value: declared,
    alert: off,
  });

  const max = bars.reduce(
    (m, b) => (b.end > m ? b.end : b.start > m ? b.start : m),
    BigInt(1),
  );
  const pct = (v: bigint) => (Number(v) / Number(max)) * 100;
  const color = {
    base: "bg-foreground/70",
    plus: "bg-status-warning",
    minus: "bg-brand",
    total: "bg-status-approved",
  };

  return (
    <div className="space-y-2 p-4">
      {bars.map((b, i) => {
        const lo = b.start < b.end ? b.start : b.end;
        const hi = b.start < b.end ? b.end : b.start;
        return (
          <div
            key={`${b.label}-${i}`}
            className="grid grid-cols-[minmax(0,9rem)_minmax(0,1fr)_auto] items-center gap-3"
          >
            <span
              className={cn(
                "truncate text-xs",
                b.kind === "total" ? "font-medium" : "text-muted-foreground",
              )}
            >
              {b.label}
            </span>
            <div className="bg-muted/60 relative h-5 overflow-hidden rounded">
              <motion.div
                className={cn(
                  "absolute inset-y-0 rounded",
                  b.alert ? "bg-status-blocking" : color[b.kind],
                )}
                initial={{ left: `${pct(lo)}%`, width: 0 }}
                animate={{
                  left: `${pct(lo)}%`,
                  width: `${Math.max(pct(hi) - pct(lo), 0.6)}%`,
                }}
                transition={{
                  duration: 0.6,
                  delay: 0.08 * i,
                  ease: [0.22, 1, 0.36, 1],
                }}
              />
            </div>
            <Figure
              className={cn(
                "text-right text-xs whitespace-nowrap",
                b.kind === "total" && "font-semibold",
                b.alert && "text-status-blocking",
              )}
            >
              {b.kind === "minus" ? "−" : b.kind === "plus" ? "+" : ""}
              {formatAmount(
                fromMinor(b.value < BigInt(0) ? -b.value : b.value),
              )}
            </Figure>
          </div>
        );
      })}
      <p
        className={cn(
          "pt-1 text-xs",
          off ? "text-status-blocking" : "text-status-approved",
        )}
      >
        {off
          ? t("office.waterfall.off", {
              diff: formatAmount(fromMinor(computed - declared)),
            })
          : t("office.waterfall.ok")}
      </p>
    </div>
  );
}
