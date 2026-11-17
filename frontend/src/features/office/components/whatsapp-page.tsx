"use client";

import Link from "next/link";
import { useEffect, useMemo, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import {
  EntityPage,
  EntitySectionCard,
  EntityTable,
  useServerListState,
} from "@/components/entity";
import {
  createActionsColumn,
  createColumn,
  createDateColumn,
  createStatusColumn,
  type StatusVariant,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { TenantForbidden } from "@/features/tenant/components/tenant-forbidden";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { fileSize } from "@/lib/utils/format";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";
import { appToast } from "@/providers/toast-provider";

import { formatChatKey, receiptStatusKey } from "../lib/whatsapp";
import {
  useCreateWhatsAppPairing,
  useDeleteWhatsAppLink,
  useWhatsAppCaptures,
  useWhatsAppLinks,
} from "../hooks/use-whatsapp";
import {
  whatsappService,
  type WhatsAppCapture,
  type WhatsAppChatLink,
  type WhatsAppPairing,
  type WhatsAppTarget,
} from "../services/whatsapp.service";

const TARGETS: WhatsAppTarget[] = ["expense", "income", "document"];

const targetVariants: Record<WhatsAppTarget, StatusVariant> = {
  expense: { labelKey: "office.whatsapp.target_expense", tone: "warn" },
  income: { labelKey: "office.whatsapp.target_income", tone: "ok" },
  document: { labelKey: "office.whatsapp.target_document", tone: "info" },
};

/** API adresi: /api/client-env ile çalışma anında gelir (PUBLIC_API_URL). */
function publicApiUrl(): string {
  if (typeof window === "undefined") return "";
  const env = (window as { __APP_ENV__?: { publicApiUrl?: string } })
    .__APP_ENV__;
  return env?.publicApiUrl ?? "";
}

/**
 * `/t/{slug}/whatsapp` (#388): WhatsApp Web uzantısının eşleştirmesi, ofisin
 * uzantıdan kaydettiği dosyalar ve hatırlanan sohbet → mükellef eşlemeleri.
 */
export function WhatsAppPage() {
  const { t } = useLocale();
  const tenant = useTenant();
  return (
    <EntityPage
      title={t("office.whatsapp.title")}
      description={t("office.whatsapp.description")}
      permission={permissions.tenant.receipts.read}
      forbiddenFallback={<TenantForbidden />}
      breadcrumbs={[
        {
          label: tenant.organization.name,
          href: routes.tenant.home(tenant.slug),
        },
        { label: t("office.whatsapp.title") },
      ]}
    >
      <div className="flex min-w-0 flex-col gap-6">
        <PairingCard />
        <CapturesSection />
        <LinksSection />
      </div>
    </EntityPage>
  );
}

function useCountdown(until: string | undefined): number {
  const [now, setNow] = useState(() => Date.now());
  useEffect(() => {
    if (!until) return;
    const id = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(id);
  }, [until]);
  return until ? Math.max(0, Math.floor((Date.parse(until) - now) / 1000)) : 0;
}

export function PairingCard() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const create = useCreateWhatsAppPairing(slug);
  const [pairing, setPairing] = useState<WhatsAppPairing | null>(null);
  const left = useCountdown(pairing?.expires_at);
  const expired = pairing !== null && left === 0;
  const api = publicApiUrl();

  const onCreate = async () => {
    try {
      setPairing(await create.mutateAsync());
    } catch {
      appToast.error(t("office.whatsapp.pair_error"));
    }
  };

  return (
    <EntitySectionCard title={t("office.whatsapp.pair_title")}>
      <div className="space-y-4">
        <p className="text-text-2 text-sm">
          {t("office.whatsapp.pair_description")}
        </p>
        <ol className="text-text-2 list-decimal space-y-1 pl-5 text-sm">
          <li>{t("office.whatsapp.pair_step_install")}</li>
          <li>{t("office.whatsapp.pair_step_code")}</li>
          <li>{t("office.whatsapp.pair_step_enter")}</li>
        </ol>
        {can(permissions.tenant.receipts.write) ? (
          <div className="flex flex-wrap items-center gap-4">
            <Button
              type="button"
              onClick={() => void onCreate()}
              disabled={create.isPending}
            >
              {pairing && !expired
                ? t("office.whatsapp.pair_new")
                : t("office.whatsapp.pair_create")}
            </Button>
            {pairing && !expired ? (
              <div aria-live="polite" className="space-y-1">
                <code
                  data-testid="whatsapp-pairing-code"
                  className="bg-muted block rounded-md px-4 py-2 font-mono text-2xl font-semibold tracking-[0.2em] tabular-nums"
                >
                  {pairing.code}
                </code>
                <p className="text-text-2 text-xs">
                  {t("office.whatsapp.pair_expires", {
                    minutes: Math.floor(left / 60),
                    seconds: String(left % 60).padStart(2, "0"),
                  })}
                </p>
              </div>
            ) : expired ? (
              <p className="text-text-2 text-sm" role="status">
                {t("office.whatsapp.pair_expired")}
              </p>
            ) : null}
          </div>
        ) : (
          <p className="text-text-2 text-sm">
            {t("office.whatsapp.pair_no_permission")}
          </p>
        )}
        <dl className="grid gap-1 text-sm sm:grid-cols-[max-content_minmax(0,1fr)] sm:gap-x-4">
          <dt className="text-text-2">{t("office.whatsapp.pair_server")}</dt>
          <dd className="min-w-0 break-all">
            {api ? (
              <code className="text-sm">{api}</code>
            ) : (
              <span className="text-text-2">
                {t("office.whatsapp.pair_server_unknown")}
              </span>
            )}
          </dd>
        </dl>
        <p className="text-text-2 text-xs">
          {t("office.whatsapp.pair_privacy")}{" "}
          <Link
            href={routes.help.guide("whatsapp-uzantisi")}
            className="text-brand font-medium hover:underline"
          >
            {t("office.whatsapp.guide_link")}
          </Link>
        </p>
      </div>
    </EntitySectionCard>
  );
}

function resultLabel(
  c: WhatsAppCapture,
  t: (key: string, vars?: Record<string, string | number>) => string,
): string {
  if (c.task_title)
    return t("office.whatsapp.result_task", { title: c.task_title });
  if (c.receipt_status) return t(receiptStatusKey(c.receipt_status));
  return c.has_file
    ? t("office.whatsapp.result_stored")
    : t("office.whatsapp.result_none");
}

function CapturesSection() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [target, setTarget] = useState<WhatsAppTarget | "">("");
  const list = useServerListState({ initialPageSize: 20 });
  const query = useWhatsAppCaptures(slug, {
    target: target || undefined,
    limit: list.params.limit,
    offset: list.params.offset,
  });
  const pageCount = Math.max(
    1,
    Math.ceil((query.data?.total ?? 0) / (list.pagination.pageSize || 20)),
  );

  const columns = useMemo<ColumnDef<WhatsAppCapture, unknown>[]>(
    () => [
      createDateColumn<WhatsAppCapture>({
        id: "created_at",
        labelKey: "office.whatsapp.col_date",
        accessor: (c) => c.created_at,
        style: "datetime",
        enableSorting: false,
      }),
      createColumn<WhatsAppCapture>({
        id: "taxpayer",
        labelKey: "office.whatsapp.col_taxpayer",
        accessorFn: (c) => c.taxpayer.title,
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <Link
            href={routes.tenant.engagement(
              slug,
              row.original.taxpayer.engagement_uuid,
            )}
            className="font-medium hover:underline"
          >
            {row.original.taxpayer.title}
          </Link>
        ),
      }),
      createStatusColumn<WhatsAppCapture>({
        id: "target",
        labelKey: "office.whatsapp.col_target",
        accessor: (c) => c.target,
        variants: targetVariants,
        enableSorting: false,
      }),
      createColumn<WhatsAppCapture>({
        id: "file",
        labelKey: "office.whatsapp.col_file",
        accessorFn: (c) => c.file_name,
        enableSorting: false,
        cell: ({ row }) => (
          <span className="break-all">
            {row.original.file_name}
            <span className="text-text-2 ml-1 text-xs tabular-nums">
              {fileSize(row.original.size_bytes)}
            </span>
          </span>
        ),
      }),
      createColumn<WhatsAppCapture>({
        id: "result",
        labelKey: "office.whatsapp.col_result",
        accessorFn: (c) => resultLabel(c, t),
        enableSorting: false,
      }),
      createColumn<WhatsAppCapture>({
        id: "created_by",
        labelKey: "office.whatsapp.col_created_by",
        accessorFn: (c) => c.created_by_name,
        enableSorting: false,
      }),
      createActionsColumn<WhatsAppCapture>({
        cell: (c) =>
          c.has_file ? (
            <Button
              size="sm"
              variant="ghost"
              onClick={() =>
                void whatsappService
                  .downloadFile(slug, c)
                  .catch(() =>
                    appToast.error(t("office.whatsapp.download_error")),
                  )
              }
            >
              {t("office.whatsapp.download")}
            </Button>
          ) : null,
      }),
    ],
    [slug, t],
  );

  return (
    <section aria-labelledby="whatsapp-captures" className="space-y-3">
      <h2
        id="whatsapp-captures"
        className="font-display text-lg font-semibold tracking-tight"
      >
        {t("office.whatsapp.captures_title")}
      </h2>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(c) => c.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("office.whatsapp.captures_empty_title")}
        emptyDescription={t("office.whatsapp.captures_empty_description")}
        pageCount={pageCount}
        state={list.tableState}
        features={{
          persistKey: "office-whatsapp-captures-v1",
          rowSelection: false,
          columnFilters: false,
          globalFilter: false,
        }}
        toolbarExtra={
          <div
            role="group"
            aria-label={t("office.whatsapp.filter_label")}
            className="flex flex-wrap gap-1"
          >
            {(["", ...TARGETS] as const).map((v) => (
              <Button
                key={v || "all"}
                size="sm"
                variant={target === v ? "default" : "outline"}
                aria-pressed={target === v}
                onClick={() => {
                  setTarget(v);
                  list.setPagination((p) => ({ ...p, pageIndex: 0 }));
                }}
              >
                {v
                  ? t(targetVariants[v].labelKey)
                  : t("office.whatsapp.filter_all")}
              </Button>
            ))}
          </div>
        }
      />
    </section>
  );
}

function LinksSection() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const { confirmDelete } = useDialogs();
  const remove = useDeleteWhatsAppLink(slug);
  const list = useServerListState({ initialPageSize: 20 });
  const query = useWhatsAppLinks(slug, {
    limit: list.params.limit,
    offset: list.params.offset,
  });
  const pageCount = Math.max(
    1,
    Math.ceil((query.data?.total ?? 0) / (list.pagination.pageSize || 20)),
  );
  const canWrite = can(permissions.tenant.receipts.write);

  const columns = useMemo<ColumnDef<WhatsAppChatLink, unknown>[]>(() => {
    const cols: ColumnDef<WhatsAppChatLink, unknown>[] = [
      createColumn<WhatsAppChatLink>({
        id: "chat",
        labelKey: "office.whatsapp.col_chat",
        accessorFn: (l) => l.label || l.chat_key,
        enableSorting: false,
        gridPrimary: true,
        cell: ({ row }) => (
          <span>
            <span className="font-medium">
              {row.original.label || formatChatKey(row.original.chat_key)}
            </span>
            {row.original.label ? (
              <span className="text-text-2 ml-2 text-xs tabular-nums">
                {formatChatKey(row.original.chat_key)}
              </span>
            ) : null}
          </span>
        ),
      }),
      createColumn<WhatsAppChatLink>({
        id: "taxpayer",
        labelKey: "office.whatsapp.col_taxpayer",
        accessorFn: (l) => l.taxpayer.title,
        enableSorting: false,
      }),
      createDateColumn<WhatsAppChatLink>({
        id: "updated_at",
        labelKey: "office.whatsapp.col_updated",
        accessor: (l) => l.updated_at,
        style: "datetime",
        enableSorting: false,
      }),
    ];
    if (canWrite) {
      cols.push(
        createActionsColumn<WhatsAppChatLink>({
          cell: (l) => (
            <Button
              size="sm"
              variant="ghost"
              disabled={remove.isPending}
              onClick={async () => {
                const ok = await confirmDelete({
                  title: t("office.whatsapp.unlink_title"),
                  description: t("office.whatsapp.unlink_description", {
                    taxpayer: l.taxpayer.title,
                  }),
                  confirmLabel: t("office.whatsapp.unlink"),
                });
                if (ok) remove.mutate(l.uuid);
              }}
            >
              {t("office.whatsapp.unlink")}
            </Button>
          ),
        }),
      );
    }
    return cols;
  }, [canWrite, confirmDelete, remove, t]);

  return (
    <section aria-labelledby="whatsapp-links" className="space-y-3">
      <div>
        <h2
          id="whatsapp-links"
          className="font-display text-lg font-semibold tracking-tight"
        >
          {t("office.whatsapp.links_title")}
        </h2>
        <p className="text-text-2 text-sm">
          {t("office.whatsapp.links_description")}
        </p>
      </div>
      <EntityTable
        columns={columns}
        data={query.data?.items ?? []}
        getRowId={(l) => l.uuid}
        isLoading={query.isLoading}
        isError={query.isError}
        onRetry={() => void query.refetch()}
        emptyTitle={t("office.whatsapp.links_empty_title")}
        emptyDescription={t("office.whatsapp.links_empty_description")}
        pageCount={pageCount}
        state={list.tableState}
        features={{
          persistKey: "office-whatsapp-links-v1",
          rowSelection: false,
          columnFilters: false,
          globalFilter: false,
        }}
      />
    </section>
  );
}
