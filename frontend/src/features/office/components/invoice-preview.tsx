"use client";

import { quantity } from "@/lib/utils/format";
import { useMemo } from "react";

import { useLocale } from "@/providers/locale-provider";

import { formatAmount } from "../lib/decimal";
import type { Invoice } from "../types";

function esc(s: string) {
  return s.replace(/[&<>"']/g, (c) => `&#${c.charCodeAt(0)};`);
}

/** Faturanın kendi gömülü XSLT'si ile tarayıcıda HTML üretir (varsa). */
function renderEmbeddedXslt(xml: string): string | null {
  if (typeof window === "undefined" || typeof XSLTProcessor === "undefined")
    return null;
  try {
    const doc = new DOMParser().parseFromString(xml, "application/xml");
    const bins = Array.from(
      doc.getElementsByTagNameNS("*", "EmbeddedDocumentBinaryObject"),
    );
    const bin = bins.find((b) =>
      /\.xslt?$/i.test(b.getAttribute("filename") ?? ""),
    );
    if (!bin) return null;
    const raw = atob((bin.textContent ?? "").replace(/\s+/g, ""));
    const xsltText = new TextDecoder().decode(
      Uint8Array.from(raw, (c) => c.charCodeAt(0)),
    );
    const xslt = new DOMParser().parseFromString(xsltText, "application/xml");
    if (xslt.getElementsByTagName("parsererror").length) return null;
    const proc = new XSLTProcessor();
    proc.importStylesheet(xslt);
    const out = proc.transformToDocument(doc);
    return out?.documentElement
      ? new XMLSerializer().serializeToString(out)
      : null;
  } catch {
    return null;
  }
}

/** XSLT yoksa kayıttan sade bir görüntü (yalnızca demo). */
function renderFallback(inv: Invoice, labels: Record<string, string>): string {
  const rows = inv.lines
    .map(
      (l) =>
        `<tr><td>${esc(l.name)}</td><td class="n">${esc(quantity(l.quantity))} ${esc(l.unit)}</td><td class="n">${formatAmount(l.unitPrice)}</td><td class="n">%${esc(l.vatPercent)}</td><td class="n">${formatAmount(l.lineTotal)}</td></tr>`,
    )
    .join("");
  const taxes = inv.taxes
    .map(
      (t) =>
        `<tr><td>${esc(t.code)} %${esc(t.percent)}</td><td class="n">${formatAmount(t.amount)}</td></tr>`,
    )
    .join("");
  return `<!doctype html><html><head><meta charset="utf-8"><style>
  body{font:13px/1.45 system-ui,sans-serif;color:#0f172b;margin:24px}
  h1{font-size:18px;margin:0 0 4px} .m{font-family:ui-monospace,monospace;font-variant-numeric:tabular-nums}
  table{width:100%;border-collapse:collapse;margin-top:16px} td,th{border-bottom:1px solid #e2e8f0;padding:6px 4px;text-align:left}
  .n{text-align:right;font-family:ui-monospace,monospace;font-variant-numeric:tabular-nums;white-space:nowrap}
  .g{display:grid;grid-template-columns:1fr 1fr;gap:16px;margin-top:12px} .muted{color:#62748e;font-size:12px}
  </style></head><body>
  <div class="muted">${esc(labels.fallback)}</div>
  <h1>${esc(inv.profile)} · ${esc(inv.typeCode)}</h1>
  <div class="m">${esc(inv.number)} · ${esc(inv.issueDate)}</div>
  <div class="m muted">ETTN ${esc(inv.ettn)}</div>
  <div class="g"><div><div class="muted">${esc(labels.counterparty)}</div><b>${esc(inv.counterparty.title)}</b><div class="m">${esc(inv.counterparty.taxId)}</div></div></div>
  <table><thead><tr><th>${esc(labels.item)}</th><th class="n">${esc(labels.qty)}</th><th class="n">${esc(labels.price)}</th><th class="n">KDV</th><th class="n">${esc(labels.total)}</th></tr></thead><tbody>${rows}</tbody></table>
  <table style="width:50%;margin-left:auto"><tbody>${taxes}
  <tr><td><b>${esc(labels.payable)}</b></td><td class="n"><b>${formatAmount(inv.totals.payable)} ${esc(inv.currency)}</b></td></tr></tbody></table>
  </body></html>`;
}

/**
 * Fatura görüntüsü (T2: kaydın yanında). Hedef: backend'in go-ubltr `render`
 * çıktısı (W2). O gelene kadar faturanın gömülü XSLT'si tarayıcıda işlenir,
 * yoksa kayıttan sade bir görüntü çizilir. Her durumda betik çalışmayan,
 * sandbox'lı iframe içinde.
 */
export function InvoicePreview({ invoice }: { invoice: Invoice }) {
  const { t } = useLocale();
  const { html, source } = useMemo(() => {
    const fromXslt = invoice.rawXml ? renderEmbeddedXslt(invoice.rawXml) : null;
    if (fromXslt) return { html: fromXslt, source: "xslt" as const };
    return {
      html: renderFallback(invoice, {
        fallback: t("office.preview_fallback_banner"),
        counterparty: t("office.col_counterparty"),
        item: t("office.col_item"),
        qty: t("office.col_quantity"),
        price: t("office.col_unit_price"),
        total: t("office.col_line_total"),
        payable: t("office.col_payable"),
      }),
      source: "fallback" as const,
    };
  }, [invoice, t]);

  return (
    <div className="flex h-full flex-col">
      <p className="text-muted-foreground border-b px-4 py-2 text-xs">
        {source === "xslt"
          ? t("office.preview_source_xslt")
          : t("office.preview_source_fallback")}
      </p>
      <iframe
        title={t("office.preview_title")}
        sandbox=""
        srcDoc={html}
        className="min-h-[720px] w-full flex-1 rounded-b-xl bg-white"
      />
    </div>
  );
}
