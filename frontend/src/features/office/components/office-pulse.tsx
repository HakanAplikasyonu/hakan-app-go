"use client";

import { ArrowDownRight, ArrowUpRight, Compass } from "lucide-react";
import { motion } from "motion/react";
import Link from "next/link";

import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { toMinor } from "../lib/decimal";
import { vatCompass } from "../lib/vat";
import type { DataWarning, Invoice, Taxpayer } from "../types";
import { Figure, Money } from "./office-ui";

type Stage = "approved" | "in_review" | "blocked";

function stageOf(inv: Invoice, warnings: DataWarning[]): Stage {
  if (
    warnings.some(
      (w) =>
        w.entityId === inv.id &&
        w.status === "open" &&
        w.severity === "blocker",
    )
  )
    return "blocked";
  return inv.status === "approved" ? "approved" : "in_review";
}

const stageClass: Record<Stage, string> = {
  approved: "bg-status-approved",
  in_review: "bg-status-review",
  blocked: "bg-status-blocking",
};

/** Mükellef başına akış çubuğu: onaylı / incelemede / engelli faturalar. */
function FlowBar({
  invoices,
  warnings,
}: {
  invoices: Invoice[];
  warnings: DataWarning[];
}) {
  const { t } = useLocale();
  const total = invoices.length || 1;
  const counts: Record<Stage, number> = {
    approved: 0,
    in_review: 0,
    blocked: 0,
  };
  for (const inv of invoices) counts[stageOf(inv, warnings)]++;
  const order: Stage[] = ["approved", "in_review", "blocked"];
  return (
    <div className="space-y-1.5">
      <div
        className="bg-muted flex h-2.5 overflow-hidden rounded-full"
        role="img"
        aria-label={order
          .map((s) => `${t(`office.pulse.stage_${s}`)} ${counts[s]}`)
          .join(", ")}
      >
        {order.map((s, i) =>
          counts[s] ? (
            <motion.div
              key={s}
              className={cn(
                "h-full",
                stageClass[s],
                i > 0 && "border-card border-l-2",
              )}
              initial={{ width: 0 }}
              animate={{ width: `${(counts[s] / total) * 100}%` }}
              transition={{
                duration: 0.7,
                delay: 0.1 * i,
                ease: [0.22, 1, 0.36, 1],
              }}
            />
          ) : null,
        )}
      </div>
      <div className="text-muted-foreground flex gap-3 text-[11px]">
        {order.map((s) => (
          <span key={s} className="inline-flex items-center gap-1">
            <span className={cn("size-1.5 rounded-full", stageClass[s])} />
            {t(`office.pulse.stage_${s}`)} <Figure>{counts[s]}</Figure>
          </span>
        ))}
      </div>
    </div>
  );
}

/** Hesaplanan ve indirilecek KDV'yi karşılıklı iki kol olarak çizer. */
function VatScale({ output, input }: { output: string; input: string }) {
  const o = Number(toMinor(output));
  const i = Number(toMinor(input));
  const max = Math.max(o, i, 1);
  return (
    <div className="grid grid-cols-2 gap-1" aria-hidden>
      <div className="bg-muted flex h-1.5 justify-end overflow-hidden rounded-l-full">
        <motion.div
          className="bg-brand h-full rounded-l-full"
          initial={{ width: 0 }}
          animate={{ width: `${(i / max) * 100}%` }}
          transition={{ duration: 0.8, ease: [0.22, 1, 0.36, 1] }}
        />
      </div>
      <div className="bg-muted h-1.5 overflow-hidden rounded-r-full">
        <motion.div
          className="bg-status-warning h-full rounded-r-full"
          initial={{ width: 0 }}
          animate={{ width: `${(o / max) * 100}%` }}
          transition={{ duration: 0.8, ease: [0.22, 1, 0.36, 1] }}
        />
      </div>
    </div>
  );
}

/**
 * Dönem nabzı: her mükellef için faturaların akışı ve tahmini KDV pusulası.
 * Müşavirin "bu ay kim ne durumda, kim KDV ödeyecek" sorusuna tek bakışta yanıt.
 */
export function OfficePulse({
  taxpayers,
  invoices,
  warnings,
}: {
  taxpayers: Taxpayer[];
  invoices: Invoice[];
  warnings: DataWarning[];
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  return (
    <div className="grid gap-3 md:grid-cols-2">
      {taxpayers.map((tp, idx) => {
        const own = invoices.filter((i) => i.taxpayerId === tp.id);
        const vat = vatCompass(own);
        const pays = toMinor(vat.net) > BigInt(0);
        return (
          <motion.article
            key={tp.id}
            initial={{ opacity: 0, y: 12 }}
            animate={{ opacity: 1, y: 0 }}
            transition={{
              delay: 0.08 * idx,
              type: "spring",
              stiffness: 200,
              damping: 24,
            }}
            className="bg-card group relative overflow-hidden rounded-xl border p-4"
          >
            <div
              className={cn(
                "pointer-events-none absolute -top-16 -right-16 size-40 rounded-full opacity-[0.07] blur-2xl transition-opacity group-hover:opacity-[0.14]",
                pays ? "bg-status-warning" : "bg-brand",
              )}
              aria-hidden
            />
            <header className="mb-3 flex items-start justify-between gap-2">
              <div className="min-w-0">
                <Link
                  href={`${routes.tenant.invoices(slug)}?taxpayer=${tp.id}`}
                  className="block truncate font-semibold hover:underline"
                >
                  {tp.title}
                </Link>
                <Figure className="text-muted-foreground text-xs">
                  {tp.taxId}
                </Figure>
              </div>
              <Figure className="text-muted-foreground text-xs whitespace-nowrap">
                {t("office.pulse.invoice_count", { count: own.length })}
              </Figure>
            </header>
            <FlowBar invoices={own} warnings={warnings} />
            <div className="mt-4 rounded-lg border border-dashed p-3">
              <div className="text-muted-foreground mb-2 flex items-center gap-1.5 text-[11px] tracking-wide uppercase">
                <Compass className="size-3.5" aria-hidden />
                {t("office.pulse.vat_title")}
              </div>
              <div className="mb-2 grid grid-cols-2 gap-2 text-xs">
                <div>
                  <div className="text-muted-foreground">
                    {t("office.pulse.vat_input")}
                  </div>
                  <Money value={vat.input} className="text-brand font-medium" />
                </div>
                <div className="text-right">
                  <div className="text-muted-foreground">
                    {t("office.pulse.vat_output")}
                  </div>
                  <Money
                    value={vat.output}
                    className="text-status-warning font-medium"
                  />
                </div>
              </div>
              <VatScale output={vat.output} input={vat.input} />
              <div className="mt-3 flex items-baseline justify-between gap-2">
                <span className="text-muted-foreground inline-flex items-center gap-1 text-xs">
                  {pays ? (
                    <ArrowUpRight className="size-3.5" />
                  ) : (
                    <ArrowDownRight className="size-3.5" />
                  )}
                  {pays
                    ? t("office.pulse.vat_payable")
                    : t("office.pulse.vat_carry")}
                </span>
                <Money
                  value={pays ? vat.net : vat.net.replace("-", "")}
                  className={cn(
                    "text-lg font-semibold",
                    pays ? "text-status-warning" : "text-brand",
                  )}
                />
              </div>
            </div>
          </motion.article>
        );
      })}
    </div>
  );
}
