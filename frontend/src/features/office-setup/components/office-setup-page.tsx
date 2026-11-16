"use client";

import { Check, Minus } from "lucide-react";
import { useRouter } from "next/navigation";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
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
import type { SetupStepProps } from "./step-props";

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
  return (
    <nav aria-label={t("setup.stepper_label")}>
      <ol className="grid gap-2 sm:grid-cols-3 lg:grid-cols-6">
        {SETUP_STEPS.map((key, i) => {
          const status = stepStatus(state, key);
          const current = i === active;
          return (
            <li key={key}>
              <button
                type="button"
                aria-current={current ? "step" : undefined}
                aria-describedby={`setup-step-status-${key}`}
                onClick={() => onSelect(i)}
                className={cn(
                  "hover:border-foreground/30 flex w-full items-start gap-2 rounded-lg border p-2.5 text-start transition-colors",
                  current && "border-primary ring-primary/30 ring-2",
                )}
              >
                <span
                  aria-hidden
                  className={cn(
                    "flex size-6 shrink-0 items-center justify-center rounded-full border text-xs font-semibold",
                    status === "done" &&
                      "bg-primary text-primary-foreground border-primary",
                    status === "skipped" && "bg-muted text-muted-foreground",
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
                  <span className="text-sm leading-tight font-medium">
                    {t(SETUP_STEP_TITLE_KEYS[key])}
                  </span>
                  {/* Described, not named: the button's name is the title. */}
                  <span
                    id={`setup-step-status-${key}`}
                    aria-hidden
                    className="text-muted-foreground text-xs"
                  >
                    {t(SETUP_STATUS_LABEL_KEYS[status])}
                  </span>
                </span>
              </button>
            </li>
          );
        })}
      </ol>
    </nav>
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
    <div className="space-y-4">
      <Stepper state={state} active={active} onSelect={setActive} />
      <section
        aria-labelledby="setup-step-title"
        className="bg-card text-card-foreground rounded-xl border p-4 sm:p-6"
      >
        <p className="text-muted-foreground text-xs">
          {t("setup.step_counter", {
            current: active + 1,
            total: SETUP_STEPS.length,
          })}
        </p>
        <h2 id="setup-step-title" className="mb-3 text-lg font-semibold">
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
        {key === "accounting" ? <AccountingStep key={key} {...props} /> : null}
        {key === "integrations" ? (
          <IntegrationsStep key={key} {...props} />
        ) : null}
        {key === "invites" ? <InvitesStep key={key} {...props} /> : null}
      </section>
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

/** `/t/{slug}/setup` — the office setup wizard (issue #273). */
export function OfficeSetupPage() {
  const { t } = useLocale();
  const tenant = useTenant();

  return (
    <EntityPage
      title={t("setup.title")}
      description={t("setup.description")}
      permission={permissions.tenant.settings.write}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("errors.forbidden_body")}
        />
      }
      breadcrumbs={[
        {
          label: tenant.organization.name,
          href: routes.tenant.home(tenant.slug),
        },
        { label: t("setup.title") },
      ]}
    >
      <WizardLoader />
    </EntityPage>
  );
}
