"use client";

import { Check, ClipboardCheck, Pencil, Sparkles, X } from "lucide-react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";
import { toast } from "sonner";

import { EmptyState } from "@/components/common/empty-state";
import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useEngagementDirectory } from "../hooks/use-engagement-directory";
import {
  ACCOUNT_CODE_RE,
  accountValue,
  confidenceOf,
  correctedAccountValue,
  suggestionContext,
  useDecisionErrorText,
  useReviewDecision,
  useReviewSuggestions,
  useReviewThreshold,
  valueLabel,
} from "../hooks/use-review";
import type { ReviewSuggestion } from "../services/review.service";
import { FilterSelect, Figure, Panel } from "./office-ui";

const PAGE_SIZE = 50;
const ACCOUNT_KIND = "account_code";

function Confidence({
  value,
  threshold,
}: {
  value: number;
  threshold: number;
}) {
  const pct = Math.round(value * 100);
  const tone =
    value >= threshold
      ? "bg-status-approved"
      : value >= 0.6
        ? "bg-status-review"
        : "bg-status-warning";
  return (
    <div className="flex items-center gap-2" title={`${pct}%`}>
      <div className="bg-muted h-1.5 w-16 overflow-hidden rounded-full">
        <div
          className={cn("h-full rounded-full", tone)}
          style={{ width: `${pct}%` }}
        />
      </div>
      <Figure className="text-xs">%{pct}</Figure>
    </div>
  );
}

function ReviewRow({
  s,
  taxpayer,
  threshold,
}: {
  s: ReviewSuggestion;
  taxpayer?: string;
  threshold: number;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const decision = useReviewDecision();
  const errorText = useDecisionErrorText();
  const proposed = accountValue(s.proposed_value);
  const ctx = suggestionContext(s);
  const isAccount = s.kind === ACCOUNT_KIND && !!proposed.code;
  const [editing, setEditing] = useState(false);
  const [code, setCode] = useState(proposed.code ?? "");
  const busy = decision.isPending;

  const onError = (err: unknown) => toast.error(errorText(err));

  const approve = () => {
    const next = code.trim();
    const changed = isAccount && editing && next && next !== proposed.code;
    if (changed) {
      if (!ACCOUNT_CODE_RE.test(next)) {
        toast.error(t("office.review_api.invalid_code"));
        return;
      }
      decision.mutate(
        {
          suggestion: s,
          action: "correct",
          value: correctedAccountValue(s.proposed_value, next),
          reason: t("office.review_api.reason_correct"),
        },
        {
          onSuccess: () =>
            toast.success(t("office.review_approved_toast", { code: next })),
          onError,
        },
      );
      return;
    }
    decision.mutate(
      { suggestion: s, action: "accept" },
      {
        onSuccess: () =>
          toast.success(
            t("office.review_approved_toast", {
              code: valueLabel(s.proposed_value),
            }),
          ),
        onError,
      },
    );
  };

  const reject = () =>
    decision.mutate(
      {
        suggestion: s,
        action: "reject",
        reason: t("office.review_api.reason_reject"),
      },
      {
        onSuccess: () => toast.success(t("office.review_api.rejected_toast")),
        onError,
      },
    );

  const title =
    ctx.itemName ??
    (ctx.counterparty
      ? t("office.review_api.counterparty_account", {
          name: ctx.counterparty,
        })
      : `${s.target_type}.${s.target_field}`);

  return (
    <li
      className={cn(
        "grid gap-3 px-4 py-3 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_auto] lg:items-center",
        busy && "opacity-60",
      )}
    >
      <div className="min-w-0">
        <div className="truncate font-medium">{title}</div>
        <div className="text-muted-foreground flex flex-wrap items-center gap-x-2 text-xs">
          {taxpayer ? <span className="truncate">{taxpayer}</span> : null}
          {ctx.counterparty && ctx.itemName ? (
            <>
              <span>·</span>
              <span className="truncate">{ctx.counterparty}</span>
            </>
          ) : null}
          {ctx.direction === "incoming" || ctx.direction === "outgoing" ? (
            <>
              <span>·</span>
              <span>{t(`office.review_api.direction_${ctx.direction}`)}</span>
            </>
          ) : null}
          {s.period ? (
            <>
              <span>·</span>
              <Figure>{s.period}</Figure>
            </>
          ) : null}
          {ctx.invoiceUuid ? (
            <>
              <span>·</span>
              <Link
                className="hover:underline"
                href={routes.tenant.invoice(
                  slug,
                  ctx.invoiceUuid,
                  s.engagement_uuid ?? undefined,
                )}
              >
                {ctx.lineNo
                  ? t("office.review_api.invoice_line", { line: ctx.lineNo })
                  : t("office.review_api.invoice_link")}
              </Link>
            </>
          ) : null}
        </div>
      </div>
      <div className="min-w-0">
        {editing ? (
          <Input
            autoFocus
            value={code}
            onChange={(e) => setCode(e.target.value)}
            className="figure h-8 w-40"
            aria-label={t("office.col_account")}
            onKeyDown={(e) => {
              if (e.key === "Enter") approve();
              if (e.key === "Escape") setEditing(false);
            }}
          />
        ) : (
          <div className="flex items-baseline gap-2">
            <Figure className="font-semibold">
              {valueLabel(s.proposed_value)}
            </Figure>
            {proposed.name ? (
              <span className="text-muted-foreground truncate text-xs">
                {proposed.name}
              </span>
            ) : null}
          </div>
        )}
        <div className="mt-1 flex flex-wrap items-center gap-2">
          <Confidence value={confidenceOf(s)} threshold={threshold} />
          <span className="text-muted-foreground text-xs">
            {t(`office.review_api.origin_${s.provenance.origin}`)}
            {s.rationale ? ` · ${s.rationale}` : ""}
          </span>
        </div>
      </div>
      <div className="flex gap-1">
        <Button size="sm" onClick={approve} disabled={busy}>
          <Check className="size-4" />
          {t("office.approve")}
        </Button>
        {isAccount && !editing ? (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setEditing(true)}
            disabled={busy}
            aria-label={t("office.change_code")}
          >
            <Pencil className="size-4" />
          </Button>
        ) : null}
        <Button
          size="sm"
          variant="outline"
          onClick={reject}
          disabled={busy}
          aria-label={t("office.reject")}
        >
          <X className="size-4" />
        </Button>
      </div>
    </li>
  );
}

function ReviewInner() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const params = useSearchParams();
  const directory = useEngagementDirectory();
  const threshold = useReviewThreshold();
  const [engagement, setEngagement] = useState("all");
  const [kind, setKind] = useState("all");
  const [offset, setOffset] = useState(0);
  const invoiceFilter = params.get("invoice");

  const filter = {
    engagement: engagement === "all" ? undefined : engagement,
    kind: kind === "all" ? undefined : kind,
  };
  const pendingQuery = useReviewSuggestions({
    ...filter,
    status: "pending",
    limit: PAGE_SIZE,
    offset,
  });
  // Total over every status minus pending = decided (incl. auto-accepted).
  const allQuery = useReviewSuggestions({
    ...filter,
    status: "all",
    limit: 1,
    offset: 0,
  });

  const page = pendingQuery.data;
  const pending = (page?.items ?? []).filter(
    (s) => !invoiceFilter || suggestionContext(s).invoiceUuid === invoiceFilter,
  );
  const total = page?.total ?? 0;
  const decided =
    allQuery.data && page ? Math.max(allQuery.data.total - total, 0) : null;

  const kinds = Array.from(
    new Set([ACCOUNT_KIND, ...(page?.items ?? []).map((s) => s.kind)]),
  );

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.review_title")}
        description={t("office.review_description", {
          threshold: Math.round(threshold * 100),
        })}
        icon={<ClipboardCheck className="size-6" />}
        actions={
          <Button asChild>
            <Link href={routes.tenant.reviewStudio(slug)}>
              <Sparkles className="size-4" />
              {t("office.studio.open")}
            </Link>
          </Button>
        }
      />
      <div className="flex flex-wrap items-center gap-3">
        <FilterSelect
          label={t("office.col_taxpayer")}
          value={engagement}
          onChange={(v) => {
            setEngagement(v);
            setOffset(0);
          }}
          options={[
            { value: "all", label: t("office.filter_all") },
            ...directory.list.map((x) => ({
              value: x.engagementUuid,
              label: x.title,
            })),
          ]}
        />
        <FilterSelect
          label={t("office.review_api.kind")}
          value={kind}
          onChange={(v) => {
            setKind(v);
            setOffset(0);
          }}
          options={[
            { value: "all", label: t("office.filter_all") },
            ...kinds.map((k) => ({
              value: k,
              label:
                k === ACCOUNT_KIND ? t("office.review_api.kind_account") : k,
            })),
          ]}
        />
        {invoiceFilter ? (
          <Button asChild size="sm" variant="ghost">
            <Link href="?">{t("office.clear_invoice_filter")}</Link>
          </Button>
        ) : null}
      </div>
      <Panel
        title={t("office.review_pending", { count: total })}
        description={t("office.review_sorted")}
      >
        {pendingQuery.isLoading ? (
          <Loading className="m-4" label={t("common.loading")} />
        ) : pendingQuery.isError ? (
          <ErrorState
            className="m-4"
            title={t("office.review_api.load_error")}
            description={pendingQuery.error.message}
            onRetry={() => void pendingQuery.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : pending.length === 0 ? (
          <EmptyState
            className="m-4"
            title={t("office.review_empty")}
            description={t("office.review_empty_description")}
          />
        ) : (
          <ul className="divide-y">
            {pending.map((s) => (
              <ReviewRow
                key={s.uuid}
                s={s}
                threshold={threshold}
                taxpayer={
                  s.engagement_uuid
                    ? directory.entries.get(s.engagement_uuid)?.title
                    : directory.entries.get(engagement)?.title
                }
              />
            ))}
          </ul>
        )}
        {total > PAGE_SIZE ? (
          <div className="flex items-center justify-end gap-2 border-t px-4 py-2 text-xs">
            <span className="text-muted-foreground">
              {t("office.review_api.page_range", {
                from: offset + 1,
                to: Math.min(offset + PAGE_SIZE, total),
                total,
              })}
            </span>
            <Button
              size="sm"
              variant="outline"
              disabled={offset === 0}
              onClick={() => setOffset((o) => Math.max(o - PAGE_SIZE, 0))}
            >
              {t("office.review_api.prev")}
            </Button>
            <Button
              size="sm"
              variant="outline"
              disabled={offset + PAGE_SIZE >= total}
              onClick={() => setOffset((o) => o + PAGE_SIZE)}
            >
              {t("office.review_api.next")}
            </Button>
          </div>
        ) : null}
      </Panel>
      {decided !== null ? (
        <p className="text-muted-foreground text-xs">
          {t("office.review_decided_count", { count: decided })}
        </p>
      ) : null}
    </div>
  );
}

export function ReviewQueuePage() {
  return (
    <Suspense>
      <ReviewInner />
    </Suspense>
  );
}
