"use client";

import { ArrowRight, Play, Sparkles } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { DOC_TASKS_ANCHOR, routes } from "@/config/routes";
import { ALL_PERIODS, useOfficePeriod } from "@/features/office/period";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useEngagementWorkflow } from "../hooks/use-copilot";
import {
  ACTION_PERMISSIONS,
  actionHref,
  isWriteAction,
  stepHref,
} from "../lib/actions";
import {
  nextActionLabel,
  periodLabel,
  stepDetail,
  stepLabel,
} from "../lib/labels";
import { bulkSteps, nextStepOf } from "../lib/next-steps";
import { useOptionalCopilot } from "./copilot-provider";
import { StepChain } from "./step-chain";

/**
 * The co-pilot on a taxpayer's screens (#392): the period close chain of
 * this engagement and its next work, one click (and one approval) away.
 * `card` on the overview, `bar` above the bank, documents and report
 * screens.
 */
export function EngagementNextWork({
  engagement,
  variant = "bar",
  className,
}: {
  engagement: string;
  variant?: "card" | "bar";
  className?: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const router = useRouter();
  const copilot = useOptionalCopilot();
  const { choice, focus, setPeriod } = useOfficePeriod();
  // Sayılan dönem açılan ekranda da seçili olsun (#566).
  const pinPeriod = () => setPeriod(focus);
  const query = useEngagementWorkflow(slug, engagement, focus);

  if (query.isLoading) {
    return <Skeleton className={cn("h-14 w-full", className)} />;
  }
  // The chain is a help, not the page: on an error the screen goes on.
  if (!query.data) return null;
  const row = query.data;
  const step = nextStepOf(row);
  const write = !!step?.action && isWriteAction(step.action);
  const allowed =
    !!step?.action && ACTION_PERMISSIONS[step.action].every((p) => can(p));

  const run = () => {
    if (!step?.action) return;
    if (write) {
      copilot?.review(bulkSteps([row]));
      return;
    }
    // Belge istekleri aynı sayfadaysa oraya kaydır: adres aynı kaldığında
    // gezinme hiçbir şey yapmaz, kullanıcı tıklamanın boşa gittiğini sanır (#563).
    const anchor =
      step.action === "open_documents"
        ? document.getElementById(DOC_TASKS_ANCHOR)
        : null;
    if (anchor) {
      anchor.scrollIntoView({ behavior: "smooth", block: "start" });
      anchor.focus({ preventScroll: true });
      return;
    }
    // Mükellef sayfalarındaki kart: hedef mükellef sekmesinde açılır (#468).
    pinPeriod();
    router.push(actionHref(slug, step.action, engagement, true));
  };

  const next = step?.action ? (
    <div className="flex min-w-0 flex-wrap items-center gap-3">
      <div className="min-w-0">
        <p className="text-sm font-medium">{stepLabel(t, step.key)}</p>
        <p className="text-text-2 text-xs">
          {stepDetail(t, step.key, step.status, step.count)}
        </p>
      </div>
      <Button
        size="sm"
        variant={write ? "default" : "outline"}
        disabled={(write && !allowed) || (write && !copilot)}
        onClick={run}
      >
        {write ? (
          <Play aria-hidden className="size-3.5" />
        ) : (
          <ArrowRight aria-hidden className="size-3.5" />
        )}
        {nextActionLabel(t, step.action)}
      </Button>
    </div>
  ) : (
    <p className="text-ok text-sm">
      {row.steps.some((s) => s.status === "waiting")
        ? t("office.copilot.waiting_only")
        : t("office.copilot.closed")}
    </p>
  );

  const title = t("office.copilot.engagement_title", {
    period: periodLabel(locale, row.period),
  });
  // Liste "Tümü"yken zincir yine tek ayı sayar; "bu dönem hareket yok"
  // listede hareket görünürken çelişkili okunmasın diye hangi ayın
  // sayıldığı söylenir.
  const scope =
    choice === ALL_PERIODS ? (
      <p className="text-text-3 text-xs" data-testid="copilot-scope-note">
        {t("office.copilot.scope_all_note", {
          period: periodLabel(locale, row.period),
        })}
      </p>
    ) : null;

  if (variant === "card") {
    return (
      <section
        aria-labelledby="copilot-next-title"
        className={cn(
          "bg-surface border-line space-y-3 rounded-xl border p-4 md:p-5",
          className,
        )}
      >
        <div className="flex flex-wrap items-center justify-between gap-2">
          <h2
            id="copilot-next-title"
            className="font-display flex items-center gap-2 text-base font-bold"
          >
            <Sparkles aria-hidden className="text-brand size-4" />
            {title}
          </h2>
          <Link
            className="text-brand-text text-sm hover:underline"
            href={routes.tenant.copilot(slug)}
          >
            {t("office.copilot.all_taxpayers")}
          </Link>
        </div>
        <StepChain
          steps={row.steps}
          next={row.next_step}
          hrefFor={(k) => stepHref(slug, k, engagement, true)}
          onNavigate={pinPeriod}
        />
        <p className="text-text-3 text-xs tabular-nums">
          {t("office.copilot.progress", {
            done: row.done_steps,
            total: row.active_steps,
          })}
        </p>
        {next}
      </section>
    );
  }

  return (
    <section
      aria-label={title}
      className={cn(
        "bg-surface border-line flex flex-wrap items-center justify-between gap-3 rounded-xl border px-4 py-3",
        className,
      )}
    >
      <div className="flex min-w-0 flex-wrap items-center gap-3">
        <Link
          href={routes.tenant.copilot(slug)}
          className="flex items-center gap-1.5 text-sm font-semibold hover:underline"
        >
          <Sparkles aria-hidden className="text-brand size-4" />
          {title}
        </Link>
        <StepChain
          steps={row.steps}
          next={row.next_step}
          compact
          hrefFor={(k) => stepHref(slug, k, engagement, true)}
          onNavigate={pinPeriod}
        />
        {scope}
      </div>
      {next}
    </section>
  );
}
