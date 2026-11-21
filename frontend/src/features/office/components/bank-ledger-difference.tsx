"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";

import { StatusPill } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { FOCUS_PARAM } from "../lib/related-href";
import { journalKeys } from "../services/journals.service";
import { errText } from "./engagement-expenses-page";
import { Money } from "./office-ui";

export type DifferenceAccount = components["schemas"]["BankDifferenceAccount"];
export type DifferenceReason = components["schemas"]["BankDifferenceReason"];
type DifferenceRecord = components["schemas"]["BankDifferenceRecord"];
type Resolution = components["schemas"]["BankDifferenceResolution"];
type Report = {
  period: string;
  from_period: string;
  items: DifferenceAccount[];
};

/** En çok bu kadar kayıt gösterilir; fazlası "daha fazla" ile açılır. */
const RECORDS_SHOWN = 8;

/** Karşı hesap önerisi: nedene göre en sık kullanılan hesap. */
export const COUNTER_HINT: Record<string, string> = {
  opening: "500",
  unbooked: "689",
  other_account: "102",
  manual_lines: "689",
  gaps: "689",
  duplicates: "689",
  unexplained: "689",
};

const day = (s: string) => s.split("-").reverse().join(".");

function useDifference(engagement: string, period: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [
      "tenant",
      slug.toLowerCase(),
      "bank-ledger-difference",
      engagement,
      period,
    ],
    queryFn: () =>
      tenantRequest<Report>(
        slug,
        "GET",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-ledger-difference`,
        { query: { period } },
      ),
    enabled: !!engagement && !!period,
  });
}

type DialogState = {
  account: DifferenceAccount;
  reason: DifferenceReason;
  action: "explained" | "corrected";
} | null;

/**
 * Banka bakiyesi ile muhasebe (102) bakiyesi farkı (#780 C): ekstrenin
 * dönem sonu bakiyesi, hesabın 102 bakiyesiyle (yılbaşından, taslaklar
 * dahil) karşılaştırılır; fark nedenlerine ayrılır. Her neden tutarıyla ve
 * kayıtlarına bağlantıyla görünür; ofis düzeltme fişi keser ya da nedeni
 * "açıklandı" işaretler. Kim, ne zaman yaptı yazar.
 */
export function BankLedgerDifferencePanel({
  engagement,
  period,
}: {
  engagement: string;
  period: string;
}) {
  const { t } = useLocale();
  const q = useDifference(engagement, period);
  const [dialog, setDialog] = useState<DialogState>(null);
  const items = q.data?.items ?? [];
  return (
    <section
      className="bg-surface border-line space-y-4 rounded-xl border p-4"
      data-testid="bank-ledger-difference"
    >
      <div className="space-y-1">
        <h2 className="font-display text-base font-semibold">
          {t("office.ledger_diff.title")}
        </h2>
        <p className="text-text-2 text-sm">
          {t("office.ledger_diff.description")}
        </p>
        <p className="text-text-3 text-xs">{t("office.ledger_diff.why")}</p>
      </div>
      {!period ? (
        <p className="text-text-2 text-sm">
          {t("office.ledger_diff.pick_period")}
        </p>
      ) : q.isLoading ? (
        <Skeleton className="h-32 w-full" />
      ) : q.isError ? (
        <p className="text-danger text-sm">{t("office.load_failed")}</p>
      ) : items.length === 0 ? (
        <p className="text-text-2 text-sm">{t("office.ledger_diff.empty")}</p>
      ) : (
        items.map((a) => (
          <AccountCard
            key={a.cash_box_uuid}
            engagement={engagement}
            period={period}
            account={a}
            onAction={(reason, action) =>
              setDialog({ account: a, reason, action })
            }
          />
        ))
      )}
      <ResolveDialog
        engagement={engagement}
        period={period}
        state={dialog}
        onClose={() => setDialog(null)}
      />
    </section>
  );
}

function AccountCard({
  engagement,
  period,
  account: a,
  onAction,
}: {
  engagement: string;
  period: string;
  account: DifferenceAccount;
  onAction: (r: DifferenceReason, action: "explained" | "corrected") => void;
}) {
  const { t } = useLocale();
  const open = Number(a.open_total);
  return (
    <article
      className="border-line space-y-3 rounded-lg border p-3"
      data-testid="difference-account"
      data-account={a.account_code}
    >
      <div className="flex flex-wrap items-start justify-between gap-2">
        <div className="min-w-0">
          <h3 className="truncate text-sm font-semibold">
            {a.name || a.iban || "—"}
          </h3>
          <p className="text-text-3 text-xs">
            {t("office.ledger_diff.account_code", { code: a.account_code })}
          </p>
        </div>
        {a.difference === null ? (
          <StatusPill tone="neutral">
            {t("office.ledger_diff.no_statement")}
          </StatusPill>
        ) : a.balanced ? (
          <StatusPill tone="ok">{t("office.ledger_diff.balanced")}</StatusPill>
        ) : open === 0 ? (
          <StatusPill tone="info">
            {t("office.ledger_diff.all_handled")}
          </StatusPill>
        ) : (
          <StatusPill tone="danger">
            {t("office.ledger_diff.unbalanced")}
          </StatusPill>
        )}
      </div>
      <dl className="grid gap-2 text-sm sm:grid-cols-3">
        <Figure
          label={t("office.ledger_diff.statement", {
            date: a.statement_date ? day(a.statement_date) : "—",
          })}
          value={a.statement_balance}
        />
        <Figure
          label={t("office.ledger_diff.ledger", { code: a.account_code })}
          value={a.ledger_balance}
        />
        <Figure
          label={t("office.ledger_diff.difference")}
          value={a.difference}
          tone={
            a.difference === null ? undefined : a.balanced ? "ok" : "danger"
          }
        />
      </dl>
      {a.shared_code ? (
        <p className="text-warn text-xs">
          {t("office.ledger_diff.shared_code")}
        </p>
      ) : null}
      {a.difference === null ? (
        <p className="text-text-2 text-sm">
          {t("office.ledger_diff.no_statement_hint")}
        </p>
      ) : a.reasons.length > 0 ? (
        <div className="space-y-2">
          <p className="text-sm font-medium">
            {t("office.ledger_diff.reasons_title")}
          </p>
          <ul className="space-y-2">
            {a.reasons.map((r) => (
              <ReasonRow
                key={r.reason}
                engagement={engagement}
                period={period}
                reason={r}
                onAction={(action) => onAction(r, action)}
              />
            ))}
          </ul>
        </div>
      ) : null}
    </article>
  );
}

function Figure({
  label,
  value,
  tone,
}: {
  label: string;
  value: string | null;
  tone?: "ok" | "danger";
}) {
  return (
    <div className="bg-surface-2 rounded-md px-3 py-2">
      <dt className="text-text-3 text-xs">{label}</dt>
      <dd
        className={cn(
          "font-semibold tabular-nums",
          tone === "ok" && "text-ok",
          tone === "danger" && "text-danger",
        )}
      >
        {value === null ? "—" : <Money value={value} />}
      </dd>
    </div>
  );
}

function ReasonRow({
  engagement,
  period,
  reason: r,
  onAction,
}: {
  engagement: string;
  period: string;
  reason: DifferenceReason;
  onAction: (action: "explained" | "corrected") => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can("tenant.bank.write");
  const canBook = canWrite && can("tenant.journals.write");
  const [all, setAll] = useState(false);
  const qc = useQueryClient();
  const undo = useMutation({
    mutationFn: (id: string) =>
      tenantRequest<void>(
        slug,
        "DELETE",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-ledger-difference/resolutions/${encodeURIComponent(id)}`,
      ),
    onSuccess: () => {
      toast.success(t("office.ledger_diff.undo_done"));
      void qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase(), "bank-ledger-difference"],
      });
    },
    onError: (e) => toast.error(errText(e, t("office.ledger_diff.failed"))),
  });
  const open = Number(r.open);
  const shown = all ? r.records : r.records.slice(0, RECORDS_SHOWN);
  return (
    <li
      className="border-line space-y-2 rounded-md border p-3"
      data-testid="difference-reason"
      data-reason={r.reason}
    >
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <span className="text-sm font-semibold">
          {t(`office.ledger_diff.reason_${r.reason}`)}
        </span>
        <span className="flex items-baseline gap-3 text-sm">
          <Money value={r.amount} className="font-semibold" />
          {Number(r.corrected) !== 0 ? (
            <span className="text-text-3 text-xs">
              {t("office.ledger_diff.corrected")} <Money value={r.corrected} />
            </span>
          ) : null}
        </span>
      </div>
      <p className="text-text-2 text-xs">
        {t(`office.ledger_diff.reason_${r.reason}_hint`)}
      </p>
      {shown.length > 0 ? (
        <ul className="divide-line divide-y text-xs">
          {shown.map((x, i) => (
            <RecordRow
              key={`${x.kind}-${x.transaction_uuid ?? x.draft_uuid ?? ""}-${i}`}
              engagement={engagement}
              period={period}
              record={x}
            />
          ))}
        </ul>
      ) : null}
      {r.records.length > RECORDS_SHOWN || r.more > 0 ? (
        <Button
          size="sm"
          variant="ghost"
          onClick={() => setAll((v) => !v)}
          disabled={r.records.length <= RECORDS_SHOWN}
        >
          {all
            ? t("office.ledger_diff.show_less")
            : t("office.ledger_diff.show_more", {
                count: r.records.length - RECORDS_SHOWN + r.more,
              })}
        </Button>
      ) : null}
      {r.corrections.map((c) => (
        <ResolutionNote key={c.uuid} engagement={engagement} res={c} />
      ))}
      {r.explained ? (
        <div className="flex flex-wrap items-center gap-2">
          <ResolutionNote engagement={engagement} res={r.explained} />
          {canWrite ? (
            <Button
              size="sm"
              variant="ghost"
              disabled={undo.isPending}
              onClick={() => r.explained && undo.mutate(r.explained.uuid)}
            >
              {t("office.ledger_diff.undo")}
            </Button>
          ) : null}
        </div>
      ) : open !== 0 && canWrite ? (
        <div className="flex flex-wrap gap-2">
          {canBook ? (
            <Button size="sm" onClick={() => onAction("corrected")}>
              {t("office.ledger_diff.correct")}
            </Button>
          ) : null}
          <Button
            size="sm"
            variant="outline"
            onClick={() => onAction("explained")}
          >
            {t("office.ledger_diff.explain")}
          </Button>
        </div>
      ) : null}
    </li>
  );
}

function RecordRow({
  engagement,
  period,
  record: x,
}: {
  engagement: string;
  period: string;
  record: DifferenceRecord;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  let href = "";
  let label = "";
  if (x.kind === "bank_transaction" && x.transaction_uuid) {
    href = `${routes.tenant.engagementBank(slug, engagement)}?period=${x.period}&${FOCUS_PARAM}=${encodeURIComponent(`bank_transaction:${x.transaction_uuid}`)}`;
    label = t("office.ledger_diff.open_movement");
  } else if (x.kind === "journal_line" && x.draft_uuid) {
    href = `${routes.tenant.engagementJournals(slug, engagement)}?${FOCUS_PARAM}=${encodeURIComponent(`journal_draft:${x.draft_uuid}`)}`;
    label = t("office.ledger_diff.open_entry", { no: x.entry_no ?? "" });
  } else {
    href = `${routes.tenant.engagementBank(slug, engagement)}?period=${x.period || period}`;
    label = t("office.ledger_diff.open_statements");
  }
  return (
    <li className="flex flex-wrap items-center gap-2 py-1.5">
      <span className="tabular-nums">
        {x.kind === "gap" && x.to_date
          ? t("office.ledger_diff.gap_range", {
              from: day(x.date),
              to: day(x.to_date),
            })
          : day(x.date)}
      </span>
      <span className="text-text-2 min-w-0 flex-1 truncate">
        {x.description ||
          (x.account_code
            ? t("office.ledger_diff.on_account", { code: x.account_code })
            : "—")}
      </span>
      <Money value={x.amount} />
      <Link href={href} className="text-brand-text font-medium hover:underline">
        {label}
      </Link>
    </li>
  );
}

function ResolutionNote({
  engagement,
  res,
}: {
  engagement: string;
  res: Resolution;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const when = new Date(res.created_at).toLocaleString(
    locale === "en" ? "en-GB" : "tr-TR",
    { dateStyle: "short", timeStyle: "short" },
  );
  const who = res.created_by || t("office.ledger_diff.someone");
  return (
    <p className="text-text-2 text-xs" data-testid="difference-resolution">
      {res.action === "explained"
        ? t("office.ledger_diff.explained_by", {
            who,
            when,
            text: res.explanation,
          })
        : t("office.ledger_diff.corrected_by", {
            who,
            when,
            amount: res.amount ?? "",
            account: res.counter_account ?? "",
            text: res.explanation,
          })}{" "}
      {res.journal_draft_uuid ? (
        <Link
          className="text-brand-text font-medium hover:underline"
          href={`${routes.tenant.engagementJournals(slug, engagement)}?${FOCUS_PARAM}=${encodeURIComponent(`journal_draft:${res.journal_draft_uuid}`)}`}
        >
          {t("office.ledger_diff.open_draft")}
        </Link>
      ) : null}
    </p>
  );
}

function ResolveDialog({
  engagement,
  period,
  state,
  onClose,
}: {
  engagement: string;
  period: string;
  state: DialogState;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const key = state
    ? `${state.account.cash_box_uuid}-${state.reason.reason}-${state.action}`
    : "";
  const [form, setForm] = useState({
    key: "",
    counter: "",
    amount: "",
    text: "",
  });
  const current =
    state && form.key === key
      ? form
      : {
          key,
          counter: state ? (COUNTER_HINT[state.reason.reason] ?? "689") : "",
          amount: state?.reason.open ?? "",
          text: "",
        };
  const corrected = state?.action === "corrected";
  const save = useMutation({
    mutationFn: () =>
      tenantRequest<unknown>(
        slug,
        "POST",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-ledger-difference/resolutions`,
        {
          body: {
            cash_box_uuid: state?.account.cash_box_uuid,
            period,
            reason: state?.reason.reason,
            action: state?.action,
            explanation: current.text.trim(),
            ...(corrected
              ? {
                  counter_account: current.counter.trim(),
                  amount: current.amount.trim().replace(",", "."),
                }
              : {}),
          },
        },
      ),
    onSuccess: () => {
      toast.success(
        t(
          corrected
            ? "office.ledger_diff.correct_done"
            : "office.ledger_diff.explain_done",
        ),
      );
      void qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase(), "bank-ledger-difference"],
      });
      void qc.invalidateQueries({
        queryKey: journalKeys.all(slug, engagement),
      });
      onClose();
    },
    onError: (e) => toast.error(errText(e, t("office.ledger_diff.failed"))),
  });
  const amount = Number(current.amount.replace(",", "."));
  const valid =
    current.text.trim() !== "" &&
    (!corrected ||
      (/^\d{3}(\.[0-9A-Za-z]+)*$/.test(current.counter.trim()) &&
        Number.isFinite(amount) &&
        amount !== 0));
  return (
    <Dialog open={state !== null} onOpenChange={(o) => (o ? null : onClose())}>
      <DialogContent data-testid="difference-dialog">
        <DialogHeader>
          <DialogTitle>
            {corrected
              ? t("office.ledger_diff.correct_title")
              : t("office.ledger_diff.explain_title")}
          </DialogTitle>
          <DialogDescription>
            {state
              ? `${state.account.name || state.account.iban} · ${t(`office.ledger_diff.reason_${state.reason.reason}`)}`
              : ""}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            if (valid) save.mutate();
          }}
        >
          {corrected ? (
            <>
              <p className="text-text-2 text-xs">
                {t("office.ledger_diff.correct_hint", {
                  code: state?.account.account_code ?? "102",
                })}
              </p>
              <div className="grid gap-3 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="diff-counter">
                    {t("office.ledger_diff.counter_account")}
                  </Label>
                  <Input
                    id="diff-counter"
                    value={current.counter}
                    inputMode="decimal"
                    onChange={(e) =>
                      setForm({ ...current, counter: e.target.value })
                    }
                  />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="diff-amount">
                    {t("office.ledger_diff.amount")}
                  </Label>
                  <Input
                    id="diff-amount"
                    value={current.amount}
                    inputMode="decimal"
                    onChange={(e) =>
                      setForm({ ...current, amount: e.target.value })
                    }
                  />
                </div>
              </div>
              <p className="text-text-3 text-xs">
                {amount > 0
                  ? t("office.ledger_diff.direction_debit", {
                      code: state?.account.account_code ?? "102",
                    })
                  : amount < 0
                    ? t("office.ledger_diff.direction_credit", {
                        code: state?.account.account_code ?? "102",
                      })
                    : ""}
              </p>
            </>
          ) : (
            <p className="text-text-2 text-xs">
              {t("office.ledger_diff.explain_hint")}
            </p>
          )}
          <div className="space-y-1.5">
            <Label htmlFor="diff-text">
              {t("office.ledger_diff.explanation")}
            </Label>
            <Textarea
              id="diff-text"
              value={current.text}
              maxLength={1000}
              onChange={(e) => setForm({ ...current, text: e.target.value })}
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!valid || save.isPending}>
              {corrected
                ? t("office.ledger_diff.correct_submit")
                : t("office.ledger_diff.explain_submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
