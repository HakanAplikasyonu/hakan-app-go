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

const PROGRAMS: { value: AccountingProgram; labelKey: string }[] = [
  { value: "luca", labelKey: "setup.accounting_luca" },
  { value: "zirve", labelKey: "setup.accounting_zirve" },
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
        className="flex flex-wrap gap-6"
      >
        {PROGRAMS.map((p) => (
          <div key={p.value} className="flex items-center gap-2">
            <RadioGroupItem
              value={p.value}
              id={`setup-accounting-${p.value}`}
            />
            <Label
              htmlFor={`setup-accounting-${p.value}`}
              className="font-normal"
            >
              {t(p.labelKey)}
            </Label>
          </div>
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
      <h3 className="mb-2 text-sm font-medium">
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
