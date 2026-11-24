"use client";

import { Banknote, CreditCard, Landmark, RefreshCw } from "lucide-react";
import { useMemo, useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { BosDurum } from "@/components/hakan";
import {
  DataTable,
  StatusPill,
  createColumn,
  createDateColumn,
  createMoneyColumn,
  createStatusColumn,
  formatCellDate,
  type StatusVariant,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Permission } from "@/config/permissions";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  EASYTRADE_ID_RE,
  periodRange,
  useEasytrade,
  type EasytradeConnection,
  type EasytradeFinancialTransaction,
  type EasytradeItemStats,
  type EasytradeSyncRun,
  type EasytradeZReport,
} from "../hooks/use-easytrade";
import { useOfficePeriod, usePeriodText } from "../period";
import { Money } from "./office-ui";
import { useRelatedColumn } from "./related-column";

/** Senkron aralığı seçenekleri (dakika); "off" yalnızca elle senkron. */
const INTERVALS = ["off", "60", "240", "720", "1440"] as const;

const runVariants: Record<string, StatusVariant> = {
  success: { labelKey: "sources.easytrade.run_success", tone: "ok" },
  partial: { labelKey: "sources.easytrade.run_partial", tone: "warn" },
  failed: { labelKey: "sources.easytrade.run_failed", tone: "danger" },
  running: { labelKey: "sources.easytrade.run_running", tone: "info" },
};

const connTone: Record<string, "ok" | "warn" | "danger" | "neutral"> = {
  active: "ok",
  rate_limited: "warn",
  auth_failed: "danger",
  disabled: "danger",
  pending: "neutral",
};

const KNOWN_TYPES = new Set([
  "income",
  "expense",
  "retail_sale",
  "opening_balance",
]);

const payIcon: Record<string, typeof Banknote> = {
  cash: Banknote,
  credit_card: CreditCard,
  bank_transfer: Landmark,
};

function errText(err: unknown, fallback: string) {
  return isApiError(err) ? err.message : fallback;
}

/**
 * `/t/{slug}/engagements/{uuid}/easytrade` (#385): Easytrade bağlantısı,
 * zamanlanmış senkron (son / sıradaki senkron, aralık, hata ve yeniden
 * deneme), dönemin finansal işlemleri (ödeme türü, kişi/cari) ve Z
 * raporları. Dönem ofisin global seçicisinden.
 */
export function EngagementEasytradePage({
  engagement,
}: {
  engagement: string;
}) {
  const { t } = useLocale();
  const { focus } = useOfficePeriod();
  const et = useEasytrade(engagement, focus);

  return (
    <div className="space-y-5">
      <div className="min-w-0">
        <h2 className="font-display truncate text-xl font-bold">
          {t("sources.easytrade.title")}
        </h2>
        <p className="text-text-2 text-sm">
          {t("sources.easytrade.description")}
        </p>
      </div>

      {et.connections.isLoading ? (
        <div className="space-y-2">
          <Skeleton className="h-28 w-full" />
          <Skeleton className="h-40 w-full" />
        </div>
      ) : et.connections.isError ? (
        <ErrorState
          title={t("office.api.load_error")}
          description={et.connections.error.message}
          onRetry={() => void et.connections.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : !et.conn ? (
        <ConnectCard et={et} />
      ) : (
        <>
          <ConnectionCard conn={et.conn} et={et} period={focus} />
          <PaymentSummary et={et} period={focus} />
          <FinancialSection et={et} period={focus} />
          <ZSection et={et} period={focus} />
          <RunsSection runs={et.runs.data?.items} loading={et.runs.isLoading} />
        </>
      )}
    </div>
  );
}

type ET = ReturnType<typeof useEasytrade>;

/**
 * Bağlantı testinin ya da bağlanmanın sonucu: Türkçe neden + teknik kod;
 * Easytrade girişi reddettiyse hangi sunucu ve hangi HTTP durumu.
 */
type Outcome =
  | { ok: true; text: string }
  | {
      ok: false;
      text: string;
      code?: string;
      server?: string;
      field?: { name: string; text: string };
    };

type EtField = "email" | "password" | "company_id";

/** Easytrade'in hata ayrıntıları (#830): sunucu, durum ve hatalı alan. */
function easytradeDetails(err: unknown) {
  if (!isApiError(err)) return {};
  const by = (code: string) => err.details.find((d) => d.code === code);
  const host = by("EASYTRADE_HOST")?.message;
  const status = by("EASYTRADE_STATUS")?.message;
  const field = by("EASYTRADE_FIELD");
  return {
    host,
    status,
    field:
      field?.field && field.message
        ? { name: field.field, text: field.message }
        : undefined,
  };
}

function ConnectCard({ et }: { et: ET }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const [form, setForm] = useState({ email: "", password: "", company_id: "" });
  const [outcome, setOutcome] = useState<Outcome | null>(null);
  const idTrim = form.company_id.trim();
  const idInvalid = idTrim !== "" && !EASYTRADE_ID_RE.test(idTrim);
  // E-posta kırpılır ve küçük harfe çevrilir; şifre olduğu gibi gider.
  const body = {
    email: form.email.trim().toLowerCase(),
    password: form.password,
    company_id: idTrim,
  };
  const ready = body.email !== "" && form.password !== "";
  const fail = (err: unknown, fallback: string): Outcome => {
    const d = easytradeDetails(err);
    const text = errText(err, fallback);
    const server = d.host
      ? d.status
        ? t("sources.easytrade.server_status", {
            host: d.host,
            status: d.status,
          })
        : t("sources.easytrade.server_only", { host: d.host })
      : undefined;
    return {
      ok: false,
      text,
      code: isApiError(err) ? err.code : undefined,
      // Türkçe metin sunucuyu zaten söylüyorsa yinelenmez.
      server: server && d.host && !text.includes(d.host) ? server : undefined,
      field: d.field,
    };
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (idInvalid) return;
    setOutcome(null);
    et.connect.mutate(body, {
      onSuccess: (c) => {
        toast.success(t("sources.easytrade.connected"));
        // Girilen ID kullanıcının firması değilse girişteki firma kullanıldı.
        if (c.entered_company_id)
          toast.info(
            t("sources.easytrade.company_used", {
              company: c.company_name || "—",
              id: c.company_id,
            }),
          );
      },
      onError: (err) =>
        setOutcome(fail(err, t("sources.easytrade.connect_failed"))),
    });
  };
  const test = () => {
    if (!ready) {
      setOutcome({ ok: false, text: t("sources.easytrade.fill_first") });
      return;
    }
    if (idInvalid) return;
    setOutcome(null);
    et.testConnection.mutate(body, {
      onSuccess: (r) =>
        setOutcome({
          ok: true,
          text: r.entered_company_id
            ? `${t("sources.easytrade.company_used", {
                company: r.company_name || "—",
                id: r.company_id,
              })} ${t("sources.easytrade.branches_n", { n: r.branches.length })}`
            : t("sources.easytrade.test_ok", {
                company: r.company_name || "—",
                id: r.company_id,
                branches: r.branches.length,
              }),
        }),
      onError: (err) =>
        setOutcome(fail(err, t("sources.easytrade.test_failed"))),
    });
  };
  // Easytrade'in reddettiği alan (422) ya da 401'de e-posta/şifre ikilisi.
  const failedField =
    outcome && !outcome.ok && outcome.field ? outcome.field.name : undefined;
  const credsRefused =
    outcome && !outcome.ok && outcome.code === "EASYTRADE_BAD_CREDENTIALS";
  const fieldInvalid = (f: EtField) =>
    (f === "company_id" && idInvalid) ||
    failedField === f ||
    (credsRefused && f !== "company_id");
  const hintOf = (f: EtField) => {
    if (f === "company_id")
      return idInvalid
        ? t("sources.easytrade.f_company_id_invalid")
        : t("sources.easytrade.f_company_id_hint");
    if (failedField === f && outcome && !outcome.ok && outcome.field)
      return outcome.field.text;
    return f === "email" ? t("sources.easytrade.f_email_hint") : null;
  };
  return (
    <section className="space-y-4">
      <BosDurum
        baslik={t("sources.easytrade.empty_title")}
        aciklama={t("sources.easytrade.empty")}
      />
      {can(Permission.TenantInvoicesWrite) ? (
        <form
          onSubmit={submit}
          className="bg-surface border-line grid gap-3 rounded-xl border p-4 sm:grid-cols-3 sm:items-start"
        >
          {(["email", "password", "company_id"] as const).map((f) => {
            const hint = hintOf(f);
            const invalid = fieldInvalid(f);
            // Kendi adları ve new-password/off: tarayıcı kullanıcının
            // Hakan'a kayıtlı e-posta ve şifresini doldurmasın.
            return (
              <div key={f} className="space-y-1.5">
                <Label htmlFor={`et-${f}`}>
                  {t(`sources.easytrade.f_${f}`)}
                </Label>
                <Input
                  id={`et-${f}`}
                  name={`easytrade_${f}`}
                  type={
                    f === "password"
                      ? "password"
                      : f === "email"
                        ? "email"
                        : "text"
                  }
                  inputMode={f === "company_id" ? "numeric" : undefined}
                  autoComplete={f === "password" ? "new-password" : "off"}
                  // Parola yöneticileri ID alanına başka bir değer yazmasın.
                  data-1p-ignore={f === "company_id" ? true : undefined}
                  data-lpignore={f === "company_id" ? "true" : undefined}
                  placeholder={
                    f === "company_id"
                      ? t("sources.easytrade.f_company_id_placeholder")
                      : undefined
                  }
                  autoCapitalize="off"
                  autoCorrect="off"
                  spellCheck={false}
                  required={f !== "company_id"}
                  aria-invalid={invalid ? true : undefined}
                  aria-describedby={hint ? `et-${f}-hint` : undefined}
                  value={form[f]}
                  onChange={(e) => {
                    setForm({ ...form, [f]: e.target.value });
                    setOutcome(null);
                  }}
                />
                {hint ? (
                  <p
                    id={`et-${f}-hint`}
                    data-testid={`et-${f}-hint`}
                    className={cn(
                      "text-xs",
                      invalid ? "text-danger" : "text-text-3",
                    )}
                  >
                    {hint}
                  </p>
                ) : null}
              </div>
            );
          })}
          <div className="flex flex-wrap gap-2 sm:col-span-3">
            <Button
              type="button"
              variant="outline"
              disabled={et.testConnection.isPending || idInvalid}
              onClick={test}
            >
              {et.testConnection.isPending
                ? t("sources.easytrade.testing")
                : t("sources.easytrade.test")}
            </Button>
            <Button type="submit" disabled={et.connect.isPending || idInvalid}>
              {t("sources.easytrade.connect")}
            </Button>
          </div>
          {outcome ? (
            <div
              role={outcome.ok ? "status" : "alert"}
              data-testid="et-outcome"
              className={cn(
                "rounded-lg px-3 py-2 text-sm sm:col-span-3",
                outcome.ok
                  ? "bg-ok-tint text-ok"
                  : "bg-danger-tint text-danger",
              )}
            >
              <p className="font-medium">{outcome.text}</p>
              {!outcome.ok && outcome.server ? (
                <p className="text-xs">{outcome.server}</p>
              ) : null}
              {!outcome.ok && outcome.code ? (
                <p className="text-xs opacity-80">
                  {t("sources.easytrade.error_code", { code: outcome.code })}
                </p>
              ) : null}
            </div>
          ) : null}
          <p className="text-text-3 text-xs sm:col-span-3">
            {t("sources.easytrade.connect_hint")}
          </p>
        </form>
      ) : null}
    </section>
  );
}

function ConnectionCard({
  conn,
  et,
  period,
}: {
  conn: EasytradeConnection;
  et: ET;
  period: string;
}) {
  const { t, locale } = useLocale();
  const periodText = usePeriodText();
  const { can } = usePermission();
  const canWrite = can(Permission.TenantInvoicesWrite);
  const dt = (v?: string | null) =>
    v ? formatCellDate(v, locale, "datetime") : t("sources.easytrade.never");
  const failures = conn.sync_failures ?? 0;
  const interval =
    conn.sync_interval_minutes == null
      ? "off"
      : String(conn.sync_interval_minutes);
  const facts: { label: string; value: string; tone?: string }[] = [
    { label: t("sources.easytrade.last_sync"), value: dt(conn.last_sync_at) },
    {
      label: t("sources.easytrade.last_success"),
      value: dt(conn.last_success_at),
    },
    {
      label: t("sources.easytrade.next_sync"),
      value:
        conn.sync_interval_minutes == null
          ? t("sources.easytrade.manual_only")
          : dt(conn.next_sync_at),
    },
    {
      label: t("sources.easytrade.failures"),
      value:
        failures > 0
          ? t("sources.easytrade.failures_n", { n: failures })
          : t("sources.easytrade.failures_none"),
      tone: failures > 0 ? "text-warn" : undefined,
    },
  ];
  return (
    <section
      aria-labelledby="et-conn"
      className="bg-surface border-line space-y-4 rounded-xl border p-4"
    >
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <h2 id="et-conn" className="font-display text-lg font-semibold">
            {t("sources.easytrade.connection")}
          </h2>
          <StatusPill tone={connTone[conn.status] ?? "neutral"} dot>
            {t(`sources.easytrade.status_${conn.status}`)}
          </StatusPill>
          <span className="text-text-3 truncate text-sm">
            {conn.company_name
              ? t("sources.easytrade.company_named", {
                  company: conn.company_name,
                  id: conn.company_id,
                  branches: conn.branches.length,
                })
              : t("sources.easytrade.company", {
                  id: conn.company_id,
                  branches: conn.branches.length,
                })}
          </span>
        </div>
        {canWrite ? (
          <div className="flex flex-wrap items-center gap-2">
            <Select
              value={interval}
              onValueChange={(v) =>
                et.schedule.mutate(v === "off" ? null : Number(v), {
                  onSuccess: () =>
                    toast.success(t("sources.easytrade.schedule_saved")),
                  onError: (err) =>
                    toast.error(
                      errText(err, t("sources.easytrade.schedule_failed")),
                    ),
                })
              }
            >
              <SelectTrigger
                className="w-44"
                aria-label={t("sources.easytrade.interval")}
              >
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {INTERVALS.map((v) => (
                  <SelectItem key={v} value={v}>
                    {t(`sources.easytrade.interval_${v}`)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
            <Button
              size="lg"
              disabled={et.syncNow.isPending || conn.status === "disabled"}
              onClick={() =>
                et.syncNow.mutate(periodRange(period), {
                  onSuccess: () => toast.success(t("sources.easytrade.synced")),
                  onError: (err) =>
                    toast.error(
                      errText(err, t("sources.easytrade.sync_failed")),
                    ),
                })
              }
            >
              <RefreshCw
                aria-hidden
                className={cn("size-4", et.syncNow.isPending && "animate-spin")}
              />
              {et.syncNow.isPending
                ? t("sources.easytrade.syncing")
                : t("sources.easytrade.sync_now")}
            </Button>
          </div>
        ) : null}
      </div>
      {canWrite ? (
        <p className="text-text-3 text-xs">
          {t("sources.easytrade.sync_now_hint", {
            period: periodText(period),
          })}
        </p>
      ) : null}
      {conn.error ? (
        <p
          role="alert"
          className="bg-danger-tint text-danger rounded-lg px-3 py-2 text-sm"
        >
          {conn.error}
        </p>
      ) : null}
      <dl className="grid gap-3 sm:grid-cols-2 lg:grid-cols-4">
        {facts.map((f) => (
          <div key={f.label} className="min-w-0">
            <dt className="text-text-3 text-xs">{f.label}</dt>
            <dd
              className={cn(
                "truncate text-sm font-medium tabular-nums",
                f.tone,
              )}
            >
              {f.value}
            </dd>
          </div>
        ))}
      </dl>
      <LastRunResult run={conn.last_run} />
      <p className="text-text-3 text-xs">
        {conn.last_webhook_at
          ? t("sources.easytrade.webhook_last", {
              at: dt(conn.last_webhook_at),
            })
          : t("sources.easytrade.webhook_none")}
      </p>
    </section>
  );
}

function PaymentSummary({ et, period }: { et: ET; period: string }) {
  const { t } = useLocale();
  const periodText = usePeriodText();
  const sum = et.financial.data;
  const cards = useMemo(() => {
    const by: Record<string, { in: number; out: number; count: number }> = {};
    for (const p of sum?.by_payment_type ?? []) {
      const c = (by[p.payment_method_type] ??= { in: 0, out: 0, count: 0 });
      if (p.direction === "out") c.out += Number(p.total);
      else c.in += Number(p.total);
      c.count += p.count;
    }
    return ["cash", "credit_card", "bank_transfer"].map((k) => ({
      key: k,
      ...(by[k] ?? { in: 0, out: 0, count: 0 }),
    }));
  }, [sum]);
  return (
    <section aria-labelledby="et-pay" className="space-y-2">
      <h2 id="et-pay" className="font-display text-lg font-semibold">
        {t("sources.easytrade.payments_title", { period: periodText(period) })}
      </h2>
      <div className="grid gap-3 sm:grid-cols-3">
        {cards.map((c) => {
          const Icon = payIcon[c.key];
          return (
            <div
              key={c.key}
              className="bg-surface border-line flex items-center gap-3.5 rounded-xl border p-4"
            >
              <span
                aria-hidden
                className="bg-brand-tint text-brand-text flex size-10 shrink-0 items-center justify-center rounded-lg"
              >
                <Icon className="size-5" />
              </span>
              <span className="flex min-w-0 flex-col">
                <span className="text-text-2 text-sm">
                  {t(`sources.payment.${c.key}`)}
                </span>
                {et.financial.isLoading ? (
                  <Skeleton className="h-6 w-24" />
                ) : (
                  <span className="font-display text-xl font-bold tabular-nums">
                    <Money value={c.in.toFixed(2)} />
                  </span>
                )}
                <span className="text-text-3 text-xs">
                  {t("sources.easytrade.payments_out", {
                    count: c.count,
                  })}{" "}
                  <Money value={c.out.toFixed(2)} />
                </span>
              </span>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function FinancialSection({ et, period }: { et: ET; period: string }) {
  const { t } = useLocale();
  const periodText = usePeriodText();
  const baseColumns = useMemo(
    () => [
      createDateColumn<EasytradeFinancialTransaction>({
        id: "transaction_date",
        labelKey: "sources.col_date",
        accessor: (r) => r.transaction_date,
      }),
      createColumn<EasytradeFinancialTransaction>({
        id: "customer_name",
        labelKey: "sources.col_customer",
        accessorFn: (r) => r.customer_name ?? "",
        cell: ({ row }) => (
          <div className="min-w-40">
            <div className="truncate">{row.original.customer_name || "—"}</div>
            <div className="text-text-3 truncate text-xs">
              {row.original.description}
            </div>
          </div>
        ),
        gridPrimary: true,
      }),
      createColumn<EasytradeFinancialTransaction>({
        id: "transaction_type",
        labelKey: "sources.col_type",
        accessorFn: (r) => r.transaction_type,
        cell: ({ row }) =>
          KNOWN_TYPES.has(row.original.transaction_type)
            ? t(`sources.type.${row.original.transaction_type}`)
            : row.original.transaction_type || "—",
        filterVariant: "faceted",
      }),
      createColumn<EasytradeFinancialTransaction>({
        id: "payment_method_type",
        labelKey: "sources.col_payment",
        accessorFn: (r) => r.payment_method_type,
        cell: ({ row }) =>
          t(`sources.payment.${row.original.payment_method_type}`),
        filterVariant: "faceted",
        filterOptions: [
          "cash",
          "bank_transfer",
          "credit_card",
          "check",
          "promissory_note",
          "credit",
          "unknown",
        ].map((v) => ({
          label: v,
          labelKey: `sources.payment.${v}`,
          value: v,
        })),
      }),
      createColumn<EasytradeFinancialTransaction>({
        id: "cash_box_name",
        labelKey: "sources.col_cash_box",
        accessorFn: (r) => r.cash_box_name ?? "",
        defaultHidden: true,
      }),
      createMoneyColumn<EasytradeFinancialTransaction>({
        id: "amount",
        labelKey: "sources.col_amount",
        accessor: (r) => (r.direction === "out" ? `-${r.amount}` : r.amount),
        currency: (r) => r.currency,
      }),
      createStatusColumn<EasytradeFinancialTransaction>({
        id: "invoice",
        labelKey: "sources.col_invoice",
        accessor: (r) =>
          r.referenceable_type !== "Invoice"
            ? "none"
            : r.invoice_uuid
              ? "matched"
              : "unmatched",
        variants: {
          none: { labelKey: "sources.easytrade.ref_none", tone: "neutral" },
          matched: { labelKey: "sources.easytrade.ref_matched", tone: "ok" },
          unmatched: {
            labelKey: "sources.easytrade.ref_unmatched",
            tone: "warn",
          },
        },
      }),
    ],
    [t],
  );
  // Hareketin ilişkili kayıtları: bağlı fatura ve uyarılar (#469).
  const relatedColumn = useRelatedColumn<EasytradeFinancialTransaction>(
    "financial_transaction",
    (x) => x.uuid,
    (x) => x.description,
  );
  const columns = useMemo(
    () => [...baseColumns, ...relatedColumn],
    [baseColumns, relatedColumn],
  );
  const data = et.financial.data;
  return (
    <section aria-labelledby="et-fin" className="space-y-2">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 id="et-fin" className="font-display text-lg font-semibold">
          {t("sources.easytrade.financial_title", {
            period: periodText(period),
          })}
        </h2>
        {data && data.invoice_refs > 0 ? (
          <span className="text-text-2 text-sm">
            {t("sources.easytrade.invoice_matched", {
              matched: data.invoice_matched,
              refs: data.invoice_refs,
            })}
          </span>
        ) : null}
      </div>
      {et.financial.isError ? (
        <ErrorState
          title={t("office.api.load_error")}
          description={et.financial.error.message}
          onRetry={() => void et.financial.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : (
        <DataTable
          columns={columns}
          data={data?.items ?? []}
          getRowId={(r) => r.uuid}
          isLoading={et.financial.isLoading}
          emptyTitle={t("sources.easytrade.financial_empty_title")}
          emptyDescription={t("sources.easytrade.financial_empty")}
          initialState={{ pagination: { pageIndex: 0, pageSize: 20 } }}
          features={{ persistKey: "easytrade-financial" }}
        />
      )}
      {data && data.by_customer.length > 0 ? (
        <CustomerTotals rows={data.by_customer} />
      ) : null}
    </section>
  );
}

function CustomerTotals({
  rows,
}: {
  rows: NonNullable<ET["financial"]["data"]>["by_customer"];
}) {
  const columns = useMemo(
    () => [
      createColumn<(typeof rows)[number]>({
        id: "customer_name",
        labelKey: "sources.col_customer",
        accessorFn: (r) => r.customer_name || "—",
        gridPrimary: true,
      }),
      createColumn<(typeof rows)[number]>({
        id: "customer_tax_number",
        labelKey: "sources.col_tax_number",
        accessorFn: (r) => r.customer_tax_number ?? "",
        cell: ({ row }) => (
          <span className="font-mono text-sm">
            {row.original.customer_tax_number || "—"}
          </span>
        ),
      }),
      createColumn<(typeof rows)[number]>({
        id: "count",
        labelKey: "sources.col_count",
        accessorFn: (r) => r.count,
        meta: { align: "end" },
      }),
      createMoneyColumn<(typeof rows)[number]>({
        id: "total_in",
        labelKey: "sources.col_in",
        accessor: (r) => r.total_in,
      }),
      createMoneyColumn<(typeof rows)[number]>({
        id: "total_out",
        labelKey: "sources.col_out",
        accessor: (r) => r.total_out,
      }),
    ],
    [],
  );
  const { t } = useLocale();
  return (
    <div className="space-y-2 pt-2">
      <h3 className="text-sm font-semibold">
        {t("sources.easytrade.by_customer")}
      </h3>
      <DataTable
        columns={columns}
        data={rows}
        getRowId={(r) => `${r.customer_name}|${r.customer_tax_number ?? ""}`}
        features={{ toolbar: false, pagination: false }}
      />
    </div>
  );
}

function ZSection({ et, period }: { et: ET; period: string }) {
  const { t } = useLocale();
  const periodText = usePeriodText();
  const z = et.zReports.data;
  const columns = useMemo(
    () => [
      createDateColumn<EasytradeZReport>({
        id: "report_date",
        labelKey: "sources.col_date",
        accessor: (r) => r.report_date,
      }),
      createColumn<EasytradeZReport>({
        id: "z_report_number",
        labelKey: "sources.col_z_number",
        accessorFn: (r) => r.z_report_number ?? "",
        cell: ({ row }) => (
          <span className="font-mono text-sm">
            {row.original.z_report_number || "—"}
          </span>
        ),
        gridPrimary: true,
      }),
      createColumn<EasytradeZReport>({
        id: "okc_serial_number",
        labelKey: "sources.col_okc",
        accessorFn: (r) => r.okc_serial_number ?? "",
        defaultHidden: true,
      }),
      createMoneyColumn<EasytradeZReport>({
        id: "cash",
        labelKey: "sources.payment.cash",
        accessor: (r) => r.payments.cash ?? "0.00",
      }),
      createMoneyColumn<EasytradeZReport>({
        id: "credit_card",
        labelKey: "sources.payment.credit_card",
        accessor: (r) => r.payments.credit_card ?? "0.00",
      }),
      createMoneyColumn<EasytradeZReport>({
        id: "tax_amount",
        labelKey: "sources.col_vat",
        accessor: (r) => r.tax_amount,
      }),
      createMoneyColumn<EasytradeZReport>({
        id: "total_amount",
        labelKey: "sources.col_total",
        accessor: (r) => r.total_amount,
        className: "font-semibold",
      }),
    ],
    [],
  );
  return (
    <section aria-labelledby="et-z" className="space-y-2">
      <div className="flex flex-wrap items-baseline justify-between gap-2">
        <h2 id="et-z" className="font-display text-lg font-semibold">
          {t("sources.easytrade.z_title", { period: periodText(period) })}
        </h2>
        {z && z.items.length > 0 ? (
          <span className="text-text-2 text-sm">
            {t("sources.easytrade.z_total")} <Money value={z.total_amount} />
          </span>
        ) : null}
      </div>
      {z && !z.supported ? (
        <p className="bg-warn-tint text-warn rounded-lg px-3 py-2 text-sm">
          {t("sources.easytrade.z_unsupported")}
        </p>
      ) : null}
      <DataTable
        columns={columns}
        data={z?.items ?? []}
        getRowId={(r) => r.uuid}
        isLoading={et.zReports.isLoading}
        emptyTitle={t("sources.easytrade.z_empty_title")}
        emptyDescription={t("sources.easytrade.z_empty")}
        features={{ pagination: false }}
      />
    </section>
  );
}

/** Kalemler, ekranda gösterilme sırasıyla (backend `Items`). */
const RUN_ITEMS = [
  "invoices",
  "expenses",
  "financial_transactions",
  "z_reports",
] as const;

/** Türkçe karşılığı olan hariç tutma nedenleri; diğerleri "başka neden". */
const KNOWN_REASONS = new Set([
  "cancelled",
  "draft",
  "rejected",
  "recurring_template",
  "no_issue_date",
  "no_expense_date",
  "no_transaction_date",
  "no_report_date",
]);

type ItemStats = EasytradeItemStats;
type ItemRow = { item: (typeof RUN_ITEMS)[number]; stats?: ItemStats };

/**
 * Son senkronun kalem kalem sonucu (#780): Easytrade listesinde taranan,
 * döneme düşüp okunan, yazılan (yeni + güncellenen), zaten var olan ve
 * hariç tutulan kayıtlar, hariç tutma nedenleriyle.
 */
function LastRunResult({ run }: { run?: EasytradeSyncRun }) {
  const { t, locale } = useLocale();
  const counters = run?.counters as
    (EasytradeSyncRun["counters"] & { unsupported?: string[] }) | undefined;
  const items = counters?.items;
  const rows: ItemRow[] = RUN_ITEMS.map((item) => ({
    item,
    stats: items?.[item],
  }));
  const columns = useMemo(() => {
    const unsupported = new Set(
      (run?.counters as { unsupported?: string[] } | undefined)?.unsupported ??
        [],
    );
    const reasons = (s?: ItemStats) =>
      Object.entries(s?.excluded ?? {})
        .filter(([, n]) => n > 0)
        .map(([k, n]) =>
          t("sources.easytrade.reason_n", {
            n,
            reason: KNOWN_REASONS.has(k)
              ? t(`sources.easytrade.reason_${k}`)
              : t("sources.easytrade.reason_other"),
          }),
        )
        .join(" · ");
    return [
      createColumn<ItemRow>({
        id: "item",
        labelKey: "sources.easytrade.res_item",
        accessorFn: (r) => r.item,
        cell: ({ row }) => (
          <span className="font-medium">
            {t(`sources.easytrade.item_${row.original.item}`)}
          </span>
        ),
      }),
      ...(["listed", "read", "written", "unchanged", "excluded"] as const).map(
        (k) =>
          createColumn<ItemRow>({
            id: k,
            labelKey: `sources.easytrade.res_${k}`,
            accessorFn: (r) => {
              const s = r.stats;
              if (!s) return 0;
              if (k === "written") return s.created + s.updated;
              if (k === "excluded")
                return Object.values(s.excluded ?? {}).reduce(
                  (a, b) => a + b,
                  0,
                );
              return s[k];
            },
            cell: ({ getValue, row }) =>
              row.original.stats ? (
                <span
                  className={cn(
                    "tabular-nums",
                    k === "written" && Number(getValue()) > 0 && "text-ok",
                    k === "excluded" && Number(getValue()) > 0 && "text-warn",
                  )}
                >
                  {Number(getValue()).toLocaleString(locale)}
                </span>
              ) : (
                <span className="text-text-3">—</span>
              ),
          }),
      ),
      createColumn<ItemRow>({
        id: "reasons",
        labelKey: "sources.easytrade.res_reasons",
        accessorFn: (r) => reasons(r.stats),
        enableSorting: false,
        cell: ({ row }) => {
          const s = row.original.stats;
          if (unsupported.has(row.original.item))
            return (
              <span className="text-text-3 text-xs">
                {t("sources.easytrade.res_unsupported")}
              </span>
            );
          const text = reasons(s);
          return (
            <span className="text-text-2 text-xs">
              {text}
              {s && s.windows_failed > 0 ? (
                <span className="text-danger block">
                  {t("sources.easytrade.windows_failed", {
                    n: s.windows_failed,
                  })}
                </span>
              ) : null}
            </span>
          );
        },
      }),
    ];
  }, [t, locale, run]);
  if (!run) return null;
  const ranges = (counters?.ranges ?? [])
    .map((r) =>
      t("sources.easytrade.res_range", {
        from: formatCellDate(r.from, locale),
        to: formatCellDate(r.to, locale),
      }),
    )
    .join(", ");
  return (
    <div
      className="border-line space-y-2 border-t pt-3"
      data-testid="et-last-run"
    >
      <div className="flex flex-wrap items-center gap-2">
        <h3 className="text-sm font-semibold">
          {t("sources.easytrade.res_title")}
        </h3>
        <StatusPill tone={runVariants[run.status]?.tone ?? "neutral"} dot>
          {t(
            runVariants[run.status]?.labelKey ??
              "sources.easytrade.run_running",
          )}
        </StatusPill>
        <span className="text-text-3 text-xs">
          {formatCellDate(
            run.finished_at ?? run.started_at,
            locale,
            "datetime",
          )}
          {" · "}
          {t(`sources.easytrade.kind_${run.kind}`)}
          {ranges ? ` · ${t("sources.easytrade.res_ranges", { ranges })}` : ""}
        </span>
      </div>
      {run.error ? (
        <p role="alert" className="text-danger text-xs">
          {run.error}
        </p>
      ) : null}
      {items ? (
        <DataTable
          columns={columns}
          data={rows}
          getRowId={(r) => r.item}
          features={{ pagination: false, toolbar: false }}
        />
      ) : (
        <p className="text-text-3 text-xs">
          {t("sources.easytrade.res_legacy")}
        </p>
      )}
    </div>
  );
}

function RunsSection({
  runs,
  loading,
}: {
  runs?: EasytradeSyncRun[];
  loading: boolean;
}) {
  const { t } = useLocale();
  const columns = useMemo(
    () => [
      createDateColumn<EasytradeSyncRun>({
        id: "started_at",
        labelKey: "sources.col_started",
        accessor: (r) => r.started_at,
        style: "datetime",
      }),
      createColumn<EasytradeSyncRun>({
        id: "kind",
        labelKey: "sources.col_kind",
        accessorFn: (r) => r.kind,
        cell: ({ row }) => t(`sources.easytrade.kind_${row.original.kind}`),
      }),
      createStatusColumn<EasytradeSyncRun>({
        id: "status",
        labelKey: "sources.col_status",
        accessor: (r) => r.status,
        variants: runVariants,
      }),
      createColumn<EasytradeSyncRun>({
        id: "summary",
        labelKey: "sources.col_result",
        accessorFn: (r) => JSON.stringify(r.counters),
        enableSorting: false,
        cell: ({ row }) => {
          const c = row.original.counters as Record<string, unknown>;
          const n = (k: string) => Number(c[k] ?? 0);
          return (
            <div className="min-w-56 text-sm">
              <div>
                {t("sources.easytrade.run_counts", {
                  invoices: n("invoices_created"),
                  expenses: n("expenses_created") + n("expenses_updated"),
                  financial: n("financial_created") + n("financial_updated"),
                  z: n("z_reports_created") + n("z_reports_updated"),
                })}
              </div>
              {row.original.error ? (
                <div className="text-danger truncate text-xs">
                  {row.original.error}
                </div>
              ) : n("windows_failed") > 0 ? (
                <div className="text-warn text-xs">
                  {t("sources.easytrade.windows_failed", {
                    n: n("windows_failed"),
                  })}
                </div>
              ) : null}
            </div>
          );
        },
      }),
    ],
    [t],
  );
  return (
    <section aria-labelledby="et-runs" className="space-y-2">
      <h2 id="et-runs" className="font-display text-lg font-semibold">
        {t("sources.easytrade.runs_title")}
      </h2>
      <DataTable
        columns={columns}
        data={runs ?? []}
        getRowId={(r) => r.uuid}
        isLoading={loading}
        emptyTitle={t("sources.easytrade.runs_empty_title")}
        emptyDescription={t("sources.easytrade.runs_empty")}
        features={{ pagination: false, toolbar: false }}
      />
    </section>
  );
}
