"use client";

import { useCallback, useMemo, useState, type ReactNode } from "react";

import { ErrorState } from "@/components/common/error-state";
import {
  EditableGrid,
  type EditableGridColumn,
  type GridRowSaveInput,
} from "@/components/tables";
import { useLocale } from "@/providers/locale-provider";

import { useEngagementScope } from "../engagement-scope";
import { useEngagementDirectory } from "../hooks/use-engagement-directory";
import {
  ACCOUNT_CODE_RE,
  accountValue,
  confidenceOf,
  correctedAccountValue,
  suggestionContext,
  useAccountPlanAccounts,
  useDecisionErrorText,
  useReviewDecision,
  useReviewSuggestions,
} from "../hooks/use-review";
import { gridRowAction } from "../lib/review-grid-action";
import { lineTypeLabel } from "../lib/suggestion-label";
import { useOfficePeriod } from "../period";
import {
  REVIEW_PAGE_MAX,
  type ReviewSuggestion,
} from "../services/review.service";
import { FilterSelect } from "./office-ui";

const ACCOUNT_KIND = "account_code";

/**
 * Öneri düzeltme ızgarası: bekleyen hesap kodu önerileri tabloda; hesap
 * kodu yazılır ya da Excel'den yapıştırılır, satır kaydedilince öneri
 * düzeltilir, onaylanır ya da reddedilir.
 */
export function ReviewGrid({ viewSwitch }: { viewSwitch: ReactNode }) {
  const { t } = useLocale();
  const directory = useEngagementDirectory();
  const { period } = useOfficePeriod();
  // Mükellef detayının İnceleme sekmesinde mükellef sabit (#468).
  const scope = useEngagementScope();
  const [engagement, setEngagement] = useState(() => scope ?? "all");
  const query = useReviewSuggestions({
    status: "pending",
    engagement: engagement === "all" ? undefined : engagement,
    kind: ACCOUNT_KIND,
    period,
    limit: REVIEW_PAGE_MAX,
    offset: 0,
  });
  const plan = useAccountPlanAccounts(
    engagement === "all" ? undefined : engagement,
    "",
  );
  const { mutateAsync } = useReviewDecision();
  const errorText = useDecisionErrorText();

  // En düşük güven önce; kuyrukla aynı sıra.
  const rows = useMemo(
    () =>
      [...(query.data?.items ?? [])]
        .filter((s) => s.kind === ACCOUNT_KIND)
        .sort((a, b) => confidenceOf(a) - confidenceOf(b)),
    [query.data],
  );

  const planAccounts = useMemo(() => plan.data?.items ?? [], [plan.data]);
  const planNames = useMemo(
    () => new Map(planAccounts.map((a) => [a.code, a.name])),
    [planAccounts],
  );

  const columns = useMemo<EditableGridColumn<ReviewSuggestion>[]>(
    () => [
      {
        id: "taxpayer",
        header: t("office.review_grid.col_taxpayer"),
        value: (s) =>
          (s.engagement_uuid &&
            directory.entries.get(s.engagement_uuid)?.title) ||
          "—",
        width: 180,
      },
      {
        id: "counterparty",
        header: t("office.review_grid.col_counterparty"),
        value: (s) => suggestionContext(s).counterparty ?? "—",
        width: 200,
      },
      {
        id: "item",
        header: t("office.review_grid.col_item"),
        value: (s) => {
          const ctx = suggestionContext(s);
          const type = ctx.lineType ? lineTypeLabel(ctx.lineType, t) : "";
          return [ctx.itemName, type].filter(Boolean).join(" · ") || "—";
        },
      },
      {
        id: "proposed",
        header: t("office.review_grid.col_proposed"),
        value: (s) => {
          const v = accountValue(s.proposed_value);
          return [v.code, v.name].filter(Boolean).join(" ") || "—";
        },
      },
      {
        id: "confidence",
        header: t("office.review_grid.col_confidence"),
        value: (s) => `%${Math.round(confidenceOf(s) * 100)}`,
        align: "end",
        width: 72,
      },
      {
        id: "code",
        header: t("office.review_grid.col_code"),
        value: (s) => accountValue(s.proposed_value).code ?? null,
        editor: "text",
        required: true,
        width: 150,
        suggestions: planAccounts.filter((a) => a.is_leaf).map((a) => a.code),
        validate: (v) =>
          ACCOUNT_CODE_RE.test(String(v))
            ? null
            : t("office.review_api.invalid_code"),
        render: (v) => {
          const code = typeof v === "string" ? v : "";
          const name = planNames.get(code);
          return (
            <span className="font-mono">
              {code}
              {name ? (
                <span className="text-muted-foreground ms-1.5 font-sans text-xs">
                  {name}
                </span>
              ) : null}
            </span>
          );
        },
      },
      {
        id: "decision",
        header: t("office.review_grid.col_decision"),
        value: () => null,
        editor: "select",
        width: 120,
        options: [
          { value: "accept", label: t("office.review_grid.decision_accept") },
          { value: "reject", label: t("office.review_grid.decision_reject") },
        ],
        placeholder: t("office.review_grid.decision_auto"),
      },
    ],
    [t, directory.entries, planAccounts, planNames],
  );

  const onSaveRow = useCallback(
    async ({ row, value }: GridRowSaveInput<ReviewSuggestion>) => {
      const proposedCode = accountValue(row.proposed_value).code;
      const code = value("code");
      const action = gridRowAction({
        proposedCode,
        code,
        decision: value("decision"),
      });
      if (!action) return;
      try {
        if (action === "reject") {
          await mutateAsync({
            suggestion: row,
            action: "reject",
            reason: t("office.review_api.reason_reject"),
          });
        } else if (action === "correct") {
          const c = String(code).trim();
          await mutateAsync({
            suggestion: row,
            action: "correct",
            value: correctedAccountValue(
              row.proposed_value,
              c,
              planNames.get(c),
            ),
            reason: t("office.review_api.reason_correct"),
          });
        } else {
          await mutateAsync({ suggestion: row, action: "accept" });
        }
      } catch (err) {
        throw new Error(errorText(err));
      }
    },
    [mutateAsync, planNames, errorText, t],
  );

  return (
    <div className="flex min-h-0 flex-col gap-4">
      <header className="flex flex-wrap items-center gap-3">
        <ReviewHeading scoped={!!scope} />
        <span className="bg-surface-2 rounded-md border px-2.5 py-0.5 font-mono text-xs font-semibold tabular-nums">
          {t("office.review_pending", { count: query.data?.total ?? 0 })}
        </span>
        {viewSwitch}
      </header>
      <p className="text-muted-foreground max-w-3xl text-sm">
        {t("office.review_grid.hint")}
      </p>
      {query.isError ? (
        <ErrorState
          title={t("office.review_api.load_error")}
          description={query.error.message}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <EditableGrid
          label={t("office.review_grid.label")}
          columns={columns}
          rows={rows}
          getRowId={getSuggestionId}
          isLoading={query.isLoading}
          emptyTitle={t("office.review_empty")}
          emptyDescription={t("office.review_empty_description")}
          onSaveRow={onSaveRow}
          maxHeight="calc(100svh - 18rem)"
          toolbar={
            <>
              {scope ? null : (
                <FilterSelect
                  label={t("office.col_taxpayer")}
                  value={engagement}
                  onChange={setEngagement}
                  options={[
                    { value: "all", label: t("office.filter_all") },
                    ...directory.list.map((x) => ({
                      value: x.engagementUuid,
                      label: x.title,
                    })),
                  ]}
                />
              )}
              {engagement === "all" ? (
                <span className="text-muted-foreground text-xs">
                  {t("office.review_grid.plan_hint")}
                </span>
              ) : null}
            </>
          }
        />
      )}
    </div>
  );
}

function getSuggestionId(s: ReviewSuggestion) {
  return s.uuid;
}

/** Sayfa başlığı: mükellef detayında (#468) h2, genel sayfada h1. */
export function ReviewHeading({ scoped }: { scoped: boolean }) {
  const { t } = useLocale();
  const Heading = scoped ? "h2" : "h1";
  return (
    <Heading className="font-display text-2xl font-semibold tracking-tight">
      {t("office.review_title")}
    </Heading>
  );
}
