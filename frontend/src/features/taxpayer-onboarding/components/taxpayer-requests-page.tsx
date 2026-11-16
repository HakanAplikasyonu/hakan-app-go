"use client";

import { MailCheck, Send } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { AppForm, AppInput, AppSelect } from "@/components/forms";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import {
  useCancelTaxpayerRequest,
  useCreateTaxpayerRequest,
  useOfficeTaxpayerRequests,
} from "@/features/taxpayer-onboarding/hooks/use-taxpayer-onboarding";
import {
  KIND_LABEL_KEYS,
  REQUEST_STATUS_LABEL_KEYS,
  REQUEST_STATUS_TONES,
  normalizeTaxId,
} from "@/features/taxpayer-onboarding/lib/status";
import type {
  TaxpayerKind,
  TaxpayerRequest,
} from "@/features/taxpayer-onboarding/services/taxpayer-onboarding.service";
import { date } from "@/lib/utils/format";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const PAGE_SIZE = 25;

type FormValues = {
  kind: TaxpayerKind;
  tax_id: string;
  title: string;
  contact_email: string;
};

/**
 * "Mükellef ekle" form. Whatever the API knows about the tax id, the answer
 * is the same (D1b), so the form shows one fixed confirmation and never
 * reads anything from the response but success.
 */
function AddTaxpayerForm() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const create = useCreateTaxpayerRequest(slug);
  const [formKey, setFormKey] = useState(0);
  const [sent, setSent] = useState(false);
  const schema = useMemo(
    () =>
      z
        .object({
          kind: z.enum(["legal", "individual"]),
          tax_id: z.string(),
          title: z.string().trim().min(1, t("onboarding.form_title_required")),
          contact_email: z.email(t("onboarding.form_email_invalid")),
        })
        .superRefine((values, ctx) => {
          // Format only (10 or 11 digits); the API checks the check digit.
          if (!/^\d{10,11}$/.test(normalizeTaxId(values.tax_id))) {
            ctx.addIssue({
              code: "custom",
              path: ["tax_id"],
              message: t("onboarding.form_tax_id_invalid"),
            });
          }
        }),
    [t],
  );

  return (
    <div className="space-y-4">
      {sent ? (
        <Alert role="status">
          <MailCheck className="size-4" />
          <AlertTitle>{t("onboarding.sent_title")}</AlertTitle>
          <AlertDescription>
            {t("onboarding.sent_description")}
          </AlertDescription>
        </Alert>
      ) : null}
      <AppForm
        key={formKey}
        id="taxpayer-request-form"
        schema={schema}
        defaultValues={{
          kind: "legal",
          tax_id: "",
          title: "",
          contact_email: "",
        }}
        onSubmit={async (values: FormValues) => {
          setSent(false);
          await create.mutateAsync({
            kind: values.kind,
            tax_id: normalizeTaxId(values.tax_id),
            title: values.title.trim(),
            contact_email: values.contact_email.trim(),
          });
          setSent(true);
          setFormKey((k) => k + 1);
        }}
      >
        {(form) => (
          <div className="grid gap-3 sm:grid-cols-2">
            <AppSelect
              name="kind"
              label={t("onboarding.form_kind")}
              options={[
                { value: "legal", label: t("onboarding.kind_legal") },
                { value: "individual", label: t("onboarding.kind_individual") },
              ]}
            />
            <AppInput
              name="tax_id"
              label={t("onboarding.form_tax_id")}
              description={t("onboarding.form_tax_id_hint")}
              inputMode="numeric"
              autoComplete="off"
              maxLength={20}
            />
            <AppInput
              name="title"
              label={t("onboarding.form_title")}
              autoComplete="off"
            />
            <AppInput
              name="contact_email"
              type="email"
              label={t("onboarding.form_email")}
              description={t("onboarding.form_email_hint")}
              autoComplete="off"
            />
            <div className="sm:col-span-2">
              <Button
                type="submit"
                disabled={create.isPending || form.formState.isSubmitting}
              >
                <Send className="size-4" />
                {t("onboarding.form_submit")}
              </Button>
            </div>
          </div>
        )}
      </AppForm>
    </div>
  );
}

function RequestRow({
  request,
  canWrite,
}: {
  request: TaxpayerRequest;
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { confirm } = useDialogs();
  const cancel = useCancelTaxpayerRequest(slug);

  const onCancel = async () => {
    const ok = await confirm({
      title: t("onboarding.cancel_title", { name: request.title }),
      description: t("onboarding.cancel_description"),
      confirmLabel: t("onboarding.cancel"),
    });
    if (ok) cancel.mutate(request.uuid);
  };

  return (
    <tr className="border-b last:border-0">
      <td className="py-2 pe-3">
        <div className="font-medium">{request.title}</div>
        <div className="text-muted-foreground text-xs">
          {t(KIND_LABEL_KEYS[request.kind])}
          {" · "}
          <span className="font-mono">{request.tax_id}</span>
        </div>
      </td>
      <td className="py-2 pe-3">{request.contact_email}</td>
      <td className="py-2 pe-3">
        <StatusChip
          label={t(REQUEST_STATUS_LABEL_KEYS[request.status])}
          tone={REQUEST_STATUS_TONES[request.status]}
        />
      </td>
      <td className="text-muted-foreground py-2 pe-3">
        {date(request.created_at, "dd.MM.yyyy", locale)}
      </td>
      <td className="text-muted-foreground py-2 pe-3">
        {request.status === "pending"
          ? date(request.expires_at, "dd.MM.yyyy", locale)
          : "—"}
      </td>
      <td className="py-2 text-end">
        {canWrite && request.status === "pending" ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={cancel.isPending}
            onClick={() => void onCancel()}
          >
            {t("onboarding.cancel")}
          </Button>
        ) : null}
      </td>
    </tr>
  );
}

function RequestsSection({ canWrite }: { canWrite: boolean }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [offset, setOffset] = useState(0);
  const query = useOfficeTaxpayerRequests(slug, { limit: PAGE_SIZE, offset });
  const items = query.data?.items ?? [];
  const total = query.data?.total ?? 0;

  return (
    <EntitySectionCard title={t("onboarding.list_title")} badge={total}>
      <p className="text-muted-foreground mb-4 text-sm">
        {t("onboarding.list_description")}
      </p>
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data && items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("onboarding.list_empty")}
        </p>
      ) : null}
      {items.length ? (
        <div className="relative overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-muted-foreground border-b text-xs">
                <th className="py-2 pe-3 text-start font-medium">
                  {t("onboarding.col_taxpayer")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("onboarding.col_email")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("onboarding.col_status")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("onboarding.col_created")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("onboarding.col_expires")}
                </th>
                <th className="py-2" />
              </tr>
            </thead>
            <tbody>
              {items.map((r) => (
                <RequestRow key={r.uuid} request={r} canWrite={canWrite} />
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
      {total > PAGE_SIZE ? (
        <div className="mt-4 flex items-center justify-end gap-2 text-sm">
          <span className="text-muted-foreground">
            {t("onboarding.list_range", {
              from: offset + 1,
              to: Math.min(offset + PAGE_SIZE, total),
              total,
            })}
          </span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={offset === 0 || query.isFetching}
            onClick={() => setOffset((o) => Math.max(0, o - PAGE_SIZE))}
          >
            {t("onboarding.list_prev")}
          </Button>
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={offset + PAGE_SIZE >= total || query.isFetching}
            onClick={() => setOffset((o) => o + PAGE_SIZE)}
          >
            {t("onboarding.list_next")}
          </Button>
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

/** `/t/{slug}/taxpayer-requests` — add a mükellef, follow the requests. */
export function TaxpayerRequestsPage() {
  const { t } = useLocale();
  const tenant = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.engagements.write);

  return (
    <EntityPage
      title={t("onboarding.page_title")}
      description={t("onboarding.page_description")}
      permission={permissions.tenant.engagements.read}
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
        { label: t("onboarding.page_title") },
      ]}
    >
      <div className="space-y-6">
        {canWrite ? (
          <EntitySectionCard title={t("onboarding.form_section")}>
            <p className="text-muted-foreground mb-4 text-sm">
              {t("onboarding.form_description")}
            </p>
            <AddTaxpayerForm />
          </EntitySectionCard>
        ) : null}
        <RequestsSection canWrite={canWrite} />
      </div>
    </EntityPage>
  );
}
