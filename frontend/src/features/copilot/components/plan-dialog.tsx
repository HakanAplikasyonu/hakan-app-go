"use client";

import {
  CheckCircle2,
  CircleAlert,
  CircleDashed,
  CircleMinus,
  Loader2,
  Sparkles,
} from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  useCancelPlan,
  useRecordStep,
  useRefreshOffice,
} from "../hooks/use-copilot";
import {
  ACTION_PERMISSIONS,
  actionHref,
  type RunnableStep,
} from "../lib/actions";
import {
  actionLabel,
  errorLabel,
  noteLabel,
  outcomeLabel,
  periodLabel,
} from "../lib/labels";
import { runStep, type StepOutcome } from "../lib/run-action";
import type { CopilotNote } from "../services/copilot.service";

/** What the dialog shows: a server plan (has uuid) or a client one. */
export type PlanView = {
  uuid?: string;
  command?: string;
  planner?: "ai" | "rules";
  steps: RunnableStep[];
  notes: CopilotNote[];
};

type StepState =
  | { phase: "pending" }
  | { phase: "running" }
  | { phase: "skipped" }
  | { phase: "finished"; outcome: StepOutcome };

/**
 * The approval screen of a plan: every step is a checkbox (on by default
 * when the user may run it). Nothing runs until "Onayla ve çalıştır";
 * then the checked steps run one by one through their own endpoints and
 * the rest is recorded as skipped.
 */
export function PlanDialog({
  open,
  onOpenChange,
  plan,
  planning = false,
  planError,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  plan: PlanView | null;
  planning?: boolean;
  planError?: string | null;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const record = useRecordStep(slug);
  const cancel = useCancelPlan(slug);
  const refresh = useRefreshOffice(slug);
  const steps = useMemo(() => plan?.steps ?? [], [plan]);
  const allowed = (s: RunnableStep) =>
    ACTION_PERMISSIONS[s.action].every((p) => can(p));
  // Steps are ticked unless the user unticks them. The provider remounts
  // the dialog for every new plan, so the state starts fresh.
  const [unchecked, setUnchecked] = useState<Record<number, boolean>>({});
  const checked = (index: number) => !unchecked[index];
  const [states, setStates] = useState<Record<number, StepState>>({});
  const [running, setRunning] = useState(false);
  const [finished, setFinished] = useState(false);

  const selected = steps.filter((s) => checked(s.index) && allowed(s));

  const report = (
    index: number,
    result: { status: "done" | "failed" | "skipped"; error_code?: string },
  ) => {
    if (!plan?.uuid) return;
    record.mutate({ plan: plan.uuid, index, result });
  };

  const onRun = async () => {
    setRunning(true);
    for (const s of steps) {
      if (!checked(s.index) || !allowed(s)) {
        setStates((m) => ({ ...m, [s.index]: { phase: "skipped" } }));
        report(s.index, { status: "skipped" });
        continue;
      }
      setStates((m) => ({ ...m, [s.index]: { phase: "running" } }));
      const outcome = await runStep(slug, s);
      setStates((m) => ({ ...m, [s.index]: { phase: "finished", outcome } }));
      report(
        s.index,
        outcome.status === "failed"
          ? { status: "failed", error_code: outcome.code.slice(0, 64) }
          : { status: "done" },
      );
    }
    setRunning(false);
    setFinished(true);
    refresh();
  };

  const onClose = (next: boolean) => {
    if (running) return;
    if (!next && plan?.uuid && !finished && steps.length > 0) {
      cancel.mutate(plan.uuid);
    }
    onOpenChange(next);
  };

  const failed = Object.values(states).filter(
    (s) => s.phase === "finished" && s.outcome.status === "failed",
  ).length;

  return (
    <Dialog open={open} onOpenChange={onClose}>
      <DialogContent className="sm:max-w-2xl">
        <DialogHeader>
          <DialogTitle className="flex items-center gap-2">
            <Sparkles aria-hidden className="text-brand size-4" />
            {t("office.copilot.plan.title")}
          </DialogTitle>
          <DialogDescription>
            {plan?.command
              ? t("office.copilot.plan.for_command", { command: plan.command })
              : t("office.copilot.plan.description")}
          </DialogDescription>
        </DialogHeader>

        {planning ? (
          <p
            className="text-text-2 flex items-center gap-2 py-6 text-sm"
            role="status"
          >
            <Loader2 aria-hidden className="size-4 animate-spin" />
            {t("office.copilot.plan.planning")}
          </p>
        ) : planError ? (
          <p className="text-danger py-4 text-sm" role="alert">
            {planError}
          </p>
        ) : plan ? (
          <div className="space-y-4">
            {plan.notes.length > 0 ? (
              <ul className="bg-surface-2 space-y-1 rounded-md p-3 text-sm">
                {plan.notes.map((n, i) => (
                  <li key={i} className="text-text-2">
                    {noteLabel(t, n)}
                  </li>
                ))}
              </ul>
            ) : null}
            {steps.length === 0 ? (
              <p className="text-text-2 text-sm">
                {t("office.copilot.plan.empty")}
              </p>
            ) : (
              <ol className="divide-border divide-y rounded-md border">
                {steps.map((s) => {
                  const st = states[s.index] ?? { phase: "pending" };
                  const ok = allowed(s);
                  const id = `copilot-step-${s.index}`;
                  return (
                    <li key={s.index} className="flex items-start gap-3 p-3">
                      {st.phase === "pending" ? (
                        <Checkbox
                          id={id}
                          className="mt-0.5"
                          checked={checked(s.index) && ok}
                          disabled={!ok || running || finished}
                          onCheckedChange={(v) =>
                            setUnchecked((m) => ({
                              ...m,
                              [s.index]: v !== true,
                            }))
                          }
                        />
                      ) : (
                        <StepIcon state={st} />
                      )}
                      <div className="min-w-0 flex-1">
                        <label
                          htmlFor={id}
                          className="block text-sm font-medium"
                        >
                          {actionLabel(t, s.action)}
                        </label>
                        <p className="text-text-2 text-xs">
                          {s.taxpayer_title} · {periodLabel(locale, s.period)}
                          {s.action === "export_transfer" ? (
                            <>
                              {" "}
                              ·{" "}
                              {t(`office.copilot.driver.${s.driver ?? "luca"}`)}
                            </>
                          ) : null}
                        </p>
                        {!ok ? (
                          <p className="text-warn text-xs">
                            {t("office.copilot.plan.no_permission")}
                          </p>
                        ) : null}
                        {st.phase === "finished" ? (
                          <p
                            className={
                              st.outcome.status === "failed"
                                ? "text-danger text-xs"
                                : "text-ok text-xs"
                            }
                          >
                            {st.outcome.status === "failed"
                              ? errorLabel(t, st.outcome.code)
                              : outcomeLabel(t, s.action, st.outcome)}
                            {st.outcome.status === "failed" ? (
                              <>
                                {" "}
                                <Link
                                  className="underline"
                                  href={actionHref(
                                    slug,
                                    s.action,
                                    s.engagement_uuid,
                                  )}
                                >
                                  {t("office.copilot.plan.open_screen")}
                                </Link>
                              </>
                            ) : null}
                          </p>
                        ) : null}
                        {st.phase === "skipped" ? (
                          <p className="text-text-3 text-xs">
                            {t("office.copilot.plan.skipped")}
                          </p>
                        ) : null}
                      </div>
                    </li>
                  );
                })}
              </ol>
            )}
            {steps.length > 0 && !finished ? (
              <p className="text-text-3 text-xs">
                {t("office.copilot.plan.approval_hint")}
              </p>
            ) : null}
            {finished ? (
              <p className="text-sm" role="status">
                {failed > 0
                  ? t("office.copilot.plan.finished_failed", { count: failed })
                  : t("office.copilot.plan.finished")}
              </p>
            ) : null}
          </div>
        ) : null}

        <DialogFooter>
          {finished || !plan || steps.length === 0 ? (
            <Button variant="outline" onClick={() => onClose(false)}>
              {t("office.copilot.plan.close")}
            </Button>
          ) : (
            <>
              <Button
                variant="outline"
                disabled={running}
                onClick={() => onClose(false)}
              >
                {t("office.copilot.plan.cancel")}
              </Button>
              <Button
                disabled={running || selected.length === 0}
                onClick={() => void onRun()}
              >
                {running ? (
                  <Loader2 aria-hidden className="size-4 animate-spin" />
                ) : null}
                {running
                  ? t("office.copilot.plan.running")
                  : t("office.copilot.plan.run", { count: selected.length })}
              </Button>
            </>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function StepIcon({ state }: { state: StepState }) {
  const { t } = useLocale();
  if (state.phase === "running")
    return (
      <Loader2
        aria-label={t("office.copilot.plan.running")}
        className="text-brand mt-0.5 size-4 animate-spin"
      />
    );
  if (state.phase === "skipped")
    return (
      <CircleMinus
        aria-label={t("office.copilot.plan.skipped")}
        className="text-text-3 mt-0.5 size-4"
      />
    );
  if (state.phase === "finished" && state.outcome.status === "failed")
    return (
      <CircleAlert
        aria-label={t("office.copilot.status.blocked")}
        className="text-danger mt-0.5 size-4"
      />
    );
  if (state.phase === "finished")
    return (
      <CheckCircle2
        aria-label={t("office.copilot.status.done")}
        className="text-ok mt-0.5 size-4"
      />
    );
  return <CircleDashed aria-hidden className="text-text-3 mt-0.5 size-4" />;
}
