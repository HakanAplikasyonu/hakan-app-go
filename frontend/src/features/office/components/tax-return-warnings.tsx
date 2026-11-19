"use client";

import Link from "next/link";

import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

import { useWarnings } from "../hooks/use-warnings";
import { warningRecordHref } from "../lib/related-href";
import { SeverityBadge } from "./office-ui";
import { ApiWarningActions } from "./warning-actions";
import { useApiWarningText, useEntityLabel } from "./warnings-page";

/**
 * Beyanı etkileyen açık uyarılar (#528): beyan sekmesinde sayı yerine
 * liste. Her satır kaydına gider ve oradan karar verilebilir; liste o
 * dönemin açık uyarılarından beyanın saydığı kodlarla süzülür.
 */
export function TaxReturnWarnings({
  engagement,
  period,
  codes,
}: {
  engagement: string;
  period: string;
  codes: string[];
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const text = useApiWarningText();
  const entity = useEntityLabel();
  const q = useWarnings(
    slug,
    { period, status: "open", limit: 100, offset: 0 },
    engagement,
  );
  if (codes.length === 0) return null;
  const items = (q.data?.items ?? []).filter((w) => codes.includes(w.code));
  const all = routes.tenant.engagementWarnings(slug, engagement);
  return (
    <section
      aria-labelledby="tr-warnings"
      className="border-warn bg-surface rounded-xl border p-4"
    >
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
        <h3 id="tr-warnings" className="font-display text-base font-semibold">
          {t("office.tax_returns.blocking_title", { count: codes.length })}
        </h3>
        <Link href={all} className="text-brand text-sm hover:underline">
          {t("office.tax_returns.blocking_all")}
        </Link>
      </div>
      {q.isLoading ? (
        <p className="text-text-3 text-sm">{t("common.loading")}</p>
      ) : items.length === 0 ? (
        <p className="text-text-3 text-sm">
          {t("office.tax_returns.blocking_none")}
        </p>
      ) : (
        <ul className="divide-line divide-y">
          {items.map((w) => {
            const href =
              w.entity_type === "invoice" && w.entity_uuid
                ? routes.tenant.engagementInvoice(
                    slug,
                    engagement,
                    w.entity_uuid,
                  )
                : warningRecordHref(slug, w);
            return (
              <li
                key={w.uuid}
                className="flex flex-wrap items-center gap-3 py-2.5"
              >
                <SeverityBadge severity={w.severity} />
                <div className="min-w-56 flex-1">
                  <div className="text-sm">{text(w)}</div>
                  <div className="text-text-3 text-xs">
                    {entity(w.entity_type)}
                  </div>
                </div>
                {href ? (
                  <Link
                    href={href}
                    className="text-brand text-sm font-medium hover:underline"
                  >
                    {t("office.warnings_api.open_record", {
                      record: entity(w.entity_type),
                    })}
                  </Link>
                ) : null}
                <ApiWarningActions warning={w} />
              </li>
            );
          })}
        </ul>
      )}
    </section>
  );
}
