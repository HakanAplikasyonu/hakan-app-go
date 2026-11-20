"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";
import { useCallback } from "react";

import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { keepPreviousWithin } from "@/lib/query/keep-previous";
import { useLocale } from "@/providers/locale-provider";

import {
  reviewKeys,
  reviewService,
  type ReviewAction,
  type ReviewListFilter,
  type ReviewSuggestion,
} from "../services/review.service";
import { valueText } from "../lib/suggestion-label";

/** Auto-accept threshold the server falls back to (`review.DefaultPolicy`). */
const DEFAULT_THRESHOLD = 0.95;

/**
 * `silent`: hata bildirimi açılmaz; bileşen hatayı kendisi gösterir (ör.
 * sayım satırı "…" yazar, aynı hata için ayrıca bildirim yığılmaz).
 */
export function useReviewSuggestions(
  filter: ReviewListFilter,
  enabled = true,
  silent = false,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reviewKeys.list(slug, filter),
    queryFn: () => reviewService.list(slug, filter),
    placeholderData: keepPreviousData,
    enabled,
    meta: silent ? { silent: true } : undefined,
  });
}

/** Karşı tarafın geçmiş satır kodları (#669 L): sunucu bütün kararlardan sayar. */
export function useCounterpartyCodeHistory(
  engagement: string | undefined,
  counterparty: string | undefined,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reviewKeys.history(slug, engagement ?? "", counterparty ?? ""),
    queryFn: () =>
      reviewService.history(slug, engagement ?? "", counterparty ?? ""),
    enabled: !!engagement && !!counterparty,
    staleTime: 60_000,
  });
}

/**
 * Effective auto-accept threshold for a suggestion kind: pending suggestions
 * are the ones whose confidence stayed below it.
 */
export function useReviewThreshold(kind = "account_code") {
  const { slug } = useTenant();
  const query = useQuery({
    queryKey: reviewKeys.settings(slug),
    queryFn: () => reviewService.settings(slug),
    staleTime: 5 * 60_000,
  });
  const policy =
    query.data?.effective[kind] ??
    query.data?.effective["*"] ??
    query.data?.default;
  const value = policy ? Number(policy.threshold) : NaN;
  return Number.isFinite(value) && value > 0 ? value : DEFAULT_THRESHOLD;
}

/** Accounts of the engagement's active plan, for the code picker. */
export function useAccountPlanAccounts(
  engagement: string | undefined,
  q: string,
  enabled = true,
) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: reviewKeys.accounts(slug, engagement ?? "", q),
    queryFn: () => reviewService.accounts(slug, engagement ?? "", q),
    enabled: enabled && !!engagement,
    placeholderData: keepPreviousWithin(engagement ?? ""),
    staleTime: 60_000,
    retry: false,
  });
}

export type ReviewDecisionInput = {
  suggestion: ReviewSuggestion;
  action: ReviewAction;
  reason?: string;
  /** correct only: the value to apply (same shape as `proposed_value`). */
  value?: unknown;
};

/**
 * Accept / reject / correct / revert one suggestion. Errors are not toasted
 * globally; the caller shows them (409 = already decided).
 */
export function useReviewDecision() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      suggestion,
      action,
      reason,
      value,
    }: ReviewDecisionInput) => {
      if (!suggestion.engagement_uuid) {
        throw new Error("suggestion has no engagement");
      }
      const body: { reason?: string; value?: unknown } = {};
      if (reason?.trim()) body.reason = reason.trim();
      if (action === "correct") body.value = value;
      return reviewService.decide(
        slug,
        suggestion.engagement_uuid,
        suggestion.uuid,
        action,
        body,
      );
    },
    onSettled: () => qc.invalidateQueries({ queryKey: reviewKeys.all(slug) }),
  });
}

/** Asks the server for account-code suggestions for one invoice (#160). */
export function useSuggestInvoiceAccounts() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      engagement,
      invoice,
    }: {
      engagement: string;
      invoice: string;
    }) => reviewService.suggestInvoice(slug, engagement, invoice),
    onSettled: () => qc.invalidateQueries({ queryKey: reviewKeys.all(slug) }),
  });
}

// --- Suggestion helpers -----------------------------------------------------

export function confidenceOf(s: ReviewSuggestion) {
  const n = Number(s.confidence);
  return Number.isFinite(n) ? n : 0;
}

/** `{account_code, account_name?}` of an account_code value, when present. */
export function accountValue(value: unknown): {
  code?: string;
  name?: string;
} {
  if (!value || typeof value !== "object") return {};
  const v = value as Record<string, unknown>;
  return {
    code: typeof v.account_code === "string" ? v.account_code : undefined,
    name: typeof v.account_name === "string" ? v.account_name : undefined,
  };
}

/** Line / invoice context the account rules engine records in provenance. */
export function suggestionContext(s: ReviewSuggestion) {
  const inputs = s.provenance.inputs ?? {};
  const str = (k: string) => {
    const v = inputs[k];
    return typeof v === "string" || typeof v === "number"
      ? String(v)
      : undefined;
  };
  return {
    invoiceUuid: str("invoice_uuid"),
    direction: str("direction"),
    invoiceType: str("invoice_type"),
    counterparty: str("counterparty_name"),
    itemName: str("item_name"),
    lineType: str("line_type"),
    taxRate: str("tax_rate"),
    lineNo: str("line_no"),
    // Belge bilgisi (#569): hangi fatura, hangi tarih, ne kadar.
    counterpartyTaxId: str("counterparty_tax_id"),
    invoiceNumber: str("invoice_number"),
    issueDate: str("issue_date"),
    invoiceTotal: str("invoice_total"),
    lineAmount: str("line_amount"),
    // Fişten okunan alan (#569): fotoğrafı gösterilecek fiş kaydı.
    receiptUuid: str("receipt_uuid"),
  };
}

/**
 * Short label for a suggestion value: account code, tr-TR amount, date,
 * match score. Never raw JSON (#375).
 */
export function valueLabel(value: unknown) {
  return valueText(value);
}

/**
 * The corrected value keeps the proposed shape with the new code; a code
 * picked from the plan names an existing account, so "open a new account"
 * flags from the proposal no longer apply.
 */
export function correctedAccountValue(
  proposed: unknown,
  code: string,
  name?: string,
) {
  const base =
    proposed && typeof proposed === "object"
      ? { ...(proposed as Record<string, unknown>) }
      : {};
  base.account_code = code;
  if (name) {
    base.account_name = name;
    if ("open_new" in base) base.open_new = false;
  } else {
    delete base.account_name;
    delete base.open_new;
  }
  return base;
}

/** Mirrors the server's `engine.ValidCode`. */
export const ACCOUNT_CODE_RE = /^[0-9]{3}(\.[0-9A-Za-z]+)*$/;

/** Readable text for a failed decision (409 = already decided elsewhere). */
export function useDecisionErrorText() {
  const { t } = useLocale();
  return useCallback(
    (err: unknown) => {
      if (isApiError(err)) {
        if (err.status === 409) return t("office.review_api.error_conflict");
        if (err.status === 404) return t("office.review_api.error_not_found");
        if (err.status === 403) return t("office.review_api.error_forbidden");
        return err.message;
      }
      return t("office.review_api.error_generic");
    },
    [t],
  );
}
