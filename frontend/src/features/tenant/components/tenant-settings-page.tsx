"use client";

import { Building2 } from "lucide-react";
import { useMemo } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { GorunumAyari } from "@/components/hakan";
import { AppForm, AppInput } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { OfficeDpaCard } from "@/features/legal";
import { AgentsPanel } from "@/features/office/components/agents-page";
import { AiApprovalSettings } from "@/features/office/components/ai-approval-settings";
import {
  usePatchTenantSettings,
  useTenantSettings,
} from "@/features/tenant/hooks/use-tenant-admin";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { StatementAiSetting } from "@/features/tenant/components/statement-ai-setting";
import { TaxReturnFinalCheckSetting } from "@/features/tenant/components/tax-return-final-check-setting";
import { VATTrackingSetting } from "@/features/tenant/components/vat-tracking-setting";
import { SupportSettingsCard } from "@/features/tenant/components/support-settings-card";
import { TenantForbidden } from "@/features/tenant/components/tenant-forbidden";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

type SectionId =
  "office" | "tax-returns" | "vat" | "support" | "ai" | "agents" | "appearance";

/** Ayarlar alt menüsü (OfisAyarlar): geniş ekranda solda, dar ekranda üstte. */
function SettingsNav({
  sections,
}: {
  sections: { id: SectionId; label: string }[];
}) {
  const { t } = useLocale();
  return (
    <nav
      aria-label={t("tenant.settings_nav_label")}
      className="lg:sticky lg:top-20 lg:self-start"
    >
      <ul className="flex gap-1 overflow-x-auto pb-1 lg:flex-col lg:overflow-visible">
        {sections.map((s) => (
          <li key={s.id} className="shrink-0">
            <a
              href={`#${s.id}`}
              className="hover:bg-surface-2 text-text-2 hover:text-foreground flex min-h-11 items-center rounded-md px-3 text-sm font-medium whitespace-nowrap"
            >
              {s.label}
            </a>
          </li>
        ))}
      </ul>
    </nav>
  );
}

function Section({
  id,
  title,
  description,
  children,
}: {
  id: SectionId;
  title: string;
  description?: string;
  children: React.ReactNode;
}) {
  return (
    <section
      id={id}
      aria-labelledby={`${id}-heading`}
      className="scroll-mt-20 space-y-4"
    >
      <div>
        <h2
          id={`${id}-heading`}
          className="font-display text-xl font-semibold tracking-tight"
        >
          {title}
        </h2>
        {description ? (
          <p className="text-text-2 text-sm">{description}</p>
        ) : null}
      </div>
      {children}
    </section>
  );
}

/**
 * `/t/{slug}/settings` — organization name. The app-defined `settings`
 * object is left to apps (PATCH /v1/tenant/settings accepts it); the slug is
 * managed by the platform because every /t/{slug} link depends on it.
 * Below it, the data processing agreement of the office (issue #57).
 */
export function TenantSettingsPage() {
  const { t } = useLocale();
  const tenant = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.settings.write);
  const query = useTenantSettings(tenant.slug);
  const patch = usePatchTenantSettings(tenant.slug);
  const schema = useMemo(
    () =>
      z.object({
        name: z
          .string()
          .trim()
          .min(1, t("tenant.settings_validation_name"))
          .max(255),
      }),
    [t],
  );
  const settings = query.data;
  const showAi =
    can(permissions.tenant.review.read) ||
    can(permissions.tenant.accountRules.read);
  const showAgents = can(permissions.tenant.agents.read);
  const sections: { id: SectionId; label: string }[] = [
    { id: "office", label: t("tenant.settings_section_office") },
    {
      id: "tax-returns",
      label: t("tenant.settings_section_tax_returns"),
    },
    { id: "vat", label: t("tenant.settings_section_vat") },
    { id: "support", label: t("tenant.settings_section_support") },
    ...(showAi
      ? [{ id: "ai" as const, label: t("tenant.settings_section_ai") }]
      : []),
    ...(showAgents
      ? [{ id: "agents" as const, label: t("tenant.settings_section_agents") }]
      : []),
    { id: "appearance", label: t("tenant.settings_section_appearance") },
  ];

  return (
    <EntityPage
      title={t("tenant.settings_title")}
      description={t("tenant.settings_description")}
      permission={permissions.tenant.settings.read}
      forbiddenFallback={<TenantForbidden />}
      breadcrumbs={[
        {
          label: tenant.organization.name,
          href: routes.tenant.home(tenant.slug),
        },
        { label: t("tenant.settings_title") },
      ]}
    >
      <div className="grid grid-cols-[minmax(0,1fr)] gap-6 lg:grid-cols-[220px_minmax(0,1fr)]">
        <SettingsNav sections={sections} />
        <div className="flex min-w-0 flex-col gap-10">
          <Section id="office" title={t("tenant.settings_section_office")}>
            {query.isLoading ? <Loading label={t("common.loading")} /> : null}
            {query.isError ? (
              <ErrorState
                title={t("common.error_generic")}
                onRetry={() => void query.refetch()}
                retryLabel={t("common.retry")}
              />
            ) : null}
            {settings ? (
              <EntitySectionCard title={t("tenant.settings_title")}>
                <AppForm
                  key={settings.name}
                  id="tenant-settings-form"
                  schema={schema}
                  defaultValues={{ name: settings.name }}
                  onSubmit={async (values: { name: string }) => {
                    await patch.mutateAsync({ name: values.name.trim() });
                  }}
                >
                  {(form) => (
                    <div className="space-y-4">
                      <div className="grid gap-4 sm:grid-cols-2">
                        <AppInput
                          name="name"
                          label={t("tenant.settings_name")}
                          startIcon={Building2}
                          disabled={!canWrite}
                        />
                        <div className="space-y-2">
                          <div className="text-sm font-medium">
                            {t("tenant.settings_slug")}
                          </div>
                          <code className="bg-muted block rounded-md px-3 py-2 text-sm">
                            /t/{settings.slug}
                          </code>
                        </div>
                      </div>
                      {canWrite ? (
                        <div className="flex justify-end">
                          <Button
                            type="submit"
                            disabled={
                              patch.isPending ||
                              form.formState.isSubmitting ||
                              !form.formState.isDirty
                            }
                          >
                            {t("tenant.settings_save")}
                          </Button>
                        </div>
                      ) : null}
                    </div>
                  )}
                </AppForm>
              </EntitySectionCard>
            ) : null}
            <OfficeDpaCard slug={tenant.slug} />
          </Section>
          {/* Beyan son kontrolü (#710): varsayılan açık, kapatılabilir. */}
          <Section
            id="tax-returns"
            title={t("tenant.settings_section_tax_returns")}
            description={t("tenant.settings_section_tax_returns_description")}
          >
            <TaxReturnFinalCheckSetting
              slug={tenant.slug}
              canWrite={canWrite}
            />
          </Section>
          {/* KDV takibi (#780): ay sonu KDV raporu; varsayılan açık. */}
          <Section
            id="vat"
            title={t("tenant.settings_section_vat")}
            description={t("tenant.settings_section_vat_description")}
          >
            <VATTrackingSetting slug={tenant.slug} canWrite={canWrite} />
          </Section>
          {/* Destek (#387): ofisin ClickFix widget kimliği. */}
          <Section
            id="support"
            title={t("tenant.settings_section_support")}
            description={t("tenant.settings_section_support_description")}
          >
            <SupportSettingsCard slug={tenant.slug} canWrite={canWrite} />
          </Section>
          {showAi ? (
            <Section
              id="ai"
              title={t("tenant.settings_section_ai")}
              description={t("tenant.settings_section_ai_description")}
            >
              <AiApprovalSettings />
              <StatementAiSetting slug={tenant.slug} canWrite={canWrite} />
            </Section>
          ) : null}
          {showAgents ? (
            <Section id="agents" title={t("tenant.settings_section_agents")}>
              <AgentsPanel />
            </Section>
          ) : null}
          {/* Görünüm (arayüz v1, #324): hesaba özel tema. */}
          <div id="appearance" className="scroll-mt-20">
            <GorunumAyari />
          </div>
        </div>
      </div>
    </EntityPage>
  );
}
