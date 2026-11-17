"use client";

import { useQuery } from "@tanstack/react-query";

import { StatusPill, type StatusTone } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { copilotKeys, copilotService } from "../services/copilot.service";
import type { CopilotPlan } from "../services/copilot.service";
import { useCopilot } from "./copilot-provider";

const TONE: Record<CopilotPlan["status"], StatusTone> = {
  proposed: "warn",
  running: "info",
  done: "ok",
  cancelled: "neutral",
};

/** The user's latest commands; a plan still waiting can be reopened. */
export function RecentPlans() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const copilot = useCopilot();
  const query = useQuery({
    queryKey: copilotKeys.plans(slug),
    queryFn: () => copilotService.plans(slug),
  });
  const items = query.data?.items ?? [];
  if (items.length === 0) return null;

  return (
    <section aria-labelledby="copilot-recent-title" className="space-y-2">
      <h2 id="copilot-recent-title" className="text-sm font-semibold">
        {t("office.copilot.recent.title")}
      </h2>
      <ul className="divide-line divide-y rounded-md border">
        {items.slice(0, 5).map((p) => {
          const done = p.steps.filter((s) => s.status === "done").length;
          const open =
            (p.status === "proposed" || p.status === "running") &&
            p.steps.some((s) => s.status === "pending");
          return (
            <li
              key={p.uuid}
              className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
            >
              <div className="min-w-0">
                <p className="truncate text-sm">{p.command}</p>
                <p className="text-text-3 text-xs">
                  {new Date(p.created_at).toLocaleString(
                    locale === "tr" ? "tr-TR" : "en-GB",
                    { dateStyle: "medium", timeStyle: "short" },
                  )}
                  {" · "}
                  {t("office.copilot.recent.steps", {
                    done,
                    total: p.steps.length,
                  })}
                </p>
              </div>
              <div className="flex items-center gap-2">
                <StatusPill tone={TONE[p.status]}>
                  {t(`office.copilot.recent.status_${p.status}`)}
                </StatusPill>
                {open ? (
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => copilot.resume(p)}
                  >
                    {t("office.copilot.recent.resume")}
                  </Button>
                ) : null}
              </div>
            </li>
          );
        })}
      </ul>
    </section>
  );
}
