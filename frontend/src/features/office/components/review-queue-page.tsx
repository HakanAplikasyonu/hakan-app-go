"use client";

import { Check, ClipboardCheck, Pencil, Sparkles, X } from "lucide-react";
import Link from "next/link";
import { useSearchParams } from "next/navigation";
import { Suspense, useState } from "react";
import { toast } from "sonner";

import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { REVIEW_THRESHOLD } from "../lib/suggest";
import { useOffice, useOfficeActions } from "../hooks/use-office";
import type { Suggestion } from "../types";
import {
  DemoNotice,
  FilterSelect,
  Figure,
  Money,
  OfficeDataGate,
  Panel,
} from "./office-ui";

function Confidence({ value }: { value: number }) {
  const pct = Math.round(value * 100);
  const tone =
    value >= REVIEW_THRESHOLD
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

function ReviewRow({ s }: { s: Suggestion }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { decideSuggestion } = useOfficeActions();
  const invoice = useOffice((st) =>
    st.invoices.find((i) => i.id === s.invoiceId),
  );
  const line = invoice?.lines.find((l) => l.id === s.lineId);
  const [editing, setEditing] = useState(false);
  const [code, setCode] = useState(s.accountCode);
  if (!invoice || !line) return null;

  const approve = () => {
    const changed = code.trim() && code.trim() !== s.accountCode;
    decideSuggestion(
      s.id,
      "approved",
      changed
        ? { accountCode: code.trim(), accountName: t("office.manual_code") }
        : undefined,
    );
    toast.success(
      t("office.review_approved_toast", {
        code: changed ? code.trim() : s.accountCode,
      }),
    );
  };

  return (
    <li className="grid gap-3 px-4 py-3 lg:grid-cols-[minmax(0,1.4fr)_minmax(0,1fr)_auto] lg:items-center">
      <div className="min-w-0">
        <div className="truncate font-medium">{line.name}</div>
        <div className="text-muted-foreground flex flex-wrap items-center gap-x-2 text-xs">
          <Link
            className="hover:underline"
            href={routes.tenant.invoice(slug, invoice.id)}
          >
            <Figure>{invoice.number}</Figure>
          </Link>
          <span>·</span>
          <span className="truncate">{invoice.counterparty.title}</span>
          <span>·</span>
          <Money value={line.lineTotal} currency={invoice.currency} />
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
            onKeyDown={(e) => e.key === "Enter" && approve()}
          />
        ) : (
          <div className="flex items-baseline gap-2">
            <Figure className="font-semibold">{s.accountCode}</Figure>
            <span className="text-muted-foreground truncate text-xs">
              {s.accountName}
            </span>
          </div>
        )}
        <div className="mt-1 flex flex-wrap items-center gap-2">
          <Confidence value={s.confidence} />
          <span className="text-muted-foreground text-xs">
            {t(`office.origin_${s.origin}`)} ·{" "}
            {t(`office.reason.${s.reason.replace(":", "_")}`)}
          </span>
        </div>
      </div>
      <div className="flex gap-1">
        <Button size="sm" onClick={approve}>
          <Check className="size-4" />
          {t("office.approve")}
        </Button>
        {!editing ? (
          <Button
            size="sm"
            variant="outline"
            onClick={() => setEditing(true)}
            aria-label={t("office.change_code")}
          >
            <Pencil className="size-4" />
          </Button>
        ) : null}
        <Button
          size="sm"
          variant="outline"
          onClick={() => decideSuggestion(s.id, "rejected")}
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
  const taxpayers = useOffice((s) => s.taxpayers);
  const suggestions = useOffice((s) => s.suggestions);
  const [taxpayer, setTaxpayer] = useState("all");
  const invoiceFilter = params.get("invoice");
  const decided = suggestions.filter((x) => x.status !== "pending");

  const pending = suggestions
    .filter((s) => s.status === "pending")
    .filter((s) => taxpayer === "all" || s.taxpayerId === taxpayer)
    .filter((s) => !invoiceFilter || s.invoiceId === invoiceFilter)
    .sort((a, b) => a.confidence - b.confidence);

  return (
    <div className="space-y-4">
      <PageHeader
        title={t("office.review_title")}
        description={t("office.review_description", {
          threshold: Math.round(REVIEW_THRESHOLD * 100),
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
      <DemoNotice />
      <div className="flex flex-wrap items-center gap-3">
        <FilterSelect
          label={t("office.col_taxpayer")}
          value={taxpayer}
          onChange={setTaxpayer}
          options={[
            { value: "all", label: t("office.filter_all") },
            ...taxpayers.map((x) => ({ value: x.id, label: x.title })),
          ]}
        />
        {invoiceFilter ? (
          <Button asChild size="sm" variant="ghost">
            <Link href="?">{t("office.clear_invoice_filter")}</Link>
          </Button>
        ) : null}
      </div>
      <Panel
        title={t("office.review_pending", { count: pending.length })}
        description={t("office.review_sorted")}
      >
        {pending.length === 0 ? (
          <EmptyState
            className="m-4"
            title={t("office.review_empty")}
            description={t("office.review_empty_description")}
          />
        ) : (
          <ul className="divide-y">
            {pending.map((s) => (
              <ReviewRow key={s.id} s={s} />
            ))}
          </ul>
        )}
      </Panel>
      <p className="text-muted-foreground text-xs">
        {t("office.review_decided_count", { count: decided.length })}
      </p>
    </div>
  );
}

export function ReviewQueuePage() {
  return (
    <OfficeDataGate>
      <Suspense>
        <ReviewInner />
      </Suspense>
    </OfficeDataGate>
  );
}
