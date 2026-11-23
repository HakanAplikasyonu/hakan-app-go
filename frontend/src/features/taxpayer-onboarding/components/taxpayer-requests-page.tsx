"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { FileScan, MailCheck, Send } from "lucide-react";
import { Suspense, useMemo, useRef, useState } from "react";
import { useFormContext } from "react-hook-form";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import {
  EntityPage,
  EntitySectionCard,
  EntityTable,
  useUrlListState,
} from "@/components/entity";
import { AppForm, AppInput, AppSelect } from "@/components/forms";
import {
  createActionsColumn,
  createColumn,
  createDateColumn,
} from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { TAX_CERTIFICATE_AI_SETTING } from "@/features/tenant/components/tax-certificate-ai-setting";
import { useTenantSettings } from "@/features/tenant/hooks/use-tenant-admin";
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
import {
  officeTaxpayerRequestService,
  type TaxCertificateFieldName,
  type TaxCertificateRead,
  type TaxpayerKind,
  type TaxpayerRequest,
} from "@/features/taxpayer-onboarding/services/taxpayer-onboarding.service";
import { ApiError } from "@/lib/api/errors";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

const PAGE_SIZE = 25;

type FormValues = {
  kind: TaxpayerKind;
  tax_id: string;
  title: string;
  tax_office: string;
  address: string;
  activity_code: string;
  contact_email: string;
};

/** Field → why it must be checked ("kontrol et"), from the last read. */
type CertChecks = Partial<Record<TaxCertificateFieldName, string>>;

const CERT_FIELDS: TaxCertificateFieldName[] = [
  "tax_id",
  "title",
  "tax_office",
  "address",
  "activity_code",
];

const CERT_PROBLEM_KEYS: Record<string, string> = {
  missing: "onboarding.cert_problem_missing",
  low_confidence: "onboarding.cert_problem_low_confidence",
  invalid: "onboarding.cert_problem_invalid",
};

const CERT_ERROR_KEYS: Record<string, string> = {
  TAX_CERTIFICATE_READING_OFF: "onboarding.cert_error_off",
  AI_UNAVAILABLE: "onboarding.cert_error_ai",
  UNSUPPORTED_FILE: "onboarding.cert_error_file",
  SCANNED_PDF: "onboarding.cert_error_scanned_pdf",
  HEIC_NOT_SUPPORTED: "onboarding.cert_error_heic",
  TAX_CERTIFICATE_UNREADABLE: "onboarding.cert_error_unreadable",
  RATE_LIMITED: "onboarding.cert_error_rate",
};

/**
 * "Vergi levhasından doldur": the photo or PDF is read with AI and the
 * fields go into the form. Nothing is saved; the office checks the fields
 * marked "Kontrol et" and sends the form itself.
 */
function CertificateFill({
  onRead,
}: {
  onRead: (checks: CertChecks) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const settings = useTenantSettings(slug);
  const { setValue } = useFormContext<FormValues>();
  const input = useRef<HTMLInputElement>(null);
  const [pending, setPending] = useState(false);
  const [message, setMessage] = useState<{
    tone: "ok" | "error";
    text: string;
  } | null>(null);

  const values = (settings.data?.settings ?? {}) as Record<string, unknown>;
  if (values[TAX_CERTIFICATE_AI_SETTING] === false) return null;

  const apply = (read: TaxCertificateRead) => {
    const checks: CertChecks = {};
    for (const field of CERT_FIELDS) {
      const f = read.fields[field];
      if (f.value) {
        setValue(field, f.value, { shouldDirty: true, shouldValidate: true });
      }
      if (f.check) checks[field] = f.problem ?? "missing";
    }
    if (read.kind === "legal" || read.kind === "individual") {
      setValue("kind", read.kind, { shouldDirty: true });
    }
    onRead(checks);
    setMessage({
      tone: "ok",
      text: t(
        read.check_fields.length > 0
          ? "onboarding.cert_done_check"
          : "onboarding.cert_done",
      ),
    });
  };

  const onFile = async (file: File | undefined) => {
    if (!file) return;
    setPending(true);
    setMessage(null);
    try {
      apply(await officeTaxpayerRequestService.readTaxCertificate(slug, file));
    } catch (error) {
      const key =
        error instanceof ApiError ? CERT_ERROR_KEYS[error.code] : undefined;
      setMessage({
        tone: "error",
        text: t(key ?? "onboarding.cert_error_generic"),
      });
    } finally {
      setPending(false);
      if (input.current) input.current.value = "";
    }
  };

  return (
    <div
      className="bg-muted/40 space-y-2 rounded-md border p-3 sm:col-span-2"
      data-testid="tax-certificate-fill"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="space-y-1">
          <p className="text-sm font-medium">{t("onboarding.cert_title")}</p>
          <p className="text-muted-foreground text-sm">
            {t("onboarding.cert_description")}
          </p>
        </div>
        <input
          ref={input}
          type="file"
          className="sr-only"
          accept="image/jpeg,image/png,image/webp,image/heic,.heic,application/pdf"
          aria-label={t("onboarding.cert_file_label")}
          onChange={(e) => void onFile(e.target.files?.[0])}
        />
        <Button
          type="button"
          variant="outline"
          disabled={pending}
          onClick={() => input.current?.click()}
        >
          <FileScan className="size-4" />
          {pending ? t("onboarding.cert_reading") : t("onboarding.cert_button")}
        </Button>
      </div>
      {message ? (
        <p
          role={message.tone === "error" ? "alert" : "status"}
          className={
            message.tone === "error"
              ? "text-destructive text-sm"
              : "text-muted-foreground text-sm"
          }
        >
          {message.text}
        </p>
      ) : null}
    </div>
  );
}

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
  const [checks, setChecks] = useState<CertChecks>({});
  const schema = useMemo(
    () =>
      z
        .object({
          kind: z.enum(["legal", "individual"]),
          tax_id: z.string(),
          title: z.string().trim().min(1, t("onboarding.form_title_required")),
          tax_office: z.string().trim().max(128),
          address: z.string().trim().max(500),
          activity_code: z
            .string()
            .trim()
            .regex(
              /^(\d{6}|\d{2}\.\d{2}\.\d{2})?$/,
              t("onboarding.form_activity_code_invalid"),
            ),
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
          tax_office: "",
          address: "",
          activity_code: "",
          contact_email: "",
        }}
        onSubmit={async (values: FormValues) => {
          setSent(false);
          await create.mutateAsync({
            kind: values.kind,
            tax_id: normalizeTaxId(values.tax_id),
            title: values.title.trim(),
            tax_office: values.tax_office.trim(),
            address: values.address.trim(),
            activity_code: values.activity_code.trim(),
            contact_email: values.contact_email.trim(),
          });
          setSent(true);
          setChecks({});
          setFormKey((k) => k + 1);
        }}
      >
        {(form) => (
          <div className="grid gap-3 sm:grid-cols-2">
            <CertificateFill onRead={setChecks} />
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
              labelAction={<CheckChip problem={checks.tax_id} />}
              description={
                checkText(t, checks.tax_id) ?? t("onboarding.form_tax_id_hint")
              }
              inputMode="numeric"
              autoComplete="off"
              maxLength={20}
            />
            <AppInput
              name="title"
              label={t("onboarding.form_title")}
              labelAction={<CheckChip problem={checks.title} />}
              description={checkText(t, checks.title)}
              autoComplete="off"
            />
            <AppInput
              name="tax_office"
              label={t("onboarding.form_tax_office")}
              labelAction={<CheckChip problem={checks.tax_office} />}
              description={checkText(t, checks.tax_office)}
              autoComplete="off"
              maxLength={128}
            />
            <AppInput
              name="activity_code"
              label={t("onboarding.form_activity_code")}
              labelAction={<CheckChip problem={checks.activity_code} />}
              description={
                checkText(t, checks.activity_code) ??
                t("onboarding.form_activity_code_hint")
              }
              inputMode="numeric"
              autoComplete="off"
              maxLength={8}
            />
            <AppInput
              name="address"
              label={t("onboarding.form_address")}
              labelAction={<CheckChip problem={checks.address} />}
              description={checkText(t, checks.address)}
              autoComplete="off"
              maxLength={500}
              className="sm:col-span-2"
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

/** "Kontrol et" next to a field the certificate read flagged. */
function CheckChip({ problem }: { problem?: string }) {
  const { t } = useLocale();
  if (!problem) return null;
  return (
    <span data-testid="cert-check">
      <StatusChip label={t("onboarding.cert_check")} tone="warning" />
    </span>
  );
}

function checkText(
  t: (key: string) => string,
  problem: string | undefined,
): string | undefined {
  if (!problem) return undefined;
  return t(CERT_PROBLEM_KEYS[problem] ?? "onboarding.cert_problem_missing");
}

/** Bekleyen talebi iptal eder; onay penceresi ve bekleme durumu satır başına. */
function CancelRequestButton({ request }: { request: TaxpayerRequest }) {
  const { t } = useLocale();
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
    <Button
      type="button"
      size="sm"
      variant="ghost"
      disabled={cancel.isPending}
      onClick={() => void onCancel()}
    >
      {t("onboarding.cancel")}
    </Button>
  );
}

function useRequestColumns(canWrite: boolean) {
  const { t } = useLocale();
  return useMemo<ColumnDef<TaxpayerRequest, unknown>[]>(
    () => [
      createColumn<TaxpayerRequest>({
        id: "taxpayer",
        labelKey: "onboarding.col_taxpayer",
        accessorKey: "title",
        gridPrimary: true,
        cell: ({ row }) => (
          <>
            <div className="font-medium">{row.original.title}</div>
            <div className="text-muted-foreground text-xs">
              {t(KIND_LABEL_KEYS[row.original.kind])}
              {" · "}
              <span className="font-mono">{row.original.tax_id}</span>
            </div>
          </>
        ),
      }),
      createColumn<TaxpayerRequest>({
        id: "contact_email",
        labelKey: "onboarding.col_email",
        accessorKey: "contact_email",
        gridSecondary: true,
      }),
      createColumn<TaxpayerRequest>({
        id: "status",
        labelKey: "onboarding.col_status",
        accessorKey: "status",
        cell: ({ row }) => (
          <StatusChip
            label={t(REQUEST_STATUS_LABEL_KEYS[row.original.status])}
            tone={REQUEST_STATUS_TONES[row.original.status]}
          />
        ),
      }),
      createDateColumn<TaxpayerRequest>({
        id: "created_at",
        labelKey: "onboarding.col_created",
        accessor: (r) => r.created_at,
        className: "text-muted-foreground",
      }),
      createDateColumn<TaxpayerRequest>({
        id: "expires_at",
        labelKey: "onboarding.col_expires",
        // Süre yalnızca bekleyen talepte anlamlı; diğerlerinde "—".
        accessor: (r) => (r.status === "pending" ? r.expires_at : null),
        className: "text-muted-foreground",
      }),
      createActionsColumn<TaxpayerRequest>({
        cell: (r) =>
          canWrite && r.status === "pending" ? (
            <CancelRequestButton request={r} />
          ) : null,
      }),
    ],
    [canWrite, t],
  );
}

function RequestsSection({ canWrite }: { canWrite: boolean }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  // Sayfa ve sayfa boyutu URL'de (#381).
  const listState = useUrlListState({ initialPageSize: PAGE_SIZE });
  const query = useOfficeTaxpayerRequests(slug, {
    limit: listState.params.limit,
    offset: listState.params.offset,
  });
  const total = query.data?.total ?? 0;
  const columns = useRequestColumns(canWrite);

  return (
    <EntitySectionCard title={t("onboarding.list_title")} badge={total}>
      <p className="text-muted-foreground mb-4 text-sm">
        {t("onboarding.list_description")}
      </p>
      <EntityTable<TaxpayerRequest>
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(r) => r.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorTitle={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("onboarding.list_empty")}
        emptyDescription=""
        state={listState.tableState}
        rowCount={total}
        pageSizeOptions={[25, 50, 100]}
        bordered={false}
        // Sunucu sıralama ve arama sunmuyor; yalnızca sayfalama.
        features={{
          toolbar: false,
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          columnVisibility: false,
          columnResizing: false,
          columnPinning: false,
          columnOrdering: false,
          rowSelection: false,
          density: false,
          viewMode: false,
          facetedFilters: false,
        }}
      />
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
        <Suspense fallback={<Loading label={t("common.loading")} />}>
          <RequestsSection canWrite={canWrite} />
        </Suspense>
      </div>
    </EntityPage>
  );
}
