"use client";

import {
  BrainCircuit,
  Download,
  FileUp,
  Loader2,
  Undo2,
  Wand2,
} from "lucide-react";
import { Suspense, useMemo, useRef, useState } from "react";
import type { ColumnDef } from "@tanstack/react-table";

import { Input } from "@/components/ui/input";
import { EmptyState } from "@/components/common/empty-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
import {
  createActionsColumn,
  createColumn,
  DataTable,
  type DataTableFeatures,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { useDialogs } from "@/providers/dialog-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  useAccountMemoryActions,
  useAccountPlanImports,
  useEngagementRules,
  useMissingPlanAccounts,
  useRuleCandidates,
} from "../hooks/use-account-memory";
import {
  accountValue,
  suggestionContext,
  useReviewDecision,
  useReviewSuggestions,
} from "../hooks/use-review";
import type {
  AccountPlanImportResult,
  AccountRule,
  AccountRuleCandidate,
  MissingPlanAccount,
} from "../services/account-memory.service";
import type { ReviewSuggestion } from "../services/review.service";
import { EngagementPicker, useSelectedEngagement } from "./engagement-picker";
import { LucaPlanPanel } from "./luca-plan-panel";
import { Panel } from "./office-ui";
import { RuleEditor } from "./rule-editor";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

const directionKey = (d?: string) =>
  d === "incoming"
    ? "office.memory.direction_incoming"
    : d === "outgoing"
      ? "office.memory.direction_outgoing"
      : "office.memory.direction_unknown";

function errorText(t: Translate, err: unknown) {
  if (isApiError(err)) {
    const key = `office.memory.error_${err.code}`;
    const msg = err.code ? t(key) : key;
    return msg !== key ? msg : err.message || t("office.api.load_error");
  }
  return t("office.api.load_error");
}

// Panel içine gömülü istemci tarafı listeler: araç çubuğu ve sayfalama yok
// (tüm kayıtlar görünür), sıralama açık; mobilde kartlar.
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

/** Hesap planı (Luca mizanı) içe aktarma ve etkin planın özeti. */
function PlanPanel({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const imports = useAccountPlanImports(slug, engagement);
  const { importPlan } = useAccountMemoryActions(slug, engagement);
  const fileRef = useRef<HTMLInputElement>(null);
  const [file, setFile] = useState<File | null>(null);
  const [result, setResult] = useState<AccountPlanImportResult | null>(null);
  const latest = imports.data?.items?.[0];
  const canWrite = can(permissions.tenant.accountPlan.write);

  const submit = () => {
    if (!file) return;
    importPlan.mutate(file, {
      onSuccess: (r) => {
        setResult(r);
        setFile(null);
        if (fileRef.current) fileRef.current.value = "";
      },
    });
  };

  return (
    <Panel
      title={t("office.memory.plan_title")}
      description={t("office.memory.plan_description")}
    >
      <div className="space-y-3 p-4 text-sm">
        {imports.isLoading ? (
          <Skeleton className="h-6 w-80" />
        ) : latest ? (
          <p data-testid="plan-summary">
            {t("office.memory.plan_active", {
              count: latest.account_count,
              file: latest.file_name,
              date: datetime(latest.created_at, undefined, locale),
            })}
          </p>
        ) : (
          <p className="text-muted-foreground">
            {t("office.memory.plan_missing")}
          </p>
        )}
        {canWrite ? (
          <div className="flex flex-wrap items-center gap-2">
            <Input
              ref={fileRef}
              type="file"
              accept=".xlsx,.csv"
              aria-label={t("office.memory.plan_file")}
              className="file:bg-secondary file:text-secondary-foreground h-auto w-auto max-w-full min-w-0 border-0 p-0 text-xs shadow-none file:me-2 file:h-auto file:rounded-md file:border-0 file:px-3 file:py-1.5 max-md:min-h-11"
              onChange={(e) => setFile(e.target.files?.[0] ?? null)}
            />
            <Button
              size="sm"
              onClick={submit}
              disabled={!file || importPlan.isPending}
            >
              {importPlan.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <FileUp className="size-4" />
              )}
              {t("office.memory.plan_import")}
            </Button>
          </div>
        ) : null}
        {importPlan.error ? (
          <p className="text-status-blocking">
            {errorText(t, importPlan.error)}
          </p>
        ) : null}
        {result ? (
          <p role="status" className="text-status-approved">
            {t("office.memory.plan_imported", {
              count: result.import.account_count,
              skipped: result.skipped.length,
            })}
          </p>
        ) : null}
      </div>
    </Panel>
  );
}

/**
 * Luca'da açılacak hesaplar (#319): uygulanmış ama planda olmayan kodlar
 * (çoğu "açılmalı" cari). Luca çıktısı bunlar açılıp mizan yeniden
 * yüklenene kadar durur; şablon Luca'ya aktarılacak Excel'dir.
 */
function MissingPanel({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const missing = useMissingPlanAccounts(slug, engagement);
  // Plan hiç yüklenmemişse "eksik yok" denmez (#572): kontrol yapılamadı.
  const imports = useAccountPlanImports(slug, engagement);
  const planMissing =
    imports.isSuccess && (imports.data?.items ?? []).length === 0;
  const { downloadTemplate } = useAccountMemoryActions(slug, engagement);
  const items = missing.data?.items ?? [];
  const missingColumns = useMemo<ColumnDef<MissingPlanAccount, unknown>[]>(
    () => [
      createColumn<MissingPlanAccount>({
        id: "code",
        labelKey: "office.memory.col_code",
        accessorKey: "code",
        gridPrimary: true,
        meta: { cellClassName: "font-mono" },
      }),
      createColumn<MissingPlanAccount>({
        id: "name",
        labelKey: "office.memory.col_account_name",
        accessorFn: (a) => a.name || "—",
      }),
      createColumn<MissingPlanAccount>({
        id: "uses",
        labelKey: "office.memory.col_uses",
        accessorKey: "uses",
        meta: { cellClassName: "tabular-nums" },
      }),
    ],
    [],
  );
  return (
    <Panel
      title={t("office.memory.missing_title")}
      description={t("office.memory.missing_description")}
    >
      {missing.isLoading || imports.isLoading ? (
        <Skeleton className="m-4 h-16" />
      ) : items.length === 0 ? (
        <p
          className="text-muted-foreground p-4 text-sm"
          data-testid="missing-accounts-empty"
        >
          {planMissing
            ? t("office.memory.missing_no_plan")
            : t("office.memory.missing_empty")}
        </p>
      ) : (
        <div className="space-y-3 p-4">
          <DataTable<MissingPlanAccount>
            columns={missingColumns}
            data={items}
            getRowId={(a) => a.code}
            features={panelTable}
          />
          <Button
            size="sm"
            disabled={downloadTemplate.isPending}
            onClick={() => downloadTemplate.mutate(items)}
          >
            {downloadTemplate.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Download className="size-4" />
            )}
            {t("office.memory.missing_download")}
          </Button>
          {downloadTemplate.error ? (
            <p className="text-status-blocking text-sm">
              {errorText(t, downloadTemplate.error)}
            </p>
          ) : null}
        </div>
      )}
    </Panel>
  );
}

const candidateId = (c: AccountRuleCandidate) =>
  `${c.counterparty_key}|${c.direction}|${c.account_code}`;

/** Adayı kurala terfi ettiren satır düğmesi (yazma yetkisi varsa). */
function PromoteButton({
  c,
  engagement,
}: {
  c: AccountRuleCandidate;
  engagement: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const { promote } = useAccountMemoryActions(slug, engagement);
  if (!can(permissions.tenant.accountRules.write)) return null;
  return (
    <Button
      size="sm"
      disabled={promote.isPending}
      onClick={() =>
        promote.mutate({
          counterparty_key: c.counterparty_key ?? "",
          counterparty_name: c.counterparty_name,
          direction:
            c.direction === "incoming" || c.direction === "outgoing"
              ? c.direction
              : undefined,
          account_code: c.account_code ?? "",
        })
      }
    >
      <Wand2 className="size-4" />
      {t("office.memory.promote")}
    </Button>
  );
}

/** Kural adayları: aynı firmanın kalemleri aynı koda en az 3 kez karar aldı. */
function CandidatesPanel({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const candidates = useRuleCandidates(slug, engagement);
  const items = candidates.data?.items ?? [];
  const candidateColumns = useMemo<ColumnDef<AccountRuleCandidate, unknown>[]>(
    () => [
      createColumn<AccountRuleCandidate>({
        id: "counterparty",
        labelKey: "office.memory.col_counterparty",
        accessorFn: (c) => c.counterparty_name || "—",
        gridPrimary: true,
        cell: ({ row }) => {
          const c = row.original;
          return (
            <>
              <div className="font-medium">{c.counterparty_name || "—"}</div>
              <div className="text-muted-foreground text-xs">
                {c.counterparty_key?.startsWith("name:")
                  ? ""
                  : c.counterparty_key}
              </div>
            </>
          );
        },
      }),
      createColumn<AccountRuleCandidate>({
        id: "direction",
        labelKey: "office.memory.col_direction",
        accessorFn: (c) => t(directionKey(c.direction)),
      }),
      createColumn<AccountRuleCandidate>({
        id: "account_code",
        labelKey: "office.memory.col_code",
        accessorFn: (c) => c.account_code ?? "",
        meta: { cellClassName: "font-mono" },
      }),
      createColumn<AccountRuleCandidate>({
        id: "decisions",
        labelKey: "office.memory.col_decisions",
        accessorKey: "decisions",
        meta: { cellClassName: "tabular-nums" },
      }),
      createActionsColumn<AccountRuleCandidate>({
        labelKey: "office.memory.col_action",
        cell: (c) => <PromoteButton c={c} engagement={engagement} />,
      }),
    ],
    [engagement, t],
  );
  return (
    <Panel
      title={t("office.memory.candidates_title", { count: items.length })}
      description={t("office.memory.candidates_description")}
    >
      {candidates.isLoading ? (
        <Skeleton className="m-4 h-16" />
      ) : items.length === 0 ? (
        <p className="text-muted-foreground p-4 text-sm">
          {t("office.memory.candidates_empty")}
        </p>
      ) : (
        <DataTable<AccountRuleCandidate>
          columns={candidateColumns}
          data={items}
          getRowId={candidateId}
          bordered={false}
          features={panelTable}
        />
      )}
    </Panel>
  );
}

const originKey: Record<string, string> = {
  candidate: "office.memory.origin_candidate",
  manual: "office.memory.origin_manual",
  template: "office.memory.origin_template",
};

function RulesPanel({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const { confirmDelete } = useDialogs();
  const { retireRule } = useAccountMemoryActions(slug, engagement);
  const canWrite = can(permissions.tenant.accountRules.write);
  const rules = useEngagementRules(slug, engagement);
  const items = (rules.data?.items ?? []).filter((r) => r.status === "active");
  const retire = retireRule.mutate;
  const retiring = retireRule.isPending;
  const ruleColumns = useMemo<ColumnDef<AccountRule, unknown>[]>(
    () => [
      createColumn<AccountRule>({
        id: "name",
        labelKey: "office.memory.col_rule",
        accessorKey: "name",
        gridPrimary: true,
        meta: { cellClassName: "font-medium" },
      }),
      createColumn<AccountRule>({
        id: "account_code",
        labelKey: "office.memory.col_code",
        accessorKey: "account_code",
        meta: { cellClassName: "font-mono" },
      }),
      createColumn<AccountRule>({
        id: "origin",
        labelKey: "office.memory.col_origin",
        accessorFn: (r) =>
          t(originKey[r.origin] ?? "office.memory.origin_manual"),
      }),
      // Biçim aynen korunur (dd.MM.yyyy HH:mm); ISO değer sıralanır.
      createColumn<AccountRule>({
        id: "created_at",
        labelKey: "office.memory.col_created",
        accessorKey: "created_at",
        cell: ({ row }) => datetime(row.original.created_at, undefined, locale),
      }),
      ...(canWrite
        ? [
            createActionsColumn<AccountRule>({
              labelKey: "office.memory.col_action",
              cell: (r) =>
                r.scope === "engagement" ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    disabled={retiring}
                    onClick={async () => {
                      const ok = await confirmDelete({
                        title: t("office.memory.retire_title", {
                          name: r.name,
                        }),
                        description: t("office.memory.retire_description"),
                        confirmLabel: t("office.memory.retire"),
                      });
                      if (ok) retire(r.rule_key);
                    }}
                  >
                    {t("office.memory.retire")}
                  </Button>
                ) : null,
            }),
          ]
        : []),
    ],
    [canWrite, confirmDelete, locale, retire, retiring, t],
  );
  return (
    <Panel
      title={t("office.memory.rules_title", { count: items.length })}
      description={t("office.memory.rules_description")}
      actions={canWrite ? <RuleEditor engagement={engagement} /> : undefined}
    >
      {rules.isLoading ? (
        <Skeleton className="m-4 h-16" />
      ) : items.length === 0 ? (
        <p className="text-muted-foreground p-4 text-sm">
          {t("office.memory.rules_empty")}
        </p>
      ) : (
        <DataTable<AccountRule>
          columns={ruleColumns}
          data={items}
          getRowId={(r) => r.uuid}
          bordered={false}
          features={panelTable}
        />
      )}
    </Panel>
  );
}

/** Soru sorulmadan (eşik üstü) kodlanan son kalemler. */
function AutoCodedPanel({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const decision = useReviewDecision();
  const canRevert = can(permissions.tenant.review.decide);
  const auto = useReviewSuggestions({
    engagement,
    status: "auto_accepted",
    kind: "account_code",
    limit: 50,
    offset: 0,
  });
  const items = (auto.data?.items ?? []).filter(
    (s) =>
      s.target_type === "invoice_item" && s.target_field === "account_code",
  );
  const decide = decision.mutate;
  const deciding = decision.isPending;
  const autoColumns = useMemo<ColumnDef<ReviewSuggestion, unknown>[]>(
    () => [
      createColumn<ReviewSuggestion>({
        id: "item",
        labelKey: "office.memory.col_item",
        accessorFn: (s) => suggestionContext(s).itemName ?? "—",
        gridPrimary: true,
      }),
      createColumn<ReviewSuggestion>({
        id: "counterparty",
        labelKey: "office.memory.col_counterparty",
        accessorFn: (s) => suggestionContext(s).counterparty ?? "—",
      }),
      createColumn<ReviewSuggestion>({
        id: "code",
        labelKey: "office.memory.col_code",
        accessorFn: (s) =>
          accountValue(s.applied_value ?? s.proposed_value).code,
        meta: { cellClassName: "font-mono" },
      }),
      createColumn<ReviewSuggestion>({
        id: "source",
        labelKey: "office.memory.col_source",
        accessorFn: (s) =>
          s.provenance.rule_ref?.startsWith("account_rule:")
            ? t("office.memory.source_rule")
            : t("office.memory.source_auto"),
        cell: ({ getValue }) => (
          <StatusChip tone="success" label={String(getValue())} />
        ),
      }),
      ...(canRevert
        ? [
            createActionsColumn<ReviewSuggestion>({
              labelKey: "office.memory.col_action",
              cell: (s) => (
                <Button
                  size="sm"
                  variant="ghost"
                  disabled={deciding}
                  onClick={() =>
                    decide({
                      suggestion: s,
                      action: "revert",
                      reason: t("office.memory.revert_reason"),
                    })
                  }
                >
                  <Undo2 className="size-4" />
                  {t("office.memory.revert")}
                </Button>
              ),
            }),
          ]
        : []),
    ],
    [canRevert, decide, deciding, t],
  );
  return (
    <Panel
      title={t("office.memory.auto_title", { count: items.length })}
      description={t("office.memory.auto_description")}
    >
      {auto.isLoading ? (
        <Skeleton className="m-4 h-16" />
      ) : items.length === 0 ? (
        <p className="text-muted-foreground p-4 text-sm">
          {t("office.memory.auto_empty")}
        </p>
      ) : (
        <DataTable<ReviewSuggestion>
          columns={autoColumns}
          data={items}
          getRowId={(s) => s.uuid}
          bordered={false}
          features={panelTable}
        />
      )}
    </Panel>
  );
}

function AccountMemoryInner() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { options, selected, select, isLoading, isError } =
    useSelectedEngagement(slug);
  const engagement = selected?.engagement ?? "";
  const header = (
    <PageHeader
      title={t("office.memory.title")}
      description={t("office.memory.page_description")}
      icon={<BrainCircuit className="size-6" />}
    />
  );
  if (isLoading) {
    return (
      <div className="space-y-4">
        {header}
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }
  if (isError || options.length === 0) {
    return (
      <div className="space-y-4">
        {header}
        <EmptyState
          title={
            isError
              ? t("office.api.load_error")
              : t("office.api.no_engagements")
          }
        />
      </div>
    );
  }
  return (
    <div className="space-y-4">
      {header}
      <EngagementPicker
        options={options}
        selected={selected}
        onSelect={select}
      />
      {/* Kimin hafızası (#572): kurallar ve plan yalnız seçili mükellefin. */}
      {selected ? (
        <p
          className="bg-muted/40 text-muted-foreground rounded-lg border px-3 py-2 text-sm"
          data-testid="memory-scope"
        >
          <span className="text-foreground font-medium">{selected.title}</span>
          {" · "}
          {t("office.memory.scope_note")}
        </p>
      ) : null}
      {/* OfisMizan sırası: kurallar, hafıza (adaylar ve otomatik kodlananlar), hesap planı. */}
      <RulesPanel engagement={engagement} />
      <CandidatesPanel engagement={engagement} />
      <AutoCodedPanel engagement={engagement} />
      <PlanPanel engagement={engagement} />
      <MissingPanel engagement={engagement} />
      <LucaPlanPanel engagement={engagement} />
    </div>
  );
}

/** `/t/{slug}/account-plan` (#310): mizan hafızası. */
export function AccountMemoryPage() {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <AccountMemoryInner />
    </Suspense>
  );
}
