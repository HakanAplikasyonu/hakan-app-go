"use client";

import { ArrowRight } from "lucide-react";
import Link from "next/link";
import { useState, type ReactNode } from "react";

import { Loading } from "@/components/common/loading";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useEngagementOptions } from "@/features/office/hooks/use-office-invoices";
import {
  InviteForm,
  useOfficeRoles,
} from "@/features/tenant/components/invite-form";
import {
  usePatchTenantSettings,
  useTenantSettings,
} from "@/features/tenant/hooks/use-tenant-admin";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import type { AccountingProgram } from "../services/office-setup.service";
import { StepActions, StepError } from "./step-actions";
import type { SetupStepProps } from "./step-props";

function Description({ children }: { children: ReactNode }) {
  return <p className="text-muted-foreground mb-4 text-sm">{children}</p>;
}

// --- Ofis bilgileri ---

function OfficeNameField({
  initial,
  step,
}: {
  initial: string;
  step: SetupStepProps;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const patch = usePatchTenantSettings(slug);
  const [name, setName] = useState(initial);
  const [error, setError] = useState<string | null>(null);

  const onSave = async () => {
    const value = name.trim();
    if (!value) {
      setError(t("setup.office_name_required"));
      return;
    }
    setError(null);
    try {
      // Only the name: `settings` would replace the whole settings object.
      await patch.mutateAsync({ name: value });
    } catch {
      return; // toasted by the mutation cache
    }
    await step.onDone();
  };

  return (
    <>
      <div className="grid max-w-md gap-2">
        <Label htmlFor="setup-office-name">{t("setup.office_name")}</Label>
        <Input
          id="setup-office-name"
          name="name"
          value={name}
          maxLength={255}
          autoComplete="organization"
          aria-invalid={Boolean(error)}
          aria-describedby={error ? "setup-office-name-error" : undefined}
          onChange={(e) => setName(e.target.value)}
        />
        {error ? (
          <p
            id="setup-office-name-error"
            role="alert"
            className="text-destructive text-sm"
          >
            {error}
          </p>
        ) : null}
      </div>
      <StepActions
        isLast={step.isLast}
        busy={step.saving || patch.isPending}
        onSkip={() => void step.onSkip()}
        onSave={() => void onSave()}
      />
    </>
  );
}

export function OfficeStep(props: SetupStepProps) {
  const { t } = useLocale();
  const { slug, organization } = useTenant();
  const settings = useTenantSettings(slug);
  return (
    <div>
      <Description>{t("setup.office_description")}</Description>
      {settings.isLoading ? (
        <Loading label={t("common.loading")} />
      ) : (
        <OfficeNameField
          initial={settings.data?.name ?? organization.name}
          step={props}
        />
      )}
    </div>
  );
}

// --- Ekip ---

export function TeamStep({
  invited,
  onInvited,
  ...props
}: SetupStepProps & {
  invited: readonly string[];
  onInvited: (email: string) => void;
}) {
  const { t } = useLocale();
  const { role } = useTenant();
  const { can } = usePermission();
  const officeRoles = useOfficeRoles();
  const canInvite = can(permissions.tenant.members.write);

  return (
    <div>
      <Description>{t("setup.team_description")}</Description>
      {canInvite ? (
        <InviteForm
          id="setup-invite-form"
          actorRole={role.key}
          roleOptions={officeRoles.options}
          submitLabel={t("setup.team_add")}
          onInvited={(inv) => onInvited(inv.email)}
        />
      ) : (
        <p className="text-muted-foreground text-sm">
          {t("setup.team_no_permission")}
        </p>
      )}
      {invited.length ? (
        <section aria-labelledby="setup-invited-title">
          <h3 id="setup-invited-title" className="mb-2 text-sm font-medium">
            {t("setup.team_invited")}
          </h3>
          <ul className="divide-y rounded-md border text-sm">
            {invited.map((email) => (
              <li key={email} className="px-3 py-2">
                {email}
              </li>
            ))}
          </ul>
        </section>
      ) : null}
      <StepActions
        isLast={props.isLast}
        busy={props.saving}
        onSkip={() => void props.onSkip()}
        onSave={() => void props.onDone()}
      />
    </div>
  );
}

// --- Muhasebe programı ---

const PROGRAMS: {
  value: AccountingProgram;
  labelKey: string;
  hintKey: string;
}[] = [
  {
    value: "luca",
    labelKey: "setup.accounting_luca",
    hintKey: "setup.accounting_luca_hint",
  },
  {
    value: "zirve",
    labelKey: "setup.accounting_zirve",
    hintKey: "setup.accounting_zirve_hint",
  },
];

export function AccountingStep(props: SetupStepProps) {
  const { t } = useLocale();
  const [program, setProgram] = useState<AccountingProgram | undefined>(
    props.state.accounting_program,
  );
  const [error, setError] = useState<string | null>(null);

  const onSave = () => {
    if (!program) {
      setError(t("setup.accounting_required"));
      return;
    }
    setError(null);
    void props.onDone({ accountingProgram: program });
  };

  return (
    <div>
      <Description>{t("setup.accounting_description")}</Description>
      <p id="setup-accounting-label" className="mb-2 text-sm font-medium">
        {t("setup.accounting_label")}
      </p>
      <RadioGroup
        aria-labelledby="setup-accounting-label"
        value={program ?? ""}
        onValueChange={(v) => {
          setProgram(v as AccountingProgram);
          setError(null);
        }}
        className="grid gap-3 sm:grid-cols-2"
      >
        {PROGRAMS.map((p) => (
          <Label
            key={p.value}
            htmlFor={`setup-accounting-${p.value}`}
            className="has-[button[data-state=checked]]:border-brand has-[button[data-state=checked]]:bg-brand-tint hover:border-line-strong flex min-h-16 cursor-pointer items-start gap-3 rounded-lg border p-4 font-normal"
          >
            <RadioGroupItem
              value={p.value}
              id={`setup-accounting-${p.value}`}
              aria-describedby={`setup-accounting-${p.value}-hint`}
              className="mt-0.5"
            />
            <span className="grid gap-0.5">
              <span className="text-base font-semibold">{t(p.labelKey)}</span>
              <span
                id={`setup-accounting-${p.value}-hint`}
                className="text-text-2 text-sm"
              >
                {t(p.hintKey)}
              </span>
            </span>
          </Label>
        ))}
      </RadioGroup>
      <StepError message={error} />
      <StepActions
        isLast={props.isLast}
        busy={props.saving}
        onSkip={() => void props.onSkip()}
        onSave={onSave}
      />
    </div>
  );
}

// --- Entegratör bağlantıları ---

export function IntegrationsStep(props: SetupStepProps) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const engagements = useEngagementOptions(slug);

  return (
    <div>
      <Description>{t("setup.integrations_description")}</Description>
      <ul className="mb-6 grid gap-3 md:grid-cols-3">
        {[
          {
            title: t("setup.source_integrator"),
            body: t("setup.source_integrator_body"),
            href: "#setup-integrations-list",
            action: t("setup.source_integrator_action"),
          },
          {
            title: t("setup.source_agent"),
            body: t("setup.source_agent_body"),
            href: routes.tenant.agents(slug),
            action: t("setup.source_agent_action"),
          },
          {
            title: t("setup.source_upload"),
            body: t("setup.source_upload_body"),
            href: routes.tenant.upload(slug),
            action: t("setup.source_upload_action"),
          },
        ].map((c) => (
          <li
            key={c.title}
            className="bg-surface-2 flex flex-col gap-2 rounded-lg border p-4"
          >
            <span className="text-sm font-semibold">{c.title}</span>
            <span className="text-text-2 flex-1 text-sm">{c.body}</span>
            <Link
              href={c.href}
              className="text-brand-text inline-flex min-h-11 items-center gap-1 text-sm font-semibold hover:underline"
            >
              {c.action}
              <ArrowRight className="size-3.5" aria-hidden />
            </Link>
          </li>
        ))}
      </ul>
      <h3 id="setup-integrations-list" className="mb-2 text-sm font-medium">
        {t("setup.integrations_list_title")}
      </h3>
      {engagements.isLoading ? (
        <Loading label={t("common.loading")} className="justify-start" />
      ) : engagements.isError ? (
        <p className="text-destructive text-sm">{t("common.error_generic")}</p>
      ) : engagements.options.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("setup.integrations_empty")}
        </p>
      ) : (
        <ul className="divide-y rounded-md border text-sm">
          {engagements.options.map((e) => (
            <li
              key={e.engagement}
              className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
            >
              <span>
                <span className="font-medium">{e.title || e.taxId || "—"}</span>
                {e.title && e.taxId ? (
                  <span className="text-muted-foreground figure ms-2 text-xs">
                    {e.taxId}
                  </span>
                ) : null}
              </span>
              <Link
                href={routes.tenant.engagement(slug, e.engagement)}
                className="text-primary inline-flex items-center gap-1 text-sm hover:underline"
              >
                {t("setup.integrations_open")}
                <span className="sr-only">: {e.title || e.taxId}</span>
                <ArrowRight className="size-3.5" aria-hidden />
              </Link>
            </li>
          ))}
        </ul>
      )}
      <StepActions
        isLast={props.isLast}
        busy={props.saving}
        onSkip={() => void props.onSkip()}
        onSave={() => void props.onDone()}
      />
    </div>
  );
}
