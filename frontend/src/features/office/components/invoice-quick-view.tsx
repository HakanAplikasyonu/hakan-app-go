"use client";

import { FileDown } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  useEngagementInvoice,
  useInvoiceUBL,
} from "../hooks/use-office-invoices";
import { toPanelInvoice } from "../lib/from-api";
import { downloadFile } from "../services/download";
import { InvoicePreview } from "./invoice-preview";

/** Faturanın kayıttan üretilen PDF'ini indirir (#528). */
export function InvoicePdfButton({
  engagement,
  invoice,
  number,
}: {
  engagement: string;
  invoice: string;
  number?: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [busy, setBusy] = useState(false);
  return (
    <Button
      size="sm"
      variant="outline"
      className="max-md:min-h-11"
      disabled={busy}
      onClick={() => {
        setBusy(true);
        void downloadFile(
          slug,
          `/v1/tenant/engagements/${encodeURIComponent(engagement)}/invoices/${encodeURIComponent(invoice)}/pdf`,
          {},
          `fatura-${number ?? invoice}.pdf`,
        )
          .catch(() => toast.error(t("office.invoice_pdf.failed")))
          .finally(() => setBusy(false));
      }}
    >
      <FileDown className="size-4" aria-hidden />
      {t("office.invoice_pdf.download")}
    </Button>
  );
}

/**
 * Faturanın görüntüsü yan panelde (#528): beyan ekranından çıkmadan
 * fatura okunur, PDF'i indirilir. Görüntü UBL'nin XSLT'si, yoksa kayıttan
 * sade şablon.
 */
export function InvoiceQuickView({
  engagement,
  invoice,
}: {
  engagement: string;
  invoice: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const detail = useEngagementInvoice(slug, engagement, invoice);
  const ubl = useInvoiceUBL(slug, engagement, invoice, !!detail.data?.has_ubl);
  const view = useMemo(
    () =>
      detail.data ? toPanelInvoice(detail.data, engagement, ubl.data) : null,
    [detail.data, engagement, ubl.data],
  );
  if (detail.isLoading || (detail.data?.has_ubl && ubl.isLoading))
    return <Skeleton className="h-96 w-full" />;
  if (detail.isError || !view || !detail.data)
    return (
      <p className="text-text-3 text-sm">
        {t("office.invoice_pdf.load_failed")}
      </p>
    );
  return (
    <section className="space-y-2" data-testid="invoice-quick-view">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <h4 className="text-sm font-semibold">{t("office.preview_title")}</h4>
        <InvoicePdfButton
          engagement={engagement}
          invoice={invoice}
          number={detail.data.invoice_number}
        />
      </div>
      <div className="border-line overflow-hidden rounded-xl border">
        <InvoicePreview invoice={view} />
      </div>
    </section>
  );
}
