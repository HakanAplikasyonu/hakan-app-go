"use client";

import { AlertTriangle, Eye, FileDown, FileText, Save } from "lucide-react";
import Link from "next/link";
import { useMemo, useState } from "react";
import { toast } from "sonner";
import type { ColumnDef, PaginationState } from "@tanstack/react-table";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import {
  EntityPage,
  EntitySectionCard,
  EntityTable,
} from "@/components/entity";
import { createActionsColumn, createColumn } from "@/components/tables";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useCreatePetition,
  usePetitions,
  usePetitionTemplates,
  usePreviewPetition,
} from "@/features/petitions/hooks/use-petitions";
import {
  PETITION_CATEGORIES,
  petitionService,
  type Petition,
  type PetitionCategory,
  type PetitionField,
  type PetitionRequest,
  type PetitionTemplate,
} from "@/features/petitions/services/petitions.service";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { datetime, download } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { PetitionLetter } from "./petition-letter";

const PAGE_SIZE = 20;

const CATEGORY_LABEL_KEYS: Record<PetitionCategory, string> = {
  vergi_dairesi: "petitions.category_vergi_dairesi",
  ticaret_sicil: "petitions.category_ticaret_sicil",
  genel: "petitions.category_genel",
};

type FieldValues = Record<string, string>;
type FieldErrors = Record<string, string>;
type Translate = ReturnType<typeof useLocale>["t"];

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

function validate(
  template: PetitionTemplate,
  values: FieldValues,
  t: Translate,
): FieldErrors {
  const errors: FieldErrors = {};
  for (const field of template.fields) {
    const value = (values[field.key] ?? "").trim();
    if (field.required && !value) {
      errors[field.key] = t("petitions.field_required");
    } else if (field.max_length > 0 && value.length > field.max_length) {
      errors[field.key] = t("petitions.field_too_long", {
        max: field.max_length,
      });
    } else if (
      field.type === "date" &&
      value &&
      !/^\d{4}-\d{2}-\d{2}$/.test(value)
    ) {
      errors[field.key] = t("petitions.field_invalid_date");
    }
  }
  return errors;
}

function buildRequest(
  template: PetitionTemplate,
  values: FieldValues,
): PetitionRequest {
  const fields: Record<string, string> = {};
  for (const field of template.fields) {
    const value = (values[field.key] ?? "").trim();
    if (value) fields[field.key] = value;
  }
  return { template_key: template.key, fields };
}

/**
 * Opens a PDF blob in a tab. The tab is opened synchronously in the click
 * handler (`pending`) so pop-up blockers allow it; when that fails the PDF is
 * downloaded instead.
 */
function showPdf(blob: Blob, pending: Window | null, filename: string) {
  const url = URL.createObjectURL(blob);
  if (pending && !pending.closed) {
    pending.location.href = url;
    window.setTimeout(() => URL.revokeObjectURL(url), 60_000);
    return;
  }
  URL.revokeObjectURL(url);
  download(blob, filename);
}

function openPendingTab() {
  // Not `noopener`: the handle is needed to point the tab at the PDF.
  return window.open("", "_blank");
}

function pdfFilename(petition: Petition) {
  return `dilekce-${petition.template_key}-${petition.uuid.slice(0, 8)}.pdf`;
}

function PetitionFieldInput({
  field,
  value,
  error,
  disabled,
  onChange,
}: {
  field: PetitionField;
  value: string;
  error?: string;
  disabled?: boolean;
  onChange: (value: string) => void;
}) {
  const { t } = useLocale();
  const id = `petition-field-${field.key}`;
  const describedBy = [
    field.hint ? `${id}-hint` : null,
    error ? `${id}-error` : null,
  ]
    .filter(Boolean)
    .join(" ");
  const common = {
    id,
    value,
    disabled,
    required: field.required,
    maxLength: field.max_length > 0 ? field.max_length : undefined,
    "aria-invalid": error ? true : undefined,
    "aria-describedby": describedBy || undefined,
  };

  return (
    <div className="space-y-1.5">
      <Label htmlFor={id}>
        {field.label}
        {field.required ? (
          <span className="text-destructive ms-0.5" aria-hidden>
            *
          </span>
        ) : (
          <span className="text-muted-foreground ms-1 text-xs font-normal">
            {t("petitions.field_optional")}
          </span>
        )}
      </Label>
      {field.type === "textarea" ? (
        <Textarea
          {...common}
          rows={5}
          onChange={(e) => onChange(e.target.value)}
        />
      ) : (
        <Input
          {...common}
          type={field.type === "date" ? "date" : "text"}
          onChange={(e) => onChange(e.target.value)}
        />
      )}
      <div className="flex items-start justify-between gap-3 text-xs">
        <div className="space-y-0.5">
          {field.hint ? (
            <p id={`${id}-hint`} className="text-muted-foreground">
              {field.hint}
            </p>
          ) : null}
          {error ? (
            <p id={`${id}-error`} className="text-destructive">
              {error}
            </p>
          ) : null}
        </div>
        {field.type !== "date" && field.max_length > 0 ? (
          <span className="text-muted-foreground shrink-0 tabular-nums">
            {value.length}/{field.max_length}
          </span>
        ) : null}
      </div>
    </div>
  );
}

function DraftBanner({ notice }: { notice: string }) {
  const { t } = useLocale();
  return (
    <Alert className="border-amber-500/60 bg-amber-50 text-amber-900 dark:bg-amber-950/40 dark:text-amber-100">
      <AlertTriangle />
      <AlertTitle>{t("petitions.draft_banner_title")}</AlertTitle>
      <AlertDescription className="text-amber-900/90 dark:text-amber-100/90">
        {notice}
      </AlertDescription>
    </Alert>
  );
}

function Composer({
  engagement,
  templates,
  draftNotice,
  canWrite,
}: {
  engagement: string;
  templates: PetitionTemplate[];
  draftNotice: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [templateKey, setTemplateKey] = useState<string>("");
  const [values, setValues] = useState<FieldValues>({});
  const [errors, setErrors] = useState<FieldErrors>({});
  const [previewedBody, setPreviewedBody] = useState<string | null>(null);
  const [pdfLoading, setPdfLoading] = useState(false);
  const preview = usePreviewPetition(slug, engagement);
  const create = useCreatePetition(slug, engagement);

  const template = templates.find((tpl) => tpl.key === templateKey);
  const grouped = useMemo(
    () =>
      PETITION_CATEGORIES.map((category) => ({
        category,
        items: templates.filter((tpl) => tpl.category === category),
      })).filter((group) => group.items.length > 0),
    [templates],
  );

  const currentBody = template
    ? JSON.stringify(buildRequest(template, values))
    : null;
  const rendered = preview.data;
  const previewStale =
    rendered !== undefined &&
    previewedBody !== null &&
    previewedBody !== currentBody;
  const isDraft = Boolean(template?.draft || rendered?.draft);
  const busy = preview.isPending || create.isPending || pdfLoading;

  function selectTemplate(key: string) {
    setTemplateKey(key);
    setValues({});
    setErrors({});
    setPreviewedBody(null);
    preview.reset();
    create.reset();
  }

  function setValue(key: string, value: string) {
    setValues((prev) => ({ ...prev, [key]: value }));
    setErrors((prev) => {
      if (!prev[key]) return prev;
      const next = { ...prev };
      delete next[key];
      return next;
    });
  }

  function checkForm(): PetitionRequest | null {
    if (!template) return null;
    const found = validate(template, values, t);
    setErrors(found);
    if (Object.keys(found).length > 0) return null;
    return buildRequest(template, values);
  }

  function onPreview() {
    const body = checkForm();
    if (!body) return;
    create.reset();
    preview.mutate(body, {
      onSuccess: () => setPreviewedBody(JSON.stringify(body)),
    });
  }

  async function onSaveAndPdf() {
    const body = checkForm();
    if (!body) return;
    const pending = openPendingTab();
    try {
      const petition = await create.mutateAsync(body);
      toast.success(t("petitions.saved_toast"));
      setPdfLoading(true);
      try {
        const blob = await petitionService.pdf(slug, engagement, petition.uuid);
        showPdf(blob, pending, pdfFilename(petition));
      } catch (error) {
        pending?.close();
        toast.error(errorMessage(error, t("petitions.pdf_failed")));
      } finally {
        setPdfLoading(false);
      }
    } catch {
      // The error is rendered inline below the form.
      pending?.close();
    }
  }

  const actionError = preview.error ?? create.error;

  return (
    <div className="space-y-4">
      {isDraft ? (
        <DraftBanner notice={rendered?.draft_notice || draftNotice} />
      ) : null}
      <div className="grid gap-6 lg:grid-cols-[minmax(0,2fr)_minmax(0,3fr)]">
        <EntitySectionCard title={t("petitions.form_title")}>
          <div className="space-y-5">
            <div className="space-y-1.5">
              <Label htmlFor="petition-template">
                {t("petitions.template_label")}
              </Label>
              <Select value={templateKey} onValueChange={selectTemplate}>
                <SelectTrigger id="petition-template">
                  <SelectValue
                    placeholder={t("petitions.template_placeholder")}
                  />
                </SelectTrigger>
                <SelectContent>
                  {grouped.map((group) => (
                    <SelectGroup key={group.category}>
                      <SelectLabel>
                        {t(CATEGORY_LABEL_KEYS[group.category])}
                      </SelectLabel>
                      {group.items.map((tpl) => (
                        <SelectItem key={tpl.key} value={tpl.key}>
                          {tpl.title}
                        </SelectItem>
                      ))}
                    </SelectGroup>
                  ))}
                </SelectContent>
              </Select>
              {template ? (
                <div className="flex flex-wrap items-center gap-2 pt-1 text-xs">
                  <Badge variant="outline">
                    {t(CATEGORY_LABEL_KEYS[template.category])}
                  </Badge>
                  <span className="text-muted-foreground">
                    {t("petitions.template_version", {
                      version: template.version,
                    })}
                  </span>
                  {template.draft ? (
                    <Badge variant="warning">
                      {t("petitions.draft_badge")}
                    </Badge>
                  ) : null}
                </div>
              ) : null}
            </div>

            {template ? (
              <>
                <p className="text-muted-foreground text-xs">
                  {t("petitions.auto_filled_hint")}
                </p>
                {template.fields.length === 0 ? (
                  <p className="text-muted-foreground text-sm">
                    {t("petitions.no_fields")}
                  </p>
                ) : (
                  <div className="space-y-4">
                    {template.fields.map((field) => (
                      <PetitionFieldInput
                        key={field.key}
                        field={field}
                        value={values[field.key] ?? ""}
                        error={errors[field.key]}
                        disabled={busy}
                        onChange={(v) => setValue(field.key, v)}
                      />
                    ))}
                  </div>
                )}

                {actionError ? (
                  <p className="text-destructive text-sm" role="alert">
                    {errorMessage(actionError, t("common.error_generic"))}
                  </p>
                ) : null}

                <div className="flex flex-wrap gap-2">
                  <Button
                    type="button"
                    variant="outline"
                    onClick={onPreview}
                    disabled={busy}
                  >
                    <Eye className="size-4" />
                    {preview.isPending
                      ? t("petitions.previewing")
                      : t("petitions.preview_action")}
                  </Button>
                  {canWrite ? (
                    <Button
                      type="button"
                      onClick={() => void onSaveAndPdf()}
                      disabled={busy}
                    >
                      <Save className="size-4" />
                      {create.isPending || pdfLoading
                        ? t("petitions.saving")
                        : t("petitions.save_pdf_action")}
                    </Button>
                  ) : null}
                </div>
                {!canWrite ? (
                  <p className="text-muted-foreground text-xs">
                    {t("petitions.write_required")}
                  </p>
                ) : null}
              </>
            ) : (
              <p className="text-muted-foreground text-sm">
                {t("petitions.pick_template_hint")}
              </p>
            )}
          </div>
        </EntitySectionCard>

        <EntitySectionCard title={t("petitions.preview_title")}>
          {rendered && template ? (
            <div className="space-y-3">
              {previewStale ? (
                <p className="text-muted-foreground text-xs">
                  {t("petitions.preview_stale")}
                </p>
              ) : null}
              <div className="bg-muted/40 rounded-md p-3 sm:p-6">
                <PetitionLetter petition={rendered} />
              </div>
            </div>
          ) : (
            <div className="text-muted-foreground flex flex-col items-center gap-2 py-10 text-center text-sm">
              <FileText className="size-8 opacity-50" />
              <p>{t("petitions.preview_empty")}</p>
            </div>
          )}
        </EntitySectionCard>
      </div>
    </div>
  );
}

function PetitionsList({
  engagement,
  enabled,
  templates,
}: {
  engagement: string;
  enabled: boolean;
  templates: PetitionTemplate[];
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  // Toplam yok: çekirdek sayfalaması toplamsız (hasNextPage) çalışır.
  const [pagination, setPagination] = useState<PaginationState>({
    pageIndex: 0,
    pageSize: PAGE_SIZE,
  });
  const offset = pagination.pageIndex * pagination.pageSize;
  const [opening, setOpening] = useState<string | null>(null);
  const query = usePetitions(
    slug,
    engagement,
    { limit: pagination.pageSize, offset },
    enabled,
  );
  const items = query.data?.items ?? [];
  const hasNext = items.length >= pagination.pageSize;

  async function openPdf(petition: Petition) {
    const pending = openPendingTab();
    setOpening(petition.uuid);
    try {
      const blob = await petitionService.pdf(slug, engagement, petition.uuid);
      showPdf(blob, pending, pdfFilename(petition));
    } catch (error) {
      pending?.close();
      toast.error(errorMessage(error, t("petitions.pdf_failed")));
    } finally {
      setOpening(null);
    }
  }

  const columns = useMemo<ColumnDef<Petition, unknown>[]>(
    () => [
      createColumn<Petition>({
        id: "title",
        labelKey: "petitions.col_title",
        accessorFn: (p) =>
          p.rendered.title ||
          templates.find((x) => x.key === p.template_key)?.title ||
          "",
        gridPrimary: true,
        cell: ({ row, getValue }) => (
          <div className="flex flex-wrap items-center gap-2">
            <span className="font-medium">{getValue() as string}</span>
            {row.original.draft ? (
              <Badge variant="warning">{t("petitions.draft_badge")}</Badge>
            ) : null}
          </div>
        ),
      }),
      createColumn<Petition>({
        id: "subject",
        labelKey: "petitions.col_subject",
        accessorFn: (p) => p.rendered.subject,
        meta: {
          cellClassName: "text-muted-foreground max-w-[22rem] truncate",
        },
      }),
      createColumn<Petition>({
        id: "created_at",
        labelKey: "petitions.col_created",
        accessorKey: "created_at",
        gridSecondary: true,
        meta: { cellClassName: "text-muted-foreground whitespace-nowrap" },
        // Saatli biçim (dd.MM.yyyy HH:mm) ve altında oluşturan korunur.
        cell: ({ row }) => (
          <>
            {datetime(row.original.created_at, undefined, locale)}
            {row.original.created_by ? (
              <span className="block text-xs">{row.original.created_by}</span>
            ) : null}
          </>
        ),
      }),
      createActionsColumn<Petition>({
        labelKey: "petitions.col_actions",
        cell: (petition) => (
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={opening === petition.uuid}
            onClick={() => void openPdf(petition)}
          >
            <FileDown className="size-4" />
            {t("petitions.pdf_action")}
          </Button>
        ),
      }),
    ],
    // openPdf yalnızca slug/engagement'a bağlı.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [templates, opening, locale, t, slug, engagement],
  );

  return (
    <EntitySectionCard title={t("petitions.list_title")}>
      <EntityTable<Petition>
        columns={columns}
        data={items}
        getRowId={(p) => p.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        errorTitle={t("common.error_generic")}
        onRetry={() => void query.refetch()}
        emptyTitle={t("petitions.list_empty")}
        emptyDescription=""
        bordered={false}
        state={{ pagination, onPaginationChange: setPagination }}
        hasNextPage={!query.isFetching && hasNext}
        pageSizeOptions={[20, 50, 100]}
        // Sunucu toplam, sıralama ve arama vermiyor.
        features={{
          toolbar: false,
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          facetedFilters: false,
          rowSelection: false,
          viewMode: false,
          columnVisibility: false,
          columnResizing: false,
          columnPinning: false,
          columnOrdering: false,
          density: false,
        }}
      />
    </EntitySectionCard>
  );
}

/**
 * `/t/{slug}/engagements/{engagementUuid}/petitions` (issue #56):
 * template-based petitions for one mükellef. Pick a template, fill its own
 * fields, preview it as a letter, then save and open the PDF.
 */
export function EngagementPetitionsPage({
  engagementUuid,
}: {
  engagementUuid: string;
}) {
  const { t } = useLocale();
  const tenant = useTenant();
  const { can } = usePermission();
  const canRead = can(permissions.tenant.engagements.read);
  const canWrite = can(permissions.tenant.engagements.write);
  const templatesQuery = usePetitionTemplates(tenant.slug, canRead);
  const templates = templatesQuery.data?.items ?? [];

  return (
    <EntityPage
      title={t("petitions.page_title")}
      description={t("petitions.page_description")}
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
        {
          label: t("office.engagement_documents_page_title"),
          href: routes.tenant.engagementDocuments(tenant.slug, engagementUuid),
        },
        { label: t("petitions.page_title") },
      ]}
      actions={
        <Button asChild variant="outline" size="sm">
          <Link
            href={routes.tenant.engagementDocuments(
              tenant.slug,
              engagementUuid,
            )}
          >
            {t("petitions.documents_link")}
          </Link>
        </Button>
      }
    >
      <div className="space-y-6">
        {templatesQuery.isLoading ? (
          <Loading label={t("common.loading")} />
        ) : null}
        {templatesQuery.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void templatesQuery.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : null}
        {templatesQuery.data ? (
          templates.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("petitions.no_templates")}
            </p>
          ) : (
            <Composer
              engagement={engagementUuid}
              templates={templates}
              draftNotice={templatesQuery.data.draft_notice}
              canWrite={canWrite}
            />
          )
        ) : null}
        <PetitionsList
          engagement={engagementUuid}
          enabled={canRead}
          templates={templates}
        />
      </div>
    </EntityPage>
  );
}
