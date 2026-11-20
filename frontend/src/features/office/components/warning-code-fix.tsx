"use client";

import { useId, useState } from "react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  correctedAccountValue,
  useAccountPlanAccounts,
  useReviewDecision,
  useReviewSuggestions,
  valueLabel,
} from "../hooks/use-review";
import { useDecideWarning } from "../hooks/use-warnings";
import {
  REVIEW_PAGE_MAX,
  type ReviewSuggestion,
} from "../services/review.service";
import type { ApiDataWarning } from "../services/warnings.service";
import { WARNINGS_WRITE_PERMISSION } from "./warning-actions";

/** Fatura kalemi uyarıları (#570): kalemin hesap kodu panelden düzeltilir. */
const ITEM_ENTITY = "invoice_item";

/**
 * Uyarının kalemine ait bekleyen hesap kodu önerisi: aynı fatura, aynı
 * kalem adı, kalemin gider hesabı alanı. Uyarı kalemin kimliğini değil
 * faturasını taşır; kalem adı ayırt eder.
 */
export function itemSuggestionFor(
  warning: Pick<ApiDataWarning, "entity_uuid" | "params">,
  suggestions: ReviewSuggestion[],
): ReviewSuggestion | null {
  const invoice = warning.entity_uuid;
  const name =
    typeof warning.params?.name === "string" ? warning.params.name : "";
  if (!invoice || !name) return null;
  return (
    suggestions.find(
      (s) =>
        s.status === "pending" &&
        s.target_type === ITEM_ENTITY &&
        s.target_field === "account_code" &&
        s.provenance.inputs.invoice_uuid === invoice &&
        s.provenance.inputs.item_name === name,
    ) ?? null
  );
}

/**
 * "Kodu düzelt" (#570): fatura kalemi uyarısında (demirbaş adayı, olağandışı
 * kayıt) kalemin bekleyen hesap kodu önerisi inceleme kuyruğunun düzeltme
 * ucuyla düzeltilir; ardından uyarı yeni kodla "Çözüldü" olarak kapanır.
 * Kalemin bekleyen önerisi yoksa (kod zaten işlenmiş) hiçbir şey göstermez.
 */
export function WarningCodeFix({
  warning,
  onDone,
}: {
  warning: ApiDataWarning;
  onDone: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const id = useId();
  const [code, setCode] = useState("");
  const [error, setError] = useState("");
  const enabled =
    warning.status === "open" &&
    warning.entity_type === ITEM_ENTITY &&
    !!warning.entity_uuid &&
    can(Permission.TenantReviewDecide) &&
    can(WARNINGS_WRITE_PERMISSION);
  const list = useReviewSuggestions(
    {
      engagement: warning.engagement_uuid,
      status: "pending",
      kind: "account_code",
      period: warning.period ?? undefined,
      limit: REVIEW_PAGE_MAX,
      offset: 0,
    },
    enabled,
  );
  const accounts = useAccountPlanAccounts(
    warning.engagement_uuid,
    code.trim(),
    enabled && code.trim().length >= 2,
  );
  const decision = useReviewDecision();
  const decideWarning = useDecideWarning(slug);
  if (!enabled) return null;
  const suggestion = itemSuggestionFor(warning, list.data?.items ?? []);
  if (!suggestion) return null;

  const busy = decision.isPending || decideWarning.isPending;
  const value = code.trim();
  const options = accounts.data?.items ?? [];
  const submit = async () => {
    if (!value) return;
    setError("");
    try {
      const picked = options.find((a) => a.code === value);
      await decision.mutateAsync({
        suggestion: {
          ...suggestion,
          engagement_uuid:
            suggestion.engagement_uuid ?? warning.engagement_uuid,
        },
        action: "correct",
        value: correctedAccountValue(
          suggestion.proposed_value,
          value,
          picked?.name,
        ),
        reason: t("office.review_api.reason_correct"),
      });
      await decideWarning.mutateAsync({
        engagement: warning.engagement_uuid,
        warning: warning.uuid,
        action: "resolve",
        note: t("office.warning_code_fix.note", { code: value }),
      });
      onDone();
    } catch (e) {
      setError(
        (e instanceof Error && e.message) || t("office.warning_code_fix.error"),
      );
    }
  };

  return (
    <section
      className="border-line space-y-2 rounded-xl border p-3"
      aria-labelledby={`${id}-title`}
      data-testid="warning-code-fix"
    >
      <h4 id={`${id}-title`} className="text-sm font-semibold">
        {t("office.warning_code_fix.title")}
      </h4>
      <p className="text-text-2 text-sm">
        {t("office.warning_code_fix.current", {
          code: valueLabel(suggestion.proposed_value),
        })}
      </p>
      <form
        className="flex flex-wrap items-end gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          void submit();
        }}
      >
        <div className="flex flex-col gap-1">
          <Label htmlFor={`${id}-code`} className="text-text-2 text-xs">
            {t("office.warning_code_fix.label")}
          </Label>
          <Input
            id={`${id}-code`}
            list={`${id}-accounts`}
            value={code}
            onChange={(e) => setCode(e.target.value)}
            placeholder="255.01"
            autoComplete="off"
            className="w-40"
          />
          <datalist id={`${id}-accounts`}>
            {options.slice(0, 20).map((a) => (
              <option key={a.code} value={a.code}>
                {a.name}
              </option>
            ))}
          </datalist>
        </div>
        <Button
          type="submit"
          size="sm"
          className="min-h-11 md:min-h-9"
          disabled={busy || !value}
        >
          {t("office.review.correct")}
        </Button>
      </form>
      {error ? (
        <p role="alert" className="text-destructive text-xs">
          {error}
        </p>
      ) : null}
    </section>
  );
}
