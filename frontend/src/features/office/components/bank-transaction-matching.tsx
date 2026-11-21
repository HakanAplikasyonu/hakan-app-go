"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  BookOpenCheck,
  History,
  Link2,
  Link2Off,
  ListTree,
  Search,
} from "lucide-react";
import Link from "next/link";
import { useDeferredValue, useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Collapsible,
  CollapsibleContent,
  CollapsibleTrigger,
} from "@/components/ui/collapsible";
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { ACCOUNT_CODE_RE, useAccountPlanAccounts } from "../hooks/use-review";
import { FOCUS_PARAM } from "../lib/related-href";
import { periodLabel } from "../period/period";
import { payrollKeys, payrollService } from "../services/payroll.service";
import {
  bankMatchingKeys,
  bankMatchingService,
  type BankMatchCandidate,
  type BankMatchJournalLineCandidate,
  type BankMatchLineRef,
  type BankTransactionMatching,
} from "../services/bank-matching.service";
import { relatedKeys } from "../services/related.service";
import { reviewKeys } from "../services/review.service";
import { warningKeys } from "../services/warnings.service";
import { errText } from "./engagement-expenses-page";
import { Figure, Money } from "./office-ui";

function trDate(iso: string) {
  const [y, m, d] = iso.split("-");
  return y && m && d ? `${d}.${m}.${y}` : iso;
}

/**
 * Seçili banka hareketinin ayrıntısı ve fatura eşleştirmesi (#528):
 * uyarıdan gelen kullanıcı hareketi görür, puanlı aday faturalardan
 * seçip eşleştirir ya da eşleşmeyi geri alır; dönemin banka uyarıları
 * hemen yenilenir.
 */
export function BankTransactionMatchingPanel({
  engagement,
  transaction,
  onChanged,
}: {
  engagement: string;
  transaction: string;
  /** Eşleşme değişince (beyan ekranı özetini yeniler). */
  onChanged?: () => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can("tenant.bank.write");
  const qc = useQueryClient();
  const key = bankMatchingKeys.one(slug, engagement, transaction);
  const [picked, setPicked] = useState<string[]>([]);
  // Seçilen fiş satırları (#669 G), anahtar "fiş uuid:satır no".
  const [pickedLines, setPickedLines] = useState<string[]>([]);
  // Seçilen bordrolar (#710): net maaş ödemesi.
  const [pickedPayroll, setPickedPayroll] = useState<string[]>([]);
  // Seçilen SGK primleri (#565): aylık prim ödemesi.
  const [pickedSgk, setPickedSgk] = useState<string[]>([]);
  const [term, setTerm] = useState("");
  const deferred = useDeferredValue(term.trim());
  // Elle arama süzgeçleri (#565): düzenleme tarihi ve tutar aralığı.
  const [range, setRange] = useState({ from: "", to: "", min: "", max: "" });
  const deferredRange = useDeferredValue(range);
  const filter = {
    q: deferred.length >= 2 ? deferred : "",
    from: deferredRange.from,
    to: deferredRange.to,
    min: deferredRange.min.trim().replace(",", "."),
    max: deferredRange.max.trim().replace(",", "."),
  };
  const searching = Object.values(filter).some((v) => v !== "");

  const q = useQuery({
    queryKey: key,
    queryFn: () => bankMatchingService.get(slug, engagement, transaction),
  });

  const settle = (v: BankTransactionMatching, message: string) => {
    qc.setQueryData(key, v);
    setPicked([]);
    setPickedLines([]);
    setPickedPayroll([]);
    setPickedSgk([]);
    void qc.invalidateQueries({
      queryKey: [
        "tenant",
        slug.toLowerCase(),
        "bank-matching",
        engagement,
        transaction,
        "line-search",
      ],
    });
    void qc.invalidateQueries({ queryKey: warningKeys.all(slug) });
    // Elle eşleştirme bekleyen öneriyi kuyruktan düşürür (#529).
    void qc.invalidateQueries({ queryKey: reviewKeys.all(slug) });
    void qc.invalidateQueries({
      queryKey: relatedKeys.one(
        slug,
        engagement,
        "bank_transaction",
        transaction,
      ),
    });
    void qc.invalidateQueries({
      queryKey: ["tenant", slug.toLowerCase(), "bank-transactions"],
    });
    // Yeni işlem geçmişe düşer (#565).
    void qc.invalidateQueries({
      queryKey: bankMatchingKeys.history(slug, engagement, transaction),
    });
    toast.success(message);
    onChanged?.();
  };
  const match = useMutation({
    mutationFn: (m: {
      invoices: string[];
      lines: BankMatchLineRef[];
      payroll: string[];
      sgk: string[];
    }) =>
      bankMatchingService.match(
        slug,
        engagement,
        transaction,
        m.invoices,
        m.lines,
        m.payroll,
        m.sgk,
      ),
    onSuccess: (v) =>
      settle(
        v,
        t(
          v.matches.length > 0
            ? "office.bank.matching.matched"
            : v.payroll_matches.length > 0
              ? "office.bank.matching.matched_payroll"
              : v.sgk_matches.length > 0
                ? "office.bank.matching.matched_sgk"
                : "office.bank.matching.matched_lines",
        ),
      ),
    onError: (e) => toast.error(errText(e, t("office.bank.matching.failed"))),
  });
  const unmatch = useMutation({
    mutationFn: () =>
      bankMatchingService.unmatch(slug, engagement, transaction),
    onSuccess: (v) => settle(v, t("office.bank.matching.unmatched")),
    onError: (e) => toast.error(errText(e, t("office.bank.matching.failed"))),
  });
  // Hesaba işle (#565): fatura yoksa cari, avans, gider ya da vergi hesabı.
  const book = useMutation({
    mutationFn: (a: { code: string; name: string }) =>
      bankMatchingService.setAccount(
        slug,
        engagement,
        transaction,
        a.code,
        a.name,
      ),
    onSuccess: (v) => {
      void qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase(), "bank-postings"],
      });
      settle(
        v,
        t("office.bank.matching.account_saved", {
          code: v.posting?.account_code ?? "",
        }),
      );
    },
    onError: (e) =>
      toast.error(errText(e, t("office.bank.matching.account_failed"))),
  });
  const busy = match.isPending || unmatch.isPending || book.isPending;
  // Elle arama (#565): aday penceresinin dışındaki faturalar.
  const search = useQuery({
    queryKey: bankMatchingKeys.search(slug, engagement, transaction, filter),
    queryFn: () =>
      bankMatchingService.search(slug, engagement, transaction, filter),
    enabled: searching && !!q.data?.matchable,
  });

  if (q.isPending) return <Skeleton className="h-32 w-full" />;
  if (q.isError || !q.data)
    return (
      <p className="text-muted-foreground text-sm">
        {t("office.bank.matching.load_failed")}
      </p>
    );
  const v = q.data;
  const invoiceHref = (id: string) =>
    routes.tenant.engagementInvoice(slug, engagement, id);
  const toggle = (id: string, on: boolean) =>
    setPicked((p) => (on ? [...p, id] : p.filter((x) => x !== id)));
  const toggleLine = (id: string, on: boolean) =>
    setPickedLines((p) => (on ? [...p, id] : p.filter((x) => x !== id)));
  const journalHref = (id: string) =>
    `${routes.tenant.engagementJournals(slug, engagement)}?${FOCUS_PARAM}=${encodeURIComponent(`journal_draft:${id}`)}`;
  const payrollHref = (id: string) =>
    `${routes.tenant.engagementPayroll(slug, engagement)}?${FOCUS_PARAM}=${encodeURIComponent(`payroll_run:${id}`)}`;
  const togglePayroll = (id: string, on: boolean) =>
    setPickedPayroll((p) => (on ? [...p, id] : p.filter((x) => x !== id)));
  const toggleSgk = (id: string, on: boolean) =>
    setPickedSgk((p) => (on ? [...p, id] : p.filter((x) => x !== id)));
  const sgkHref = routes.tenant.engagementPayroll(slug, engagement);
  const hasMatches =
    v.matches.length +
      v.journal_matches.length +
      v.payroll_matches.length +
      v.sgk_matches.length >
    0;
  const pickedCount =
    picked.length +
    pickedLines.length +
    pickedPayroll.length +
    pickedSgk.length;
  const submit = () =>
    match.mutate({
      payroll: pickedPayroll,
      sgk: pickedSgk,
      invoices: picked,
      lines: pickedLines.map((k) => {
        const [uuid, no] = k.split(":");
        return { journal_draft_uuid: uuid ?? "", line_no: Number(no) };
      }),
    });

  return (
    <div className="space-y-4" data-testid="bank-transaction-matching">
      <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <dt className="text-muted-foreground">
            {t("office.bank.matching.date")}
          </dt>
          <dd>
            <Figure>{trDate(v.transaction_date)}</Figure>
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">
            {t("office.bank.matching.amount")}
          </dt>
          <dd>
            {v.debit_credit === "A" ? "+" : "−"}
            <Money value={v.amount} currency={v.currency} />
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">
            {t("office.bank.matching.counterparty")}
          </dt>
          <dd>
            {v.counterparty_name ?? "—"}
            {v.counterparty_from_description ? (
              <span className="text-muted-foreground text-xs">
                {" "}
                {t("office.bank.matching.counterparty_guessed")}
              </span>
            ) : null}
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">
            {t("office.bank.matching.status")}
          </dt>
          <dd>
            <Badge
              variant={v.match_status === "matched" ? "default" : "outline"}
            >
              {t(`office.bank.matching.status_${v.match_status}`)}
            </Badge>
          </dd>
        </div>
        <div className="sm:col-span-2 lg:col-span-4">
          <dt className="text-muted-foreground">
            {t("office.bank.matching.description")}
          </dt>
          <dd className="break-words">{v.description || "—"}</dd>
        </div>
      </dl>

      {hasMatches && (
        <section className="space-y-2">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium">
              {v.matches.length > 0
                ? t("office.bank.matching.matches_title")
                : v.payroll_matches.length > 0
                  ? t("office.bank.matching.payroll_matches_title")
                  : v.sgk_matches.length > 0
                    ? t("office.bank.matching.sgk_matches_title")
                    : t("office.bank.matching.line_matches_title")}
            </h3>
            {canWrite && (
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => unmatch.mutate()}
              >
                <Link2Off className="size-4" aria-hidden />
                {t("office.bank.matching.unmatch")}
              </Button>
            )}
          </div>
          {canWrite ? (
            <p className="text-text-2 text-xs">
              {t("office.bank.matching.unmatch_hint")}
            </p>
          ) : null}
          {v.matches.length > 0 ? (
            <ul className="divide-y rounded-md border text-sm">
              {v.matches.map((m) => (
                <li
                  key={m.invoice_uuid}
                  className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
                >
                  <Link
                    className="font-medium underline-offset-4 hover:underline"
                    href={invoiceHref(m.invoice_uuid)}
                  >
                    <Figure>{m.invoice_number}</Figure>
                  </Link>
                  <span className="text-muted-foreground">
                    {trDate(m.issue_date)} · {m.counterparty_name ?? "—"}
                  </span>
                  <Money value={m.amount} currency={v.currency} />
                  <Badge variant="secondary">
                    {t(`office.bank.matching.method_${m.method}`)}
                  </Badge>
                  <MatchReason parts={m.parts} />
                </li>
              ))}
            </ul>
          ) : null}
          {v.payroll_matches.length > 0 ? (
            <ul
              className="divide-y rounded-md border text-sm"
              data-testid="bank-transaction-payroll-matches"
            >
              {v.payroll_matches.map((m) => (
                <li
                  key={m.payroll_run_uuid}
                  className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
                >
                  <Link
                    className="font-medium underline-offset-4 hover:underline"
                    href={payrollHref(m.payroll_run_uuid)}
                  >
                    {t("office.bank.matching.payroll_label", {
                      period: periodLabel(m.period, locale),
                    })}
                  </Link>
                  <span className="text-muted-foreground">
                    {t("office.bank.matching.payroll_net")}{" "}
                    <Money value={m.net_total} currency={v.currency} />
                  </span>
                  <Money value={m.amount} currency={v.currency} />
                  <Badge variant="secondary">
                    {t(`office.bank.matching.method_${m.method}`)}
                  </Badge>
                  <PayrollBreakdown
                    engagement={engagement}
                    run={m.payroll_run_uuid}
                    paid={m.amount}
                    currency={v.currency}
                    href={payrollHref(m.payroll_run_uuid)}
                  />
                </li>
              ))}
            </ul>
          ) : null}
          {v.sgk_matches.length > 0 ? (
            <ul
              className="divide-y rounded-md border text-sm"
              data-testid="bank-transaction-sgk-matches"
            >
              {v.sgk_matches.map((m) => (
                <li
                  key={m.sgk_premium_uuid}
                  className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
                >
                  <Link
                    className="font-medium underline-offset-4 hover:underline"
                    href={sgkHref}
                  >
                    {t("office.bank.matching.sgk_label", {
                      period: periodLabel(m.period, locale),
                    })}
                  </Link>
                  <span className="text-muted-foreground">
                    {t("office.bank.matching.sgk_payable")}{" "}
                    <Money value={m.payable_amount} currency={v.currency} />
                  </span>
                  <Money value={m.amount} currency={v.currency} />
                  <Badge variant="secondary">
                    {t(`office.bank.matching.method_${m.method}`)}
                  </Badge>
                </li>
              ))}
            </ul>
          ) : null}
          {v.journal_matches.length > 0 ? (
            <>
              {v.matches.length > 0 ? (
                <h4 className="text-muted-foreground text-xs font-medium">
                  {t("office.bank.matching.line_matches_title")}
                </h4>
              ) : null}
              <ul
                className="divide-y rounded-md border text-sm"
                data-testid="bank-transaction-line-matches"
              >
                {v.journal_matches.map((m) => (
                  <li
                    key={`${m.journal_draft_uuid}:${m.line_no}`}
                    className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
                  >
                    <Link
                      className="font-medium underline-offset-4 hover:underline"
                      href={journalHref(m.journal_draft_uuid)}
                    >
                      <Figure>{m.label}</Figure>
                    </Link>
                    <span className="text-muted-foreground min-w-0 truncate">
                      {trDate(m.entry_date)} · {m.description || "—"}
                    </span>
                    <Money value={m.amount} currency={v.currency} />
                  </li>
                ))}
              </ul>
            </>
          ) : null}
        </section>
      )}

      {!v.matchable ? (
        <p className="text-muted-foreground text-sm">
          {t("office.bank.matching.not_matchable")}
        </p>
      ) : (
        <section className="space-y-2">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium">
              {t("office.bank.matching.candidates_title")}
            </h3>
          </div>
          {v.candidates.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("office.bank.matching.no_candidates")}
            </p>
          ) : (
            <CandidateList
              items={v.candidates}
              currency={v.currency}
              picked={picked}
              onToggle={canWrite ? toggle : undefined}
              invoiceHref={invoiceHref}
            />
          )}
          {canWrite ? (
            <div className="space-y-2 pt-2">
              <InputGroup className="max-w-md">
                <InputGroupAddon>
                  <Search className="size-4" aria-hidden />
                </InputGroupAddon>
                <InputGroupInput
                  type="search"
                  value={term}
                  onChange={(e) => setTerm(e.target.value)}
                  placeholder={t("office.bank.matching.search_placeholder")}
                  aria-label={t("office.bank.matching.search_placeholder")}
                />
              </InputGroup>
              <div
                className="grid max-w-md grid-cols-2 gap-2"
                data-testid="bank-invoice-search-range"
              >
                {(
                  [
                    ["from", "date"],
                    ["to", "date"],
                    ["min", "text"],
                    ["max", "text"],
                  ] as const
                ).map(([key, type]) => (
                  <div key={key} className="space-y-1">
                    <Label
                      htmlFor={`bank-search-${transaction}-${key}`}
                      className="text-text-2 text-xs"
                    >
                      {t(`office.bank.matching.search_${key}`)}
                    </Label>
                    <Input
                      id={`bank-search-${transaction}-${key}`}
                      type={type}
                      inputMode={type === "text" ? "decimal" : undefined}
                      value={range[key]}
                      onChange={(e) =>
                        setRange((r) => ({ ...r, [key]: e.target.value }))
                      }
                      className="h-8"
                    />
                  </div>
                ))}
              </div>
              {searching ? (
                search.isLoading ? (
                  <Skeleton className="h-16 w-full" />
                ) : (search.data?.items ?? []).length === 0 ? (
                  <p className="text-muted-foreground text-sm">
                    {t("office.bank.matching.search_empty")}
                  </p>
                ) : (
                  <CandidateList
                    items={(search.data?.items ?? []).filter(
                      (c) =>
                        !v.candidates.some(
                          (x) => x.invoice_uuid === c.invoice_uuid,
                        ),
                    )}
                    currency={v.currency}
                    picked={picked}
                    onToggle={toggle}
                    invoiceHref={invoiceHref}
                  />
                )
              ) : (
                <p className="text-muted-foreground text-xs">
                  {t("office.bank.matching.search_hint")}
                </p>
              )}
            </div>
          ) : null}
        </section>
      )}

      {v.payroll_matchable && canWrite && v.payroll_candidates.length > 0 ? (
        <section className="space-y-2" data-testid="bank-payroll-candidates">
          <h3 className="text-sm font-medium">
            {t("office.bank.matching.payroll_title")}
          </h3>
          <p className="text-text-2 text-xs">
            {t("office.bank.matching.payroll_hint")}
          </p>
          <ul className="divide-y rounded-md border text-sm">
            {v.payroll_candidates.map((c) => (
              <li
                key={c.payroll_run_uuid}
                className="flex flex-wrap items-center gap-3 px-3 py-2"
              >
                <Checkbox
                  checked={pickedPayroll.includes(c.payroll_run_uuid)}
                  onCheckedChange={(on) =>
                    togglePayroll(c.payroll_run_uuid, on === true)
                  }
                  aria-label={t("office.bank.matching.pick_payroll", {
                    period: periodLabel(c.period, locale),
                  })}
                />
                <Link
                  className="font-medium underline-offset-4 hover:underline"
                  href={payrollHref(c.payroll_run_uuid)}
                >
                  {t("office.bank.matching.payroll_label", {
                    period: periodLabel(c.period, locale),
                  })}
                </Link>
                <span className="text-muted-foreground">
                  {t("office.bank.matching.payroll_employees", {
                    count: c.employee_count,
                  })}
                </span>
                <span className="ms-auto">
                  {t("office.bank.matching.open")}{" "}
                  <Money value={c.open_amount} currency={v.currency} />
                </span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {v.payroll_matchable && canWrite && v.sgk_candidates.length > 0 ? (
        <section className="space-y-2" data-testid="bank-sgk-candidates">
          <h3 className="text-sm font-medium">
            {t("office.bank.matching.sgk_title")}
          </h3>
          <p className="text-text-2 text-xs">
            {t("office.bank.matching.sgk_hint")}
          </p>
          <ul className="divide-y rounded-md border text-sm">
            {v.sgk_candidates.map((c) => (
              <li
                key={c.sgk_premium_uuid}
                className="flex flex-wrap items-center gap-3 px-3 py-2"
              >
                <Checkbox
                  checked={pickedSgk.includes(c.sgk_premium_uuid)}
                  onCheckedChange={(on) =>
                    toggleSgk(c.sgk_premium_uuid, on === true)
                  }
                  aria-label={t("office.bank.matching.pick_sgk", {
                    period: periodLabel(c.period, locale),
                  })}
                />
                <Link
                  className="font-medium underline-offset-4 hover:underline"
                  href={sgkHref}
                >
                  {t("office.bank.matching.sgk_label", {
                    period: periodLabel(c.period, locale),
                  })}
                </Link>
                <span className="text-muted-foreground">
                  {t("office.bank.matching.sgk_due", {
                    date: trDate(c.due_date),
                  })}
                </span>
                <span className="ms-auto">
                  {t("office.bank.matching.open")}{" "}
                  <Money value={c.open_amount} currency={v.currency} />
                </span>
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {v.line_matchable && canWrite ? (
        <LineSection
          engagement={engagement}
          transaction={transaction}
          currency={v.currency}
          picked={pickedLines}
          onToggle={toggleLine}
          journalHref={journalHref}
        />
      ) : null}

      {canWrite && (v.matchable || v.line_matchable) ? (
        <div className="flex flex-wrap items-center justify-end gap-2">
          {hasMatches && pickedCount > 0 ? (
            <span className="text-muted-foreground text-xs">
              {t("office.bank.matching.rematch_hint")}
            </span>
          ) : null}
          <Button
            size="sm"
            disabled={busy || pickedCount === 0}
            onClick={submit}
          >
            <Link2 className="size-4" aria-hidden />
            {hasMatches
              ? t("office.bank.matching.rematch", {
                  count: String(pickedCount),
                })
              : t("office.bank.matching.match", {
                  count: String(pickedCount),
                })}
          </Button>
        </div>
      ) : null}

      <AccountSection
        engagement={engagement}
        posting={v.posting}
        canWrite={canWrite}
        busy={busy}
        onBook={(code, name) => book.mutate({ code, name })}
      />

      {v.posting && v.debit_credit === "A" && v.currency === "TRY" ? (
        <CommissionSection
          engagement={engagement}
          transaction={transaction}
          amount={v.amount}
          posting={v.posting}
          canWrite={canWrite}
          onSaved={() => {
            void qc.invalidateQueries({ queryKey: key });
            void qc.invalidateQueries({
              queryKey: ["tenant", slug.toLowerCase(), "bank-postings"],
            });
          }}
        />
      ) : null}

      <HistorySection engagement={engagement} transaction={transaction} />
    </div>
  );
}

/**
 * Hareket geçmişi (#565): kim, ne zaman faturaya eşleştirdi, eşleşmeyi
 * geri aldı ya da hesaba işledi. Yalnız bu dosyadaki işlemler listelenir.
 */
function HistorySection({
  engagement,
  transaction,
}: {
  engagement: string;
  transaction: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const h = useQuery({
    queryKey: bankMatchingKeys.history(slug, engagement, transaction),
    queryFn: () => bankMatchingService.history(slug, engagement, transaction),
  });
  const when = new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    dateStyle: "short",
    timeStyle: "short",
  });
  const items = h.data?.items ?? [];
  return (
    <section className="space-y-2" data-testid="bank-transaction-history">
      <h3 className="flex items-center gap-2 text-sm font-medium">
        <History className="size-4" aria-hidden />
        {t("office.bank.matching.history_title")}
      </h3>
      {h.isPending ? (
        <Skeleton className="h-10 w-full" />
      ) : h.isError ? (
        <p className="text-muted-foreground text-sm">
          {t("office.bank.matching.history_failed")}
        </p>
      ) : items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("office.bank.matching.history_empty")}
        </p>
      ) : (
        <ol className="divide-y rounded-md border text-sm">
          {items.map((e) => (
            <li
              key={e.uuid}
              className="flex flex-wrap items-baseline gap-x-3 gap-y-1 px-3 py-2"
            >
              <Figure className="text-muted-foreground shrink-0 text-xs">
                <time dateTime={e.at}>{when.format(new Date(e.at))}</time>
              </Figure>
              <span className="font-medium">
                {e.actor_name ??
                  t("office.bank.matching.history_unknown_actor")}
              </span>
              <span>
                {t(
                  e.action === "matched" &&
                    e.invoices.length === 0 &&
                    e.journal_lines.length > 0
                    ? "office.bank.matching.history_matched_lines"
                    : `office.bank.matching.history_${e.action}`,
                )}
              </span>
              {e.action === "booked" && e.account_code ? (
                <span className="text-muted-foreground">
                  <Figure>{e.account_code}</Figure>
                  {e.account_name ? ` ${e.account_name}` : ""}
                </span>
              ) : e.invoices.length + e.journal_lines.length > 0 ? (
                <Figure className="text-muted-foreground">
                  {[...e.invoices, ...e.journal_lines].join(", ")}
                </Figure>
              ) : null}
              {e.replaced.length > 0 ? (
                <span className="text-muted-foreground text-xs">
                  {t("office.bank.matching.history_replaced", {
                    items: e.replaced.join(", "),
                  })}
                </span>
              ) : null}
            </li>
          ))}
        </ol>
      )}
    </section>
  );
}

/**
 * Hareketin mizan hesabı (#565). Fatura yoksa ya da hareket fatura
 * kapatmıyorsa (maaş, vergi, kira, virman, banka masrafı) müşavir hareketi
 * buradan plandaki bir hesaba işler; fişe girmiş hareket değiştirilmez.
 */
function AccountSection({
  engagement,
  posting,
  canWrite,
  busy,
  onBook,
}: {
  engagement: string;
  posting: BankTransactionMatching["posting"];
  canWrite: boolean;
  busy: boolean;
  onBook: (code: string, name: string) => void;
}) {
  const { t } = useLocale();
  const [term, setTerm] = useState("");
  const q = useDeferredValue(term.trim());
  const plan = useAccountPlanAccounts(engagement, q, canWrite && q.length >= 2);
  const items = q.length >= 2 ? (plan.data?.items ?? []) : [];
  const drafted = posting?.status === "drafted";
  // Planda olmayan ama biçimi doğru kod da yazılabilir (alt hesap açılacaksa).
  const custom = ACCOUNT_CODE_RE.test(q) && !items.some((a) => a.code === q);
  const book = (code: string, name: string) => {
    onBook(code, name);
    setTerm("");
  };
  return (
    <section className="space-y-2" data-testid="bank-transaction-account">
      <h3 className="text-sm font-medium">
        {t("office.bank.matching.account_title")}
      </h3>
      {posting ? (
        <p className="flex flex-wrap items-center gap-2 text-sm">
          <Figure className="font-semibold">{posting.account_code}</Figure>
          <span>{posting.account_name}</span>
          <Badge
            variant={posting.status === "suggested" ? "outline" : "default"}
          >
            {t(`office.bank.matching.account_status_${posting.status}`)}
          </Badge>
          {/* Öneri "onay bekliyor" diyorsa onay burada (#710): yeni müşavir
              inceleme kuyruğunu aramadan öneriyi tek tıkla onaylar. */}
          {posting.status === "suggested" && canWrite ? (
            <Button
              size="sm"
              disabled={busy}
              onClick={() =>
                onBook(posting.account_code, posting.account_name ?? "")
              }
            >
              {t("office.bank.matching.account_approve", {
                code: posting.account_code,
              })}
            </Button>
          ) : null}
        </p>
      ) : (
        <p className="text-muted-foreground text-sm">
          {t("office.bank.matching.account_none")}
        </p>
      )}
      {drafted ? (
        <p className="text-muted-foreground text-xs">
          {t("office.bank.matching.account_drafted_hint")}
        </p>
      ) : canWrite ? (
        <div className="space-y-2">
          <InputGroup className="max-w-md">
            <InputGroupAddon>
              <Search className="size-4" aria-hidden />
            </InputGroupAddon>
            <InputGroupInput
              type="search"
              value={term}
              onChange={(e) => setTerm(e.target.value)}
              placeholder={t("office.bank.matching.account_search")}
              aria-label={t("office.bank.matching.account_search")}
            />
          </InputGroup>
          {q.length < 2 ? (
            <p className="text-muted-foreground text-xs">
              {t("office.bank.matching.account_hint")}
            </p>
          ) : plan.isLoading ? (
            <Skeleton className="h-16 w-full" />
          ) : items.length === 0 && !custom ? (
            <p className="text-muted-foreground text-sm">
              {t("office.bank.matching.account_empty")}
            </p>
          ) : (
            <ul className="divide-y rounded-md border text-sm">
              {custom ? (
                <AccountRow
                  code={q}
                  name={t("office.bank.matching.account_custom")}
                  disabled={busy}
                  onBook={() => book(q, "")}
                />
              ) : null}
              {items.map((a) => (
                <AccountRow
                  key={a.code}
                  code={a.code}
                  name={a.name}
                  parent={!a.is_leaf}
                  disabled={busy}
                  onBook={() => book(a.code, a.name)}
                />
              ))}
            </ul>
          )}
        </div>
      ) : null}
    </section>
  );
}

/** POS komisyonu gider hesapları (#565). */
const COMMISSION_ACCOUNTS = ["653", "780"] as const;

/**
 * POS tahsilatında komisyonun netlenmesi (#565): banka brütten komisyonu
 * düşüp neti yatırır. Müşavir komisyonu girer; fişte banka net, komisyon
 * 653 / 780 borç, karşı hesap brüt (net + komisyon) alacak olur.
 */
function CommissionSection({
  engagement,
  transaction,
  amount,
  posting,
  canWrite,
  onSaved,
}: {
  engagement: string;
  transaction: string;
  amount: string;
  posting: NonNullable<BankTransactionMatching["posting"]>;
  canWrite: boolean;
  onSaved: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [value, setValue] = useState("");
  const [code, setCode] = useState<string>(COMMISSION_ACCOUNTS[0]);
  const drafted = posting.status === "drafted";
  const save = useMutation({
    mutationFn: (v: string | null) =>
      bankMatchingService.setCommission(slug, engagement, transaction, v, code),
    onSuccess: (_, v) => {
      setValue("");
      toast.success(
        t(
          v === null
            ? "office.bank.matching.commission_removed"
            : "office.bank.matching.commission_saved",
        ),
      );
      onSaved();
    },
    onError: (e) =>
      toast.error(errText(e, t("office.bank.matching.commission_failed"))),
  });
  const typed = value.trim().replace(/\./g, "").replace(",", ".");
  const valid = /^\d+(\.\d{1,2})?$/.test(typed) && Number(typed) > 0;
  if (!posting.commission_amount && (drafted || !canWrite)) return null;
  return (
    <section className="space-y-2" data-testid="bank-transaction-commission">
      <h3 className="text-sm font-medium">
        {t("office.bank.matching.commission_title")}
      </h3>
      {posting.commission_amount ? (
        <div className="flex flex-wrap items-center gap-x-4 gap-y-1 text-sm">
          <span>
            {t("office.bank.matching.commission_net")}{" "}
            <Money value={amount} currency="TRY" />
          </span>
          <span>
            {t("office.bank.matching.commission_amount_label")}{" "}
            <Money value={posting.commission_amount} currency="TRY" /> (
            <Figure>{posting.commission_account_code}</Figure>)
          </span>
          <span className="font-medium">
            {t("office.bank.matching.commission_gross")}{" "}
            <Money value={posting.gross_amount ?? amount} currency="TRY" />
          </span>
          {canWrite && !drafted ? (
            <Button
              size="sm"
              variant="outline"
              disabled={save.isPending}
              onClick={() => save.mutate(null)}
            >
              {t("office.bank.matching.commission_remove")}
            </Button>
          ) : null}
        </div>
      ) : (
        <>
          <p className="text-text-2 text-xs">
            {t("office.bank.matching.commission_hint")}
          </p>
          <div className="flex flex-wrap items-end gap-2">
            <div className="space-y-1">
              <Label htmlFor={`commission-${transaction}`}>
                {t("office.bank.matching.commission_amount_label")}
              </Label>
              <Input
                id={`commission-${transaction}`}
                inputMode="decimal"
                className="w-36"
                value={value}
                onChange={(e) => setValue(e.target.value)}
                placeholder="0,00"
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor={`commission-account-${transaction}`}>
                {t("office.bank.matching.commission_account_label")}
              </Label>
              <NativeSelect
                id={`commission-account-${transaction}`}
                value={code}
                onChange={(e) => setCode(e.target.value)}
              >
                {COMMISSION_ACCOUNTS.map((c) => (
                  <NativeSelectOption key={c} value={c}>
                    {t(`office.bank.matching.commission_account_${c}`)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <Button
              size="sm"
              disabled={!valid || save.isPending}
              onClick={() => save.mutate(typed)}
            >
              {t("office.bank.matching.commission_save")}
            </Button>
          </div>
        </>
      )}
    </section>
  );
}

function AccountRow({
  code,
  name,
  parent,
  disabled,
  onBook,
}: {
  code: string;
  name: string;
  /** Ana hesap: altında alt hesap var, fişe alt hesap yazılır. */
  parent?: boolean;
  disabled: boolean;
  onBook: () => void;
}) {
  const { t } = useLocale();
  return (
    <li className="flex flex-wrap items-center gap-3 px-3 py-2">
      <Figure
        className={cn("w-28 shrink-0 font-semibold", parent && "opacity-60")}
      >
        {code}
      </Figure>
      <span className={cn("min-w-0 flex-1 truncate", parent && "opacity-60")}>
        {name}
      </span>
      {parent ? (
        <span className="text-muted-foreground text-xs">
          {t("office.bank.matching.account_parent")}
        </span>
      ) : (
        <Button
          size="sm"
          variant="outline"
          disabled={disabled}
          onClick={onBook}
          aria-label={t("office.bank.matching.account_book_aria", { code })}
        >
          <BookOpenCheck className="size-4" aria-hidden />
          {t("office.bank.matching.account_book")}
        </Button>
      )}
    </li>
  );
}

/**
 * Fiş satırı / cari hareketi ile eşleştirme (#669 G). Faturası olmayan
 * tahsilat ya da ödeme, daha önce girilmiş bir fişin satırına (cari 120 /
 * 320 alt hesabı, avans, kira …) bağlanır. Terim yoksa tutarı tutan
 * satırlar listelenir; "Yalnız cari hesaplar" cari ana hesaplara daraltır.
 */
function LineSection({
  engagement,
  transaction,
  currency,
  picked,
  onToggle,
  journalHref,
}: {
  engagement: string;
  transaction: string;
  currency: string;
  picked: string[];
  onToggle: (id: string, on: boolean) => void;
  journalHref: (id: string) => string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [term, setTerm] = useState("");
  const [cari, setCari] = useState(false);
  const q = useDeferredValue(term.trim());
  const ready = q.length === 0 || q.length >= 2;
  const lines = useQuery({
    queryKey: bankMatchingKeys.lineSearch(
      slug,
      engagement,
      transaction,
      q,
      cari,
    ),
    queryFn: () =>
      bankMatchingService.searchLines(slug, engagement, transaction, q, cari),
    enabled: ready,
  });
  const items = lines.data?.items ?? [];
  return (
    <section className="space-y-2" data-testid="bank-transaction-lines">
      <h3 className="flex items-center gap-2 text-sm font-medium">
        <ListTree className="size-4" aria-hidden />
        {t("office.bank.matching.lines_title")}
      </h3>
      <div className="flex flex-wrap items-center gap-3">
        <InputGroup className="max-w-md">
          <InputGroupAddon>
            <Search className="size-4" aria-hidden />
          </InputGroupAddon>
          <InputGroupInput
            type="search"
            value={term}
            onChange={(e) => setTerm(e.target.value)}
            placeholder={t("office.bank.matching.lines_search")}
            aria-label={t("office.bank.matching.lines_search")}
          />
        </InputGroup>
        <Label className="flex items-center gap-2 text-sm font-normal">
          <Checkbox
            checked={cari}
            onCheckedChange={(on) => setCari(on === true)}
          />
          {t("office.bank.matching.lines_cari_only")}
        </Label>
      </div>
      {!ready ? (
        <p className="text-muted-foreground text-xs">
          {t("office.bank.matching.lines_hint")}
        </p>
      ) : lines.isLoading ? (
        <Skeleton className="h-16 w-full" />
      ) : items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {q.length === 0
            ? t("office.bank.matching.lines_none")
            : t("office.bank.matching.lines_empty")}
        </p>
      ) : (
        <LineList
          items={items}
          currency={currency}
          picked={picked}
          onToggle={onToggle}
          journalHref={journalHref}
        />
      )}
    </section>
  );
}

function LineList({
  items,
  currency,
  picked,
  onToggle,
  journalHref,
}: {
  items: BankMatchJournalLineCandidate[];
  currency: string;
  picked: string[];
  onToggle: (id: string, on: boolean) => void;
  journalHref: (id: string) => string;
}) {
  const { t } = useLocale();
  return (
    <ul className="divide-y rounded-md border text-sm">
      {items.map((l) => {
        const id = `${l.journal_draft_uuid}:${l.line_no}`;
        return (
          <li key={id} className="flex flex-wrap items-center gap-3 px-3 py-2">
            <Checkbox
              checked={picked.includes(id)}
              onCheckedChange={(on) => onToggle(id, on === true)}
              aria-label={t("office.bank.matching.pick_line", {
                line: l.label,
              })}
            />
            <Link
              className="font-medium underline-offset-4 hover:underline"
              href={journalHref(l.journal_draft_uuid)}
            >
              <Figure>{l.label}</Figure>
            </Link>
            <span className="text-muted-foreground min-w-0 truncate">
              {trDate(l.entry_date)} · {l.description || "—"}
              {l.document_no ? ` · ${l.document_no}` : ""}
            </span>
            {l.cari ? (
              <Badge variant="outline">
                {t("office.bank.matching.lines_cari")}
              </Badge>
            ) : null}
            <span className="ms-auto">
              {t("office.bank.matching.open")}{" "}
              <Money value={l.open_amount} currency={currency} />
            </span>
          </li>
        );
      })}
    </ul>
  );
}

/** Aday ya da arama sonucu faturalar: seçilip eşleştirilir. */
function CandidateList({
  items,
  currency,
  picked,
  onToggle,
  invoiceHref,
}: {
  items: BankMatchCandidate[];
  currency: string;
  picked: string[];
  onToggle?: (id: string, on: boolean) => void;
  invoiceHref: (id: string) => string;
}) {
  const { t } = useLocale();
  return (
    <ul className="divide-y rounded-md border text-sm">
      {items.map((c) => (
        <li
          key={c.invoice_uuid}
          className="flex flex-wrap items-center gap-3 px-3 py-2"
        >
          {onToggle ? (
            <Checkbox
              checked={picked.includes(c.invoice_uuid)}
              onCheckedChange={(on) => onToggle(c.invoice_uuid, on === true)}
              aria-label={t("office.bank.matching.pick", {
                invoice: c.invoice_number,
              })}
            />
          ) : null}
          <Link
            className="font-medium underline-offset-4 hover:underline"
            href={invoiceHref(c.invoice_uuid)}
          >
            <Figure>{c.invoice_number}</Figure>
          </Link>
          <span className="text-muted-foreground">
            {trDate(c.issue_date)} · {c.counterparty_name ?? "—"}
          </span>
          <span className="ms-auto">
            {t("office.bank.matching.open")}{" "}
            <Money value={c.open_amount} currency={currency} />
          </span>
          {c.score > 0 ? (
            <Badge variant={c.score >= 80 ? "default" : "outline"}>
              {t("office.bank.matching.score", { score: String(c.score) })}
            </Badge>
          ) : null}
          <MatchReason parts={c.parts} />
        </li>
      ))}
    </ul>
  );
}

/** Score parts in the order they weigh (21 §6.2). */
const REASONS = [
  "tax_id",
  "iban",
  "invoice_number",
  "amount_exact",
  "amount_less_fee",
  "name",
  "date",
] as const;
const RELATED = ["tax_id", "iban", "invoice_number", "name"];

/**
 * "Neden önerildi" (#710): eşleştiricinin puan parçaları Türkçe cümle
 * olarak görünür. Yalnız tutarı tutan aday ayrıca uyarılır. Elle
 * eşleşmede parça yoktur, satır çizilmez.
 */
function MatchReason({ parts }: { parts: Record<string, number> }) {
  const { t } = useLocale();
  const keys = REASONS.filter((k) => k in parts);
  if (keys.length === 0) return null;
  const related = keys.some((k) => RELATED.includes(k));
  return (
    <p className="text-text-2 w-full text-xs" data-testid="bank-match-reason">
      {t("office.bank.matching.why", {
        reasons: keys
          .map((k) => t(`office.bank.matching.reason_${k}`))
          .join(", "),
      })}
      {related ? null : (
        <span className="text-warn ms-1">
          {t("office.bank.matching.why_amount_only")}
        </span>
      )}
    </p>
  );
}

/**
 * Bağlanan bordronun personel dökümü (#710): açılınca kişi başı net,
 * bordro net toplamı, bu ödemenin bağlanan tutarı ve fark. Kişi adı bordro
 * sayfasında o dönemi (personel satırlarıyla) açar.
 */
function PayrollBreakdown({
  engagement,
  run,
  paid,
  currency,
  href,
}: {
  engagement: string;
  run: string;
  paid: string;
  currency: string;
  href: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [open, setOpen] = useState(false);
  const q = useQuery({
    queryKey: [...payrollKeys.all(slug, engagement), "run", run],
    queryFn: () => payrollService.run(slug, engagement, run),
    enabled: open,
  });
  const lines = q.data?.lines ?? [];
  const net = q.data?.totals.net_total ?? "0";
  const diff = (Number(paid) - Number(net)).toFixed(2);
  return (
    <Collapsible
      className="w-full"
      data-testid="bank-payroll-breakdown"
      open={open}
      onOpenChange={setOpen}
    >
      <CollapsibleTrigger className="text-brand-text cursor-pointer text-xs font-medium underline-offset-4 hover:underline">
        {t("office.bank.matching.payroll_breakdown")}
      </CollapsibleTrigger>
      <CollapsibleContent>
        {q.isLoading ? (
          <Skeleton className="mt-2 h-16 w-full" />
        ) : q.isError ? (
          <p className="text-danger mt-2 text-xs" role="alert">
            {t("office.bank.matching.payroll_breakdown_error")}
          </p>
        ) : q.data ? (
          <Table className="mt-2 text-xs">
            <TableHeader>
              <TableRow>
                <TableHead>
                  {t("office.bank.matching.payroll_col_employee")}
                </TableHead>
                <TableHead className="text-end">
                  {t("office.bank.matching.payroll_col_net")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {lines.map((l, i) => (
                <TableRow key={`${l.employee_name}-${i}`}>
                  <TableCell>
                    <Link
                      className="underline-offset-4 hover:underline"
                      href={href}
                    >
                      {l.employee_name}
                    </Link>
                  </TableCell>
                  <TableCell className="text-end">
                    <Money value={l.net} currency={currency} />
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
            <TableFooter>
              <TableRow>
                <TableCell>
                  {t("office.bank.matching.payroll_total_net", {
                    count: lines.length,
                  })}
                </TableCell>
                <TableCell className="text-end">
                  <Money value={net} currency={currency} />
                </TableCell>
              </TableRow>
              <TableRow>
                <TableCell>{t("office.bank.matching.payroll_paid")}</TableCell>
                <TableCell className="text-end">
                  <Money value={paid} currency={currency} />
                </TableCell>
              </TableRow>
              <TableRow data-testid="bank-payroll-diff">
                <TableCell>
                  {Number(diff) === 0
                    ? t("office.bank.matching.payroll_diff_none")
                    : t("office.bank.matching.payroll_diff")}
                </TableCell>
                <TableCell className="text-end">
                  {Number(diff) === 0 ? (
                    "—"
                  ) : (
                    <Money value={diff} currency={currency} />
                  )}
                </TableCell>
              </TableRow>
            </TableFooter>
          </Table>
        ) : null}
      </CollapsibleContent>
    </Collapsible>
  );
}
