"use client";

import { BookOpenCheck, X } from "lucide-react";
import Link from "next/link";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { usePostInvoices } from "../hooks/use-office-invoices";
import { relatedHref } from "../lib/related-href";
import type {
  InvoicePostingBatch,
  InvoicePostingResult,
} from "../services/office-invoices.service";
import { monthText } from "./vat-deduction-period";

/**
 * Faturadan fiş (#669): dönemin fişi olmayan alış / satış faturaları taslak
 * fişe; satış belge ayında, alış KDV indirim ayında. Fişler normal kontrol →
 * onay → aktarım akışına girer. Fişlenemeyen fatura nedeniyle listelenir.
 */
export function InvoicePostings({
  slug,
  engagement,
  period,
}: {
  slug: string;
  engagement: string;
  period: string;
}) {
  const { t, locale } = useLocale();
  const { can } = usePermission();
  const post = usePostInvoices(slug, engagement);
  const [result, setResult] = useState<InvoicePostingBatch | null>(null);
  if (!engagement || !can(permissions.tenant.journals.write)) return null;
  const month = monthText(period, locale);

  return (
    <div className="space-y-2">
      <Button
        type="button"
        variant="outline"
        disabled={post.isPending}
        onClick={() => {
          setResult(null);
          post.mutate(period, { onSuccess: setResult });
        }}
      >
        <BookOpenCheck className="size-4" aria-hidden />
        {post.isPending
          ? t("office.invoice_posting.running")
          : t("office.invoice_posting.run", { month })}
      </Button>
      {post.isError ? (
        <p role="alert" className="text-danger text-sm">
          {post.error.message || t("office.invoice_posting.error")}
        </p>
      ) : null}
      {result ? (
        <PostingSummary
          slug={slug}
          engagement={engagement}
          month={month}
          result={result}
          onClose={() => setResult(null)}
        />
      ) : null}
    </div>
  );
}

function PostingSummary({
  slug,
  engagement,
  month,
  result,
  onClose,
}: {
  slug: string;
  engagement: string;
  month: string;
  result: InvoicePostingBatch;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const created = result.items.filter((i) => i.outcome === "created");
  const refused = result.items.filter((i) => i.outcome === "refused");
  return (
    <section
      data-testid="invoice-posting-result"
      aria-label={t("office.invoice_posting.result_title")}
      className="border-border bg-surface-2 space-y-2 rounded-md border p-3 text-sm"
    >
      <div className="flex items-start justify-between gap-2">
        <p role="status">
          {result.items.length === 0
            ? t("office.invoice_posting.nothing", { month })
            : t("office.invoice_posting.summary", {
                created: created.length,
                refused: refused.length,
              })}
        </p>
        <Button
          type="button"
          size="icon"
          variant="ghost"
          className="size-8"
          aria-label={t("office.invoice_posting.close")}
          onClick={onClose}
        >
          <X className="size-4" aria-hidden />
        </Button>
      </div>
      {created.length > 0 ? (
        <ul className="space-y-1">
          {created.map((i) => (
            <PostingRow
              key={i.invoice_uuid}
              slug={slug}
              engagement={engagement}
              item={i}
            />
          ))}
        </ul>
      ) : null}
      {refused.length > 0 ? (
        <>
          <p className="text-text-2">
            {t("office.invoice_posting.refused_title")}
          </p>
          <ul className="space-y-1">
            {refused.map((i) => (
              <PostingRow
                key={i.invoice_uuid}
                slug={slug}
                engagement={engagement}
                item={i}
              />
            ))}
          </ul>
        </>
      ) : null}
    </section>
  );
}

function PostingRow({
  slug,
  engagement,
  item,
}: {
  slug: string;
  engagement: string;
  item: InvoicePostingResult;
}) {
  const { t } = useLocale();
  if (item.outcome !== "refused" && item.draft) {
    const href = relatedHref(slug, engagement, {
      kind: "journal_draft",
      uuid: item.draft.uuid,
      label: "",
    });
    const text = t("office.invoice_posting.created_row", {
      number: item.invoice_number,
      entry: item.draft.entry_no,
    });
    return (
      <li data-testid="invoice-posting-created">
        {href ? (
          <Link href={href} className="underline">
            {text}
          </Link>
        ) : (
          text
        )}
      </li>
    );
  }
  return (
    <li data-testid="invoice-posting-refused">
      <span className="font-medium">{item.invoice_number}</span>
      {": "}
      {t(`office.invoice_posting.reason_${item.reason ?? "UNSUPPORTED_TYPE"}`, {
        line: item.line_no ?? 0,
      })}
    </li>
  );
}
