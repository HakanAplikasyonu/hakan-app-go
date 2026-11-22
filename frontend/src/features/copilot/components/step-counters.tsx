"use client";

import { CircleCheck, Hourglass } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { stepLabel } from "../lib/labels";
import type {
  CopilotStepKey,
  CopilotWorkflowSummary,
} from "../services/copilot.service";
import { STEP_ICON } from "./step-chain";

/**
 * Ofis sayaçları (#780, ay sonu kapanış): seçili dönemde kaç mükellefin
 * sıradaki adımı hangisi ("12 mükellef Toplandı'da, 8'i Luca'ya
 * aktarımda"). Sayaca tıklayınca liste o adıma süzülür; yeniden
 * tıklayınca süzgeç kalkar.
 */
export function StepCounters({
  summary,
  active,
  onPick,
}: {
  summary: CopilotWorkflowSummary | undefined;
  active: CopilotStepKey | null;
  onPick: (key: CopilotStepKey) => void;
}) {
  const { t } = useLocale();
  if (!summary) return <Skeleton className="h-20 w-full" />;
  return (
    <section
      aria-label={t("office.copilot.counters_title")}
      data-testid="copilot-step-counters"
      className="space-y-2"
    >
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 className="font-display text-base font-bold">
          {t("office.copilot.counters_title")}
        </h2>
        <p className="text-text-3 text-xs">
          {t("office.copilot.counters_hint")}
        </p>
      </div>
      <div className="grid grid-cols-2 gap-2 sm:grid-cols-4 xl:grid-cols-8">
        {summary.steps.map((c) => {
          const Icon = STEP_ICON[c.key];
          const pressed = active === c.key;
          return (
            <Button
              key={c.key}
              variant="outline"
              aria-pressed={pressed}
              data-step={c.key}
              disabled={c.count === 0 && !pressed}
              onClick={() => onPick(c.key)}
              className={cn(
                "bg-card hover:bg-card h-auto flex-col items-start gap-1 rounded-xl p-3 text-left font-normal shadow-none",
                pressed ? "border-foreground/40" : "hover:border-foreground/20",
              )}
            >
              <span className="text-muted-foreground flex items-center gap-1.5 text-xs">
                <Icon aria-hidden className="size-3.5" />
                {stepLabel(t, c.key)}
              </span>
              <span className="text-xl font-semibold tabular-nums">
                {c.count}
              </span>
              <span className="sr-only">
                {t("office.copilot.counter_sr", {
                  count: c.count,
                  step: stepLabel(t, c.key),
                })}
              </span>
            </Button>
          );
        })}
      </div>
      <p className="text-text-3 flex flex-wrap items-center gap-x-4 gap-y-1 text-xs">
        <span className="inline-flex items-center gap-1">
          <Hourglass aria-hidden className="size-3.5" />
          {t("office.copilot.counter_waiting", { count: summary.waiting })}
        </span>
        <span className="inline-flex items-center gap-1">
          <CircleCheck aria-hidden className="size-3.5" />
          {t("office.copilot.counter_closed", { count: summary.closed })}
        </span>
        {summary.truncated ? (
          <span>
            {t("office.copilot.counters_truncated", {
              count: summary.scanned,
            })}
          </span>
        ) : null}
      </p>
    </section>
  );
}
