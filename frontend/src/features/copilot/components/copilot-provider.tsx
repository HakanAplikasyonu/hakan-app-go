"use client";

import {
  createContext,
  useCallback,
  useContext,
  useMemo,
  useState,
  type ReactNode,
} from "react";

import { useOfficePeriod } from "@/features/office/period";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { ApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import { useCreatePlan } from "../hooks/use-copilot";
import type { RunnableStep } from "../lib/actions";
import type { CopilotPlan } from "../services/copilot.service";
import { PlanDialog, type PlanView } from "./plan-dialog";

type CopilotContextValue = {
  /** Plans a natural language command and shows it for approval. */
  ask: (command: string) => void;
  /** Shows steps picked on a screen (one row, a bulk selection). */
  review: (steps: Omit<RunnableStep, "index">[]) => void;
  /** Reopens a stored plan whose steps still wait for approval. */
  resume: (plan: CopilotPlan) => void;
};

const CopilotContext = createContext<CopilotContextValue | null>(null);

export function planView(p: CopilotPlan): PlanView {
  return {
    uuid: p.uuid,
    command: p.command,
    planner: p.planner,
    notes: p.notes,
    steps: p.steps
      .filter((s) => s.status === "pending")
      .map((s) => ({
        index: s.index,
        action: s.action,
        engagement_uuid: s.engagement_uuid,
        taxpayer_title: s.taxpayer_title,
        period: s.period,
        driver: s.driver,
      })),
  };
}

/**
 * Office-wide co-pilot (#392): the command bar (⌘K → "Co-pilot'a sor") and
 * the approval dialog every screen opens.
 */
export function CopilotProvider({ children }: { children: ReactNode }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { period } = useOfficePeriod();
  const create = useCreatePlan(slug);
  const [open, setOpen] = useState(false);
  const [plan, setPlan] = useState<PlanView | null>(null);
  const [error, setError] = useState<string | null>(null);
  // A new plan remounts the dialog (fresh ticks and outcomes).
  const [version, setVersion] = useState(0);

  const ask = useCallback(
    (command: string) => {
      const text = command.trim();
      if (!text) return;
      setPlan(null);
      setError(null);
      setVersion((v) => v + 1);
      setOpen(true);
      create.mutate(
        { command: text, period },
        {
          onSuccess: (p) => setPlan(planView(p)),
          onError: (e) =>
            setError(
              e instanceof ApiError && e.status === 403
                ? t("office.copilot.plan.forbidden")
                : t("office.copilot.plan.error"),
            ),
        },
      );
    },
    [create, period, t],
  );

  const review = useCallback((steps: Omit<RunnableStep, "index">[]) => {
    setError(null);
    setVersion((v) => v + 1);
    setPlan({ steps: steps.map((s, index) => ({ ...s, index })), notes: [] });
    setOpen(true);
  }, []);

  const resume = useCallback((p: CopilotPlan) => {
    setError(null);
    setVersion((v) => v + 1);
    setPlan(planView(p));
    setOpen(true);
  }, []);

  const value = useMemo(() => ({ ask, review, resume }), [ask, review, resume]);

  return (
    <CopilotContext.Provider value={value}>
      {children}
      <PlanDialog
        key={version}
        open={open}
        onOpenChange={setOpen}
        plan={plan}
        planning={create.isPending}
        planError={error}
      />
    </CopilotContext.Provider>
  );
}

export function useCopilot() {
  const ctx = useContext(CopilotContext);
  if (!ctx) throw new Error("useCopilot must be used within CopilotProvider");
  return ctx;
}

/** The co-pilot when the screen is inside the office shell, else null. */
export function useOptionalCopilot() {
  return useContext(CopilotContext);
}
