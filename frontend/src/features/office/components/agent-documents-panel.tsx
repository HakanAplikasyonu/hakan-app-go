"use client";

import { useMemo, useState } from "react";
import { toast } from "sonner";
import type { ColumnDef } from "@tanstack/react-table";

import { ErrorState } from "@/components/common/error-state";
import { StatusChip } from "@/components/common/status-chip";
import {
  createActionsColumn,
  createColumn,
  DataTable,
  type DataTableFeatures,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { MonthPicker } from "@/components/ui/month-picker";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { datetime, relativeDatetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  useAgentDocuments,
  useDismissAgentDocument,
  useResolveAgentDocument,
} from "../hooks/use-agents";
import { useEngagementOptions } from "../hooks/use-office-invoices";
import type {
  AgentDocument,
  AgentDocumentKind,
} from "../services/agents.service";
import { EngagementPicker } from "./engagement-picker";
import { Panel } from "./office-ui";

const KINDS: AgentDocumentKind[] = [
  "payroll",
  "trial_balance",
  "account_plan",
  "foreign_invoice",
];

const kindKey: Record<AgentDocument["kind"], string> = {
  unknown: "office.agents.docs_kind_unknown",
  payroll: "office.agents.docs_kind_payroll",
  trial_balance: "office.agents.docs_kind_trial_balance",
  account_plan: "office.agents.docs_kind_account_plan",
  foreign_invoice: "office.agents.docs_kind_foreign_invoice",
};

const panelTable: DataTableFeatures = {
  toolbar: false,
  pagination: false,
  rowSelection: false,
  columnFilters: false,
  facetedFilters: false,
  globalFilter: false,
  viewMode: false,
  columnVisibility: false,
  columnResizing: false,
  columnPinning: false,
  columnOrdering: false,
  density: false,
};

/**
 * Ajandan gelen dosyalar (#780 e): Luca bordro dökümü, mizan ve yabancı
 * fatura PDF'i. Türü, mükellefi ya da dönemi anlaşılamayanlar burada
 * "Eşleştir" ile içe aktarılır; tanınanlar kendiliğinden aktarılır.
 */
export function AgentDocumentsPanel() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canRead = can(permissions.tenant.agents.read);
  const canWrite = can(permissions.tenant.agents.write);
  const docs = useAgentDocuments(slug, canRead);
  const [matching, setMatching] = useState<AgentDocument | null>(null);
  const items = docs.data?.items ?? [];
  const columns = useDocumentColumns(canWrite, setMatching);
  if (!canRead) return null;

  return (
    <Panel
      title={t("office.agents.docs_title", {
        count: docs.data?.total ?? items.length,
      })}
      description={t("office.agents.docs_description")}
    >
      {docs.isLoading ? (
        <div className="space-y-2 p-4">
          <Skeleton className="h-8 w-full" />
          <Skeleton className="h-8 w-full" />
        </div>
      ) : docs.isError ? (
        <ErrorState
          title={t("office.agents.docs_load_error")}
          onRetry={() => void docs.refetch()}
        />
      ) : items.length === 0 ? (
        <div className="space-y-1 p-6 text-center">
          <p className="font-medium">{t("office.agents.docs_empty_title")}</p>
          <p className="text-muted-foreground text-sm">
            {t("office.agents.docs_empty_description")}
          </p>
        </div>
      ) : (
        <DataTable<AgentDocument>
          columns={columns}
          data={items}
          getRowId={(d) => d.uuid}
          bordered={false}
          features={panelTable}
        />
      )}
      {matching ? (
        <MatchDialog doc={matching} onClose={() => setMatching(null)} />
      ) : null}
    </Panel>
  );
}

function useDocumentColumns(
  canWrite: boolean,
  onMatch: (d: AgentDocument) => void,
) {
  const { t, locale } = useLocale();
  return useMemo<ColumnDef<AgentDocument, unknown>[]>(
    () => [
      createColumn<AgentDocument>({
        id: "file_name",
        labelKey: "office.agents.docs_col_file",
        accessorKey: "file_name",
        gridPrimary: true,
        cell: ({ row }) => (
          <div className="min-w-0">
            <p className="truncate font-medium">{row.original.file_name}</p>
            {row.original.folder ? (
              <p
                className="text-muted-foreground truncate text-xs"
                title={row.original.folder}
              >
                {row.original.folder}
              </p>
            ) : null}
          </div>
        ),
      }),
      createColumn<AgentDocument>({
        id: "kind",
        labelKey: "office.agents.docs_col_kind",
        accessorFn: (d) => t(kindKey[d.kind]),
      }),
      createColumn<AgentDocument>({
        id: "period",
        labelKey: "office.agents.docs_col_period",
        accessorFn: (d) => d.period ?? "—",
        meta: { cellClassName: "font-mono text-xs" },
      }),
      createColumn<AgentDocument>({
        id: "taxpayer",
        labelKey: "office.agents.docs_col_taxpayer",
        accessorFn: (d) => d.taxpayer_title || "—",
      }),
      createColumn<AgentDocument>({
        id: "status",
        labelKey: "office.agents.docs_col_status",
        accessorKey: "status",
        cell: ({ row }) => (
          <div className="space-y-1">
            <StatusChip
              tone={row.original.status === "failed" ? "danger" : "warning"}
              label={t(
                row.original.status === "failed"
                  ? "office.agents.docs_status_failed"
                  : "office.agents.docs_status_needs_match",
              )}
            />
            {row.original.reason ? (
              <p className="text-muted-foreground text-xs">
                {row.original.reason}
              </p>
            ) : null}
          </div>
        ),
      }),
      createColumn<AgentDocument>({
        id: "created_at",
        labelKey: "office.agents.docs_col_received",
        accessorKey: "created_at",
        gridSecondary: true,
        cell: ({ row }) => (
          <time
            dateTime={row.original.created_at}
            title={datetime(row.original.created_at, undefined, locale)}
          >
            {relativeDatetime(row.original.created_at, locale)}
          </time>
        ),
      }),
      ...(canWrite
        ? [
            createActionsColumn<AgentDocument>({
              labelKey: "office.agents.col_actions",
              cell: (d) => <DocumentActions doc={d} onMatch={onMatch} />,
            }),
          ]
        : []),
    ],
    [canWrite, locale, onMatch, t],
  );
}

function DocumentActions({
  doc,
  onMatch,
}: {
  doc: AgentDocument;
  onMatch: (d: AgentDocument) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const dismiss = useDismissAgentDocument(slug);
  return (
    <div className="flex gap-1">
      <Button size="sm" onClick={() => onMatch(doc)}>
        {t("office.agents.docs_match")}
      </Button>
      <Button
        variant="ghost"
        size="sm"
        disabled={dismiss.isPending}
        onClick={() =>
          dismiss.mutate(doc.uuid, {
            onSuccess: () => toast.success(t("office.agents.docs_dismissed")),
          })
        }
      >
        {t("office.agents.docs_dismiss")}
      </Button>
    </div>
  );
}

/** "Eşleştir": mükellef, dosya türü ve (bordro / mizan için) dönem. */
function MatchDialog({
  doc,
  onClose,
}: {
  doc: AgentDocument;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { options } = useEngagementOptions(slug);
  const resolve = useResolveAgentDocument(slug);
  const [engagement, setEngagement] = useState(doc.engagement_uuid ?? "");
  const [kind, setKind] = useState<AgentDocumentKind>(
    doc.kind === "unknown" ? "payroll" : doc.kind,
  );
  const [period, setPeriod] = useState(doc.period ?? "");
  const needsPeriod = kind === "payroll" || kind === "trial_balance";
  const selected = options.find((o) => o.engagement === engagement) ?? null;
  const ready = !!engagement && (!needsPeriod || !!period);

  const submit = () => {
    if (!ready) return;
    resolve.mutate(
      {
        uuid: doc.uuid,
        engagement,
        kind,
        period: needsPeriod ? period : undefined,
      },
      {
        onSuccess: (res) => {
          if (res.status === "imported") {
            toast.success(t("office.agents.docs_imported"));
            onClose();
          } else {
            toast.error(res.reason || t("office.agents.docs_failed"));
          }
        },
        onError: () => toast.error(t("office.agents.docs_failed")),
      },
    );
  };

  return (
    <Dialog
      open
      onOpenChange={(open) => !open && !resolve.isPending && onClose()}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("office.agents.docs_match_title")}</DialogTitle>
          <DialogDescription>
            {t("office.agents.docs_match_hint", { name: doc.file_name })}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            submit();
          }}
        >
          <div className="space-y-1">
            <Label>{t("office.agents.docs_col_taxpayer")}</Label>
            <EngagementPicker
              options={options}
              selected={selected}
              onSelect={setEngagement}
            />
          </div>
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1">
              <Label htmlFor="ad-kind">
                {t("office.agents.docs_col_kind")}
              </Label>
              <NativeSelect
                id="ad-kind"
                value={kind}
                onChange={(e) => setKind(e.target.value as AgentDocumentKind)}
              >
                {KINDS.map((k) => (
                  <NativeSelectOption key={k} value={k}>
                    {t(kindKey[k])}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            {needsPeriod ? (
              <div className="space-y-1">
                <Label htmlFor="ad-period">
                  {t("office.agents.docs_col_period")}
                </Label>
                <MonthPicker
                  id="ad-period"
                  value={period}
                  onChange={setPeriod}
                />
              </div>
            ) : null}
          </div>
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={onClose}
              disabled={resolve.isPending}
            >
              {t("office.agents.docs_cancel")}
            </Button>
            <Button type="submit" disabled={!ready || resolve.isPending}>
              {t("office.agents.docs_import")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
