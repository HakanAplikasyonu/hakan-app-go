"use client";

import { X } from "lucide-react";
import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { useRelatedRecords } from "../hooks/use-related";
import { FOCUS_PARAM, parseFocus, relatedHref } from "../lib/related-href";
import type {
  RelatedRecord,
  RelatedSourceKind,
} from "../services/related.service";
import { Figure, Money, Panel, SeverityBadge } from "./office-ui";

/** Çeviri yoksa ham değer (bilinmeyen durum ya da öneri türü). */
function useTr() {
  const { t } = useLocale();
  return (key: string, fallback: string, vars?: Record<string, string>) => {
    const v = t(key, vars);
    return v === key ? fallback : v;
  };
}

function trDate(iso: string | null) {
  if (!iso) return "—";
  const [y, m, d] = iso.split("-");
  return y && m && d ? `${d}.${m}.${y}` : iso;
}

function RecordLabel({ item }: { item: RelatedRecord }) {
  const tr = useTr();
  if (item.kind === "suggestion")
    return <>{tr(`office.related.suggestion.${item.label}`, item.label)}</>;
  if (item.kind === "journal_draft")
    return (
      <>
        {item.detail
          ? tr("office.related.entry_no", item.detail, { no: item.detail })
          : null}
        {item.label ? ` · ${item.label}` : ""}
      </>
    );
  if (item.kind === "account")
    return (
      <>
        <Figure>{item.label}</Figure>
        {item.detail ? ` · ${item.detail}` : ""}
      </>
    );
  if (item.kind === "warning" || item.kind === "rule")
    return <Figure>{item.label}</Figure>;
  return <>{item.label || "—"}</>;
}

function Detail({ item }: { item: RelatedRecord }) {
  const tr = useTr();
  if (!item.detail) return null;
  if (item.kind === "warning" || item.kind === "rule") {
    const sev = item.detail;
    return sev === "info" || sev === "warning" || sev === "blocker" ? (
      <SeverityBadge severity={sev} />
    ) : null;
  }
  if (item.kind === "bank_transaction")
    return (
      <span className="text-text-2 text-xs">
        {tr(`office.related.debit_credit.${item.detail}`, item.detail)}
      </span>
    );
  if (item.kind === "invoice" || item.kind === "expense")
    return (
      <span className="text-text-2 block max-w-64 truncate text-xs">
        {item.detail}
      </span>
    );
  return null;
}

/**
 * Bir kaydın ilişkili kayıtları (#469): tür, kayıt, tarih, tutar, durum.
 * Bağlantılar mükellef detayında açılır; sekme çubuğu kaybolmaz.
 */
export function RelatedRecordsTable({
  engagement,
  kind,
  uuid,
}: {
  engagement: string;
  kind: RelatedSourceKind;
  uuid: string;
}) {
  const { t } = useLocale();
  const tr = useTr();
  const { slug } = useTenant();
  const query = useRelatedRecords(slug, engagement, kind, uuid);

  if (query.isLoading) return <Skeleton className="m-4 h-24" />;
  if (query.isError)
    return (
      <p role="alert" className="text-danger p-4 text-sm">
        {t("office.related.load_error")}
      </p>
    );
  const items = query.data?.items ?? [];
  if (items.length === 0)
    return (
      <p className="text-text-2 p-4 text-sm" data-testid="related-empty">
        {t("office.related.empty")}
      </p>
    );
  return (
    <div className="overflow-x-auto">
      <table className="w-full text-sm" data-testid="related-records">
        <thead className="text-text-2 text-left text-xs">
          <tr className="border-line border-b">
            <th scope="col" className="px-4 py-2 font-medium">
              {t("office.related.col_kind")}
            </th>
            <th scope="col" className="px-4 py-2 font-medium">
              {t("office.related.col_record")}
            </th>
            <th scope="col" className="px-4 py-2 font-medium">
              {t("office.related.col_date")}
            </th>
            <th scope="col" className="px-4 py-2 text-right font-medium">
              {t("office.related.col_amount")}
            </th>
            <th scope="col" className="px-4 py-2 font-medium">
              {t("office.related.col_status")}
            </th>
          </tr>
        </thead>
        <tbody className="divide-line divide-y">
          {items.map((item, i) => {
            const href = relatedHref(slug, engagement, item);
            const kindLabel = t(`office.related.kind.${item.kind}`);
            return (
              <tr key={`${item.kind}-${item.uuid ?? item.label}-${i}`}>
                <td className="px-4 py-2 align-top whitespace-nowrap">
                  <div className="font-medium">{kindLabel}</div>
                  <div className="text-text-3 text-xs">
                    {t(`office.related.relation.${item.relation}`)}
                  </div>
                </td>
                <td className="px-4 py-2 align-top">
                  {href ? (
                    <Link
                      href={href}
                      className="text-brand-text hover:underline"
                      aria-label={`${kindLabel}: ${item.label}`}
                    >
                      <RecordLabel item={item} />
                    </Link>
                  ) : (
                    <RecordLabel item={item} />
                  )}
                  <Detail item={item} />
                </td>
                <td className="px-4 py-2 align-top whitespace-nowrap">
                  <Figure>{trDate(item.date)}</Figure>
                </td>
                <td className="px-4 py-2 text-right align-top whitespace-nowrap">
                  {item.amount ? (
                    <Money
                      value={item.amount}
                      currency={item.currency ?? "TRY"}
                    />
                  ) : (
                    "—"
                  )}
                </td>
                <td className="px-4 py-2 align-top whitespace-nowrap">
                  {tr(`office.related.status.${item.status}`, item.status)}
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}

/** Detay sayfalarındaki "İlişkili kayıtlar" bölümü. */
export function RelatedRecordsPanel({
  engagement,
  kind,
  uuid,
}: {
  engagement: string;
  kind: RelatedSourceKind;
  uuid: string;
}) {
  const { t } = useLocale();
  return (
    <Panel
      title={t("office.related.title")}
      description={t("office.related.description")}
    >
      <RelatedRecordsTable engagement={engagement} kind={kind} uuid={uuid} />
    </Panel>
  );
}

/**
 * Mükellef detayında `?kayit=<tür>:<uuid>` ile seçili kaydın ilişkili
 * kayıtları; sekmenin üstünde, sekme çubuğunun altında (#469).
 */
export function FocusedRecord({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const params = useSearchParams();
  const pathname = usePathname();
  const focus = parseFocus(params.get(FOCUS_PARAM));
  if (!focus) return null;
  const rest = new URLSearchParams(params.toString());
  rest.delete(FOCUS_PARAM);
  const qs = rest.toString();
  return (
    <section
      aria-labelledby="focused-record-title"
      data-testid="focused-record"
    >
      <Panel
        title={
          <span id="focused-record-title">
            {t("office.related.focus_title", {
              kind: t(`office.related.kind.${focus.kind}`),
            })}
          </span>
        }
        description={t("office.related.description")}
        actions={
          <Button asChild size="sm" variant="ghost">
            <Link href={`${pathname}${qs ? `?${qs}` : ""}`} scroll={false}>
              <X className="size-4" aria-hidden />
              {t("office.related.focus_close")}
            </Link>
          </Button>
        }
      >
        <RelatedRecordsTable
          engagement={engagement}
          kind={focus.kind}
          uuid={focus.uuid}
        />
      </Panel>
    </section>
  );
}
