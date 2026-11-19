"use client";

import { Check, Minus } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PermissionGuard } from "@/components/common/permission-guard";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useOfficeSetup, useSaveOfficeSetup } from "../hooks/use-office-setup";
import {
  SETUP_STATUS_LABEL_KEYS,
  SETUP_STEPS,
  SETUP_STEP_TITLE_KEYS,
  initialStepIndex,
  setupInput,
  stepStatus,
  type SetupStepKey,
} from "../lib/steps";
import type {
  OfficeSetupState,
  OfficeSetupStepStatus,
} from "../services/office-setup.service";
import {
  AccountingStep,
  IntegrationsStep,
  OfficeStep,
  TeamStep,
} from "./basic-steps";
import { InvitesStep, TaxpayersStep } from "./import-steps";
import { WizardNav } from "./step-actions";
import type { SetupStepProps } from "./step-props";

/** Sol adım listesi (OfisKurulum): ilerleme, adım ve durum; dar ekranda üstte yatay. */
function Stepper({
  state,
  active,
  onSelect,
}: {
  state: OfficeSetupState;
  active: number;
  onSelect: (index: number) => void;
}) {
  const { t } = useLocale();
  const { organization } = useTenant();
  const done = SETUP_STEPS.filter(
    (k) => stepStatus(state, k) === "done",
  ).length;
  return (
    <aside className="bg-card flex flex-col gap-4 rounded-[14px] border p-4 lg:sticky lg:top-20 lg:self-start lg:p-5">
      <div>
        <h1 className="font-display text-xl font-semibold tracking-tight">
          {t("setup.title")}
        </h1>
        <p className="text-text-2 truncate text-sm">{organization.name}</p>
      </div>
      <div>
        <p className="text-text-2 mb-1.5 text-xs" id="setup-progress-label">
          {t("setup.progress", { done, total: SETUP_STEPS.length })}
        </p>
        <div
          role="progressbar"
          aria-labelledby="setup-progress-label"
          aria-valuemin={0}
          aria-valuemax={SETUP_STEPS.length}
          aria-valuenow={done}
          className="bg-surface-3 h-1.5 overflow-hidden rounded-full"
        >
          <div
            className="bg-brand h-full rounded-full motion-safe:transition-[width] motion-safe:duration-300"
            style={{ width: `${(done / SETUP_STEPS.length) * 100}%` }}
          />
        </div>
      </div>
      <nav aria-label={t("setup.stepper_label")}>
        <ol className="-mx-1 flex gap-1 overflow-x-auto px-1 pb-1 lg:flex-col lg:overflow-visible">
          {SETUP_STEPS.map((key, i) => {
            const status = stepStatus(state, key);
            const current = i === active;
            return (
              <li key={key} className="shrink-0">
                <Button
                  variant="ghost"
                  aria-current={current ? "step" : undefined}
                  aria-describedby={`setup-step-status-${key}`}
                  onClick={() => onSelect(i)}
                  className={cn(
                    "h-auto min-h-11 w-full justify-start gap-3 rounded-[10px] px-2.5 py-2 text-start font-normal transition-colors",
                    current
                      ? "bg-brand-tint text-brand-text hover:bg-brand-tint hover:text-brand-text"
                      : "hover:bg-surface-2",
                  )}
                >
                  <span
                    aria-hidden
                    className={cn(
                      "flex size-7 shrink-0 items-center justify-center rounded-full border font-mono text-xs font-semibold",
                      status === "done" && "bg-ok border-ok text-white",
                      status === "skipped" && "bg-surface-3 text-text-2",
                      current && status === "pending" && "border-brand",
                    )}
                  >
                    {status === "done" ? (
                      <Check className="size-3.5" />
                    ) : status === "skipped" ? (
                      <Minus className="size-3.5" />
                    ) : (
                      i + 1
                    )}
                  </span>
                  <span className="grid gap-0.5">
                    <span className="text-sm leading-tight font-medium whitespace-nowrap">
                      {t(SETUP_STEP_TITLE_KEYS[key])}
                    </span>
                    {/* Described, not named: the button's name is the title. */}
                    <span
                      id={`setup-step-status-${key}`}
                      aria-hidden
                      className="text-text-2 text-xs"
                    >
                      {current
                        ? t("setup.status_current")
                        : t(SETUP_STATUS_LABEL_KEYS[status])}
                    </span>
                  </span>
                </Button>
              </li>
            );
          })}
        </ol>
      </nav>
      <p className="text-text-2 hidden text-xs lg:block">
        {t("setup.resume_hint")}
      </p>
    </aside>
  );
}

function Wizard({ state }: { state: OfficeSetupState }) {
  const { t } = useLocale();
  const router = useRouter();
  const { slug } = useTenant();
  const save = useSaveOfficeSetup(slug);
  // Resume at the first pending step (the last one when none is pending).
  const [active, setActive] = useState(() => initialStepIndex(state));
  // Team invitations sent from this wizard visit.
  const [invited, setInvited] = useState<string[]>([]);
  const key: SetupStepKey = SETUP_STEPS[active] ?? "office";
  const isLast = active === SETUP_STEPS.length - 1;

  const settle = async (
    status: OfficeSetupStepStatus,
    extra?: Parameters<SetupStepProps["onDone"]>[0],
  ) => {
    try {
      await save.mutateAsync(
        setupInput(state, {
          step: { key, status },
          accountingProgram: extra?.accountingProgram,
        }),
      );
    } catch {
      return; // toasted by the mutation cache; stay on the step
    }
    if (isLast) router.push(routes.tenant.home(slug));
    else setActive(active + 1);
  };

  const props: SetupStepProps = {
    state,
    isLast,
    saving: save.isPending,
    onSkip: () => settle("skipped"),
    onDone: (extra) => settle("done", extra),
  };

  return (
    <div className="grid grid-cols-[minmax(0,1fr)] gap-4 lg:grid-cols-[280px_minmax(0,1fr)] lg:gap-6">
      <Stepper state={state} active={active} onSelect={setActive} />
      <WizardNav.Provider
        value={{ onBack: active > 0 ? () => setActive(active - 1) : undefined }}
      >
        <section
          aria-labelledby="setup-step-title"
          className="bg-card text-card-foreground rounded-[14px] border p-5 sm:p-8"
        >
          <p className="text-text-2 font-mono text-xs">
            {t("setup.step_counter", {
              current: active + 1,
              total: SETUP_STEPS.length,
            })}
          </p>
          <h2
            id="setup-step-title"
            className="font-display mt-1 mb-4 text-2xl font-semibold tracking-tight"
          >
            {t(SETUP_STEP_TITLE_KEYS[key])}
          </h2>
          {/* key: a step's local state starts fresh when it is opened again. */}
          {key === "office" ? <OfficeStep key={key} {...props} /> : null}
          {key === "team" ? (
            <TeamStep
              key={key}
              {...props}
              invited={invited}
              onInvited={(email) =>
                setInvited((list) =>
                  list.includes(email) ? list : [...list, email],
                )
              }
            />
          ) : null}
          {key === "taxpayers" ? <TaxpayersStep key={key} {...props} /> : null}
          {key === "accounting" ? (
            <AccountingStep key={key} {...props} />
          ) : null}
          {key === "integrations" ? (
            <IntegrationsStep key={key} {...props} />
          ) : null}
          {key === "invites" ? <InvitesStep key={key} {...props} /> : null}
        </section>
      </WizardNav.Provider>
    </div>
  );
}

/** Loads the state inside the permission gate (no request when refused). */
function WizardLoader() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const query = useOfficeSetup(slug);
  if (query.isLoading) return <Loading label={t("common.loading")} />;
  if (!query.data) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }
  return <Wizard state={query.data} />;
}

/** `/t/{slug}/setup` — the office setup wizard (issue #273, OfisKurulum #330). */
export function OfficeSetupPage() {
  const { t } = useLocale();
  return (
    <PermissionGuard
      permission={permissions.tenant.settings.write}
      fallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("errors.forbidden_body")}
        />
      }
    >
      <WizardLoader />
    </PermissionGuard>
  );
}
