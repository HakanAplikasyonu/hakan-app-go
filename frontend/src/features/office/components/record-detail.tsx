"use client";

import { ExternalLink } from "lucide-react";
import Link from "next/link";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { warningRecordHref } from "../lib/related-href";
import type { RelatedSourceKind } from "../services/related.service";
import { BankTransactionMatchingPanel } from "./bank-transaction-matching";
import { ExpenseQuickView } from "./expense-quick-view";
import { InvoiceQuickView } from "./invoice-quick-view";
import { RelatedRecordsTable } from "./related-records";
import { ZReportQuickView } from "./zreport-quick-view";
import { useEntityLabel } from "./warnings-page";

/**
 * Bir kaydın yan paneldeki gövdesi (#528, #529): düzeltme alanı (banka
 * hareketinde fatura eşleştirme, faturada görüntü ve PDF, masrafta onay, Z raporunda düzeltme), araya konan
 * içerik (uyarılar), ilişkili kayıtlar ve kaydın kendi sayfasına bağlantı.
 * Kaydı olmayan uyarıda (`kind` null) ilişkili kayıtlar uyarının kendisinden.
 */
export function RecordDetail({
  engagement,
  entityType,
  kind,
  uuid,
  warning,
  onChanged,
  children,
}: {
  engagement: string;
  entityType: string;
  kind: RelatedSourceKind | null;
  uuid: string | null;
  /** Kaydı olmayan uyarıda ilişkili kayıtların kaynağı. */
  warning?: string;
  onChanged?: () => void;
  children?: ReactNode;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const entity = useEntityLabel();
  const record = entity(entityType);
  const href = !uuid
    ? null
    : kind === "invoice"
      ? routes.tenant.engagementInvoice(slug, engagement, uuid)
      : warningRecordHref(slug, {
          engagement_uuid: engagement,
          entity_type: entityType,
          entity_uuid: uuid,
        });
  const related: { kind: RelatedSourceKind; uuid: string } | null =
    kind && uuid
      ? { kind, uuid }
      : warning
        ? { kind: "warning", uuid: warning }
        : null;
  return (
    <div className="space-y-5">
      {kind === "bank_transaction" && uuid ? (
        <BankTransactionMatchingPanel
          engagement={engagement}
          transaction={uuid}
          onChanged={onChanged}
        />
      ) : null}
      {kind === "invoice" && uuid ? (
        <InvoiceQuickView engagement={engagement} invoice={uuid} />
      ) : null}
      {kind === "retail_sale" && uuid ? (
        <ZReportQuickView
          engagement={engagement}
          sale={uuid}
          onChanged={onChanged}
        />
      ) : null}
      {kind === "expense" && uuid ? (
        <ExpenseQuickView
          engagement={engagement}
          expense={uuid}
          onChanged={onChanged}
        />
      ) : null}

      {children}

      {related ? (
        <section className="space-y-2">
          <h4 className="text-sm font-semibold">{t("office.related.title")}</h4>
          <RelatedRecordsTable
            engagement={engagement}
            kind={related.kind}
            uuid={related.uuid}
          />
        </section>
      ) : null}

      {href ? (
        <Button asChild variant="outline" size="sm">
          <Link href={href}>
            <ExternalLink className="size-4" aria-hidden />
            {t("office.tax_returns.panel_open_tab", { record })}
          </Link>
        </Button>
      ) : null}
    </div>
  );
}
