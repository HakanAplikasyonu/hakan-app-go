"use client";

import { Check, Pencil, X } from "lucide-react";
import { useId, useMemo, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  ACCOUNT_CODE_RE,
  accountValue,
  correctedAccountValue,
  suggestionContext,
  useAccountPlanAccounts,
  useDecisionErrorText,
  useReviewDecision,
  useReviewSuggestions,
} from "../hooks/use-review";
import {
  REVIEW_PAGE_MAX,
  type ReviewSuggestion,
} from "../services/review.service";
import { Panel } from "./office-ui";

const KIND = "account_code";
const DECIDE_PERMISSION = "tenant.review.decide";

/** Eski öneriler (yerine yenisi gelen, geri alınan) gösterilmez. */
const HIDDEN = new Set(["superseded", "reverted"]);

/**
 * Fatura detayında hesap kodları (#669 F): satır, KDV ve cari hesap kodu
 * faturadan çıkmadan görülür, onaylanır ya da değiştirilir. Aynı alanın
 * birden fazla önerisi varsa en güncel olanı gösterilir.
 */
export function InvoiceAccountCodes({
  engagement,
  invoice,
}: {
  engagement: string;
  invoice: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canDecide = can(DECIDE_PERMISSION);
  const query = useReviewSuggestions({
    engagement,
    invoice,
    status: "all",
    kind: KIND,
    limit: REVIEW_PAGE_MAX,
    offset: 0,
  });
  const rows = useMemo(
    () => latestPerField(query.data?.items ?? []),
    [query.data],
  );
  const [editing, setEditing] = useState<string>();
  const plan = useAccountPlanAccounts(engagement, "", !!editing);

  return (
    <Panel
      id="hesap-kodlari"
      title={t("office.invoice_codes.title")}
      description={
        query.isLoading
          ? undefined
          : rows.length
            ? t("office.invoice_codes.description")
            : t("office.invoice_codes.empty")
      }
    >
      {rows.length ? (
        <ul className="divide-y" data-testid="invoice-account-codes">
          {rows.map((s) => (
            <CodeRow
              key={s.uuid}
              suggestion={s}
              canDecide={canDecide}
              editing={editing === s.uuid}
              onEdit={(on) => setEditing(on ? s.uuid : undefined)}
              planCodes={(plan.data?.items ?? [])
                .filter((a) => a.is_leaf)
                .map((a) => ({ code: a.code, name: a.name }))}
            />
          ))}
        </ul>
      ) : null}
    </Panel>
  );
}

function CodeRow({
  suggestion: s,
  canDecide,
  editing,
  onEdit,
  planCodes,
}: {
  suggestion: ReviewSuggestion;
  canDecide: boolean;
  editing: boolean;
  onEdit: (on: boolean) => void;
  planCodes: { code: string; name: string }[];
}) {
  const { t } = useLocale();
  const decision = useReviewDecision();
  const errorText = useDecisionErrorText();
  const listId = useId();
  const ctx = suggestionContext(s);
  const shown =
    s.status === "pending" || s.status === "rejected"
      ? accountValue(s.proposed_value)
      : accountValue(s.applied_value ?? s.proposed_value);
  const [code, setCode] = useState(shown.code ?? "");
  const valid = ACCOUNT_CODE_RE.test(code.trim());

  const label =
    s.target_type === "customer"
      ? t("office.invoice_codes.customer", { name: ctx.counterparty ?? "" })
      : t("office.invoice_codes.line", {
          no: ctx.lineNo ?? "",
          name: ctx.itemName ?? "",
        });
  const field =
    s.target_field === "vat_account_code"
      ? t("office.invoice_codes.field_vat")
      : s.target_type === "customer"
        ? t("office.invoice_codes.field_customer")
        : t("office.invoice_codes.field_line");

  const save = () => {
    const c = code.trim();
    const name = planCodes.find((a) => a.code === c)?.name;
    decision.mutate(
      {
        suggestion: s,
        action: "correct",
        value: correctedAccountValue(s.proposed_value, c, name),
        reason: t("office.invoice_codes.reason"),
      },
      {
        onSuccess: () => {
          toast.success(t("office.invoice_codes.saved", { code: c }));
          onEdit(false);
        },
        onError: (err) => toast.error(errorText(err)),
      },
    );
  };
  const accept = () =>
    decision.mutate(
      { suggestion: s, action: "accept" },
      { onError: (err) => toast.error(errorText(err)) },
    );

  return (
    <li
      className="flex flex-wrap items-center gap-3 px-4 py-2.5"
      data-testid="invoice-account-code"
    >
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm">{label}</div>
        <div className="text-muted-foreground text-xs">{field}</div>
      </div>
      {editing ? (
        <div className="flex items-center gap-2">
          <Input
            aria-label={t("office.invoice_codes.code_label", { field })}
            className="h-8 w-36 font-mono"
            list={listId}
            value={code}
            onChange={(e) => setCode(e.target.value)}
            onKeyDown={(e) => {
              if (e.key === "Enter" && valid) save();
              if (e.key === "Escape") onEdit(false);
            }}
            autoFocus
          />
          <datalist id={listId}>
            {planCodes.map((a) => (
              <option key={a.code} value={a.code}>
                {a.name}
              </option>
            ))}
          </datalist>
          <Button
            size="sm"
            disabled={!valid || decision.isPending}
            onClick={save}
          >
            {t("office.invoice_codes.save")}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            aria-label={t("common.cancel")}
            onClick={() => onEdit(false)}
          >
            <X className="size-4" />
          </Button>
        </div>
      ) : (
        <>
          <span className="font-mono text-sm">
            {shown.code ?? "—"}
            {shown.name ? (
              <span className="text-muted-foreground ms-1.5 font-sans text-xs">
                {shown.name}
              </span>
            ) : null}
          </span>
          <span
            className="bg-surface-2 rounded-md border px-2 py-0.5 text-xs"
            data-status={s.status}
          >
            {t(`office.invoice_codes.status_${s.status}`)}
          </span>
          {canDecide ? (
            <div className="flex items-center gap-1">
              {s.status === "pending" ? (
                <Button
                  size="sm"
                  variant="outline"
                  disabled={decision.isPending}
                  onClick={accept}
                >
                  <Check className="size-4" />
                  {t("office.invoice_codes.accept")}
                </Button>
              ) : null}
              {s.status !== "rejected" ? (
                <Button
                  size="sm"
                  variant="ghost"
                  onClick={() => {
                    setCode(shown.code ?? "");
                    onEdit(true);
                  }}
                >
                  <Pencil className="size-4" />
                  {t("office.invoice_codes.change")}
                </Button>
              ) : null}
            </div>
          ) : null}
        </>
      )}
    </li>
  );
}

/** Her alan (hedef + alan) için en güncel öneri; satır sırasıyla, cari sonda. */
function latestPerField(items: ReviewSuggestion[]) {
  const best = new Map<string, ReviewSuggestion>();
  for (const s of items) {
    if (HIDDEN.has(s.status)) continue;
    const key = `${s.target_type}:${s.target_id ?? 0}:${s.target_field}`;
    const prev = best.get(key);
    if (!prev || s.updated_at > prev.updated_at) best.set(key, s);
  }
  const lineNo = (s: ReviewSuggestion) =>
    s.target_type === "customer"
      ? Number.MAX_SAFE_INTEGER
      : Number(suggestionContext(s).lineNo ?? 0);
  return [...best.values()].sort(
    (a, b) =>
      lineNo(a) - lineNo(b) || a.target_field.localeCompare(b.target_field),
  );
}
