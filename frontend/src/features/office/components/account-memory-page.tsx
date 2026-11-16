"use client";

import { BrainCircuit, FileUp, Loader2, Undo2, Wand2 } from "lucide-react";
import { Suspense, useRef, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { StatusChip } from "@/components/common/status-chip";
import { PageHeader } from "@/components/layout/page-header";
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
  AccountRuleCandidate,
} from "../services/account-memory.service";
import { EngagementPicker, useSelectedEngagement } from "./engagement-picker";
import { Panel, TableBox } from "./office-ui";

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
            <label className="flex items-center gap-2">
              <span className="sr-only">{t("office.memory.plan_file")}</span>
              <input
                ref={fileRef}
                type="file"
                accept=".xlsx,.csv"
                aria-label={t("office.memory.plan_file")}
                className="file:bg-secondary file:text-secondary-foreground text-xs file:me-2 file:rounded-md file:border-0 file:px-3 file:py-1.5"
                onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              />
            </label>
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

function CandidateRow({
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
  return (
    <tr>
      <td>
        <div className="font-medium">{c.counterparty_name || "—"}</div>
        <div className="text-muted-foreground text-xs">
          {c.counterparty_key?.startsWith("name:") ? "" : c.counterparty_key}
        </div>
      </td>
      <td>{t(directionKey(c.direction))}</td>
      <td className="font-mono">{c.account_code}</td>
      <td className="tabular-nums">{c.decisions}</td>
      <td className="text-right">
        {can(permissions.tenant.accountRules.write) ? (
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
        ) : null}
      </td>
    </tr>
  );
}

/** Kural adayları: aynı firmanın kalemleri aynı koda en az 3 kez karar aldı. */
function CandidatesPanel({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const candidates = useRuleCandidates(slug, engagement);
  const items = candidates.data?.items ?? [];
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
        <TableBox>
          <thead>
            <tr>
              <th>{t("office.memory.col_counterparty")}</th>
              <th>{t("office.memory.col_direction")}</th>
              <th>{t("office.memory.col_code")}</th>
              <th>{t("office.memory.col_decisions")}</th>
              <th>
                <span className="sr-only">{t("office.memory.col_action")}</span>
              </th>
            </tr>
          </thead>
          <tbody>
            {items.map((c) => (
              <CandidateRow
                key={`${c.counterparty_key}|${c.direction}|${c.account_code}`}
                c={c}
                engagement={engagement}
              />
            ))}
          </tbody>
        </TableBox>
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
  return (
    <Panel
      title={t("office.memory.rules_title", { count: items.length })}
      description={t("office.memory.rules_description")}
    >
      {rules.isLoading ? (
        <Skeleton className="m-4 h-16" />
      ) : items.length === 0 ? (
        <p className="text-muted-foreground p-4 text-sm">
          {t("office.memory.rules_empty")}
        </p>
      ) : (
        <TableBox>
          <thead>
            <tr>
              <th>{t("office.memory.col_rule")}</th>
              <th>{t("office.memory.col_code")}</th>
              <th>{t("office.memory.col_origin")}</th>
              <th>{t("office.memory.col_created")}</th>
              {canWrite ? (
                <th>
                  <span className="sr-only">
                    {t("office.memory.col_action")}
                  </span>
                </th>
              ) : null}
            </tr>
          </thead>
          <tbody>
            {items.map((r) => (
              <tr key={r.uuid}>
                <td className="font-medium">{r.name}</td>
                <td className="font-mono">{r.account_code}</td>
                <td>
                  {t(originKey[r.origin] ?? "office.memory.origin_manual")}
                </td>
                <td>{datetime(r.created_at, undefined, locale)}</td>
                {canWrite ? (
                  <td className="text-right">
                    {r.scope === "engagement" ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={retireRule.isPending}
                        onClick={async () => {
                          const ok = await confirmDelete({
                            title: t("office.memory.retire_title", {
                              name: r.name,
                            }),
                            description: t("office.memory.retire_description"),
                            confirmLabel: t("office.memory.retire"),
                          });
                          if (ok) retireRule.mutate(r.rule_key);
                        }}
                      >
                        {t("office.memory.retire")}
                      </Button>
                    ) : null}
                  </td>
                ) : null}
              </tr>
            ))}
          </tbody>
        </TableBox>
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
        <TableBox>
          <thead>
            <tr>
              <th>{t("office.memory.col_item")}</th>
              <th>{t("office.memory.col_counterparty")}</th>
              <th>{t("office.memory.col_code")}</th>
              <th>{t("office.memory.col_source")}</th>
              {canRevert ? (
                <th>
                  <span className="sr-only">
                    {t("office.memory.col_action")}
                  </span>
                </th>
              ) : null}
            </tr>
          </thead>
          <tbody>
            {items.map((s) => {
              const ctx = suggestionContext(s);
              const byRule = s.provenance.rule_ref?.startsWith("account_rule:");
              return (
                <tr key={s.uuid}>
                  <td>{ctx.itemName ?? "—"}</td>
                  <td>{ctx.counterparty ?? "—"}</td>
                  <td className="font-mono">
                    {accountValue(s.applied_value ?? s.proposed_value).code}
                  </td>
                  <td>
                    <StatusChip
                      tone="success"
                      label={
                        byRule
                          ? t("office.memory.source_rule")
                          : t("office.memory.source_auto")
                      }
                    />
                  </td>
                  {canRevert ? (
                    <td className="text-right">
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={decision.isPending}
                        onClick={() =>
                          decision.mutate({
                            suggestion: s,
                            action: "revert",
                            reason: t("office.memory.revert_reason"),
                          })
                        }
                      >
                        <Undo2 className="size-4" />
                        {t("office.memory.revert")}
                      </Button>
                    </td>
                  ) : null}
                </tr>
              );
            })}
          </tbody>
        </TableBox>
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
      <PlanPanel engagement={engagement} />
      <CandidatesPanel engagement={engagement} />
      <RulesPanel engagement={engagement} />
      <AutoCodedPanel engagement={engagement} />
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
