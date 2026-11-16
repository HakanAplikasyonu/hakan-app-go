"use client";

import type { ReactNode } from "react";

import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatAmount, formatMoney } from "../lib/decimal";
import type { DataWarning, Invoice, Severity } from "../types";

/** Tutar, hesap kodu, VKN, ETTN: JetBrains Mono + hizalı rakam (arayüz v1). */
export function Figure({
  children,
  className,
}: {
  children: ReactNode;
  className?: string;
}) {
  return <span className={cn("figure", className)}>{children}</span>;
}

export function Money({
  value,
  currency = "TRY",
  className,
}: {
  value: string;
  currency?: string;
  className?: string;
}) {
  return (
    <Figure className={cn("whitespace-nowrap", className)}>
      {formatMoney(value, currency)}
    </Figure>
  );
}

const severityClass: Record<Severity, string> = {
  info: "bg-brand/10 text-brand",
  warning: "bg-status-warning/12 text-status-warning",
  blocker: "bg-status-blocking/12 text-status-blocking",
};

export function SeverityBadge({ severity }: { severity: Severity }) {
  const { t } = useLocale();
  return (
    <span
      className={cn(
        "inline-flex items-center gap-1.5 rounded-md px-2 py-0.5 text-xs font-medium whitespace-nowrap",
        severityClass[severity],
      )}
    >
      <span className="size-1.5 rounded-full bg-current" aria-hidden />
      {t(`office.severity_${severity}`)}
    </span>
  );
}

const statusClass: Record<Invoice["status"], string> = {
  new: "bg-secondary text-secondary-foreground",
  in_review: "bg-status-review/20 text-amber-800 dark:text-status-review",
  approved: "bg-status-approved/12 text-status-approved",
};

export function InvoiceStatusBadge({ status }: { status: Invoice["status"] }) {
  const { t } = useLocale();
  return (
    <span
      className={cn(
        "inline-flex items-center rounded-md px-2 py-0.5 text-xs font-medium whitespace-nowrap",
        statusClass[status],
      )}
    >
      {t(`office.invoice_status_${status}`)}
    </span>
  );
}

export function WarningStatusLabel({
  status,
}: {
  status: DataWarning["status"];
}) {
  const { t } = useLocale();
  return (
    <span className="text-muted-foreground text-xs">
      {t(`office.warning_status_${status}`)}
    </span>
  );
}

/** Uyarı metni koddan üretilir (paket 05). Bilinmeyen kod ham gösterilir. */
export function useWarningText() {
  const { t } = useLocale();
  return (w: Pick<DataWarning, "code" | "params">) => {
    const key = `office.warning.${w.code}`;
    const params = Object.fromEntries(
      Object.entries(w.params).map(([k, v]) => [
        k,
        /^-?\d+\.\d{1,4}$/.test(v) ? formatAmount(v) : v,
      ]),
    );
    const msg = t(key, params);
    return msg === key ? w.code : msg;
  };
}

export function Panel({
  title,
  description,
  actions,
  children,
  className,
}: {
  title?: ReactNode;
  description?: ReactNode;
  actions?: ReactNode;
  children: ReactNode;
  className?: string;
}) {
  return (
    <section
      className={cn(
        "bg-card text-card-foreground rounded-xl border",
        className,
      )}
    >
      {title || actions ? (
        <header className="flex flex-wrap items-center justify-between gap-2 border-b px-4 py-3">
          <div>
            {title ? <h2 className="text-sm font-semibold">{title}</h2> : null}
            {description ? (
              <p className="text-muted-foreground text-xs">{description}</p>
            ) : null}
          </div>
          {actions}
        </header>
      ) : null}
      {children}
    </section>
  );
}

/** Yatay kayan, sade tablo kabı. */
export function TableBox({ children }: { children: ReactNode }) {
  return (
    <div className="overflow-x-auto">
      <table className="[&_th]:text-muted-foreground w-full text-sm [&_tbody_tr]:border-b [&_tbody_tr:last-child]:border-0 [&_td]:px-4 [&_td]:py-2.5 [&_th]:px-4 [&_th]:py-2 [&_th]:text-left [&_th]:text-xs [&_th]:font-medium [&_thead]:border-b">
        {children}
      </table>
    </div>
  );
}

export function FilterSelect<T extends string>({
  label,
  value,
  onChange,
  options,
}: {
  label: string;
  value: T;
  onChange: (v: T) => void;
  options: { value: T; label: string }[];
}) {
  return (
    <label className="flex items-center gap-2 text-xs">
      <span className="text-muted-foreground">{label}</span>
      <select
        className="bg-background h-8 rounded-md border px-2 text-sm"
        value={value}
        onChange={(e) => onChange(e.target.value as T)}
      >
        {options.map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    </label>
  );
}
