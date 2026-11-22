import type { components } from "@/generated/api";
import { apiConfig } from "@/config/api";
import { unwrap } from "@/lib/api";

import { TAXPAYER_HEADER, taxpayerRequest } from "./taxpayer.service";

export type TaxpayerForeignInvoice =
  components["schemas"]["TaxpayerForeignInvoice"];
export type TaxpayerForeignInvoiceList =
  components["schemas"]["TaxpayerForeignInvoiceList"];
export type TaxpayerForeignInvoiceUpload =
  components["schemas"]["EnvelopeTaxpayerForeignInvoiceUpload"]["data"];

/** Yabancı fatura dosyası sınırı (arşivle aynı, 20 MB). */
export const FOREIGN_INVOICE_MAX_BYTES = 20 * 1024 * 1024;

export const taxpayerForeignInvoiceKeys = {
  all: (uuid: string) =>
    ["taxpayer", uuid.toLowerCase(), "foreign-invoices"] as const,
  list: (uuid: string, period: string) =>
    ["taxpayer", uuid.toLowerCase(), "foreign-invoices", period] as const,
};

export const taxpayerForeignInvoiceService = {
  /** Bu ay (ya da seçilen ay) gönderilen yabancı faturalar, salt okunur. */
  list(taxpayer: string, period: string) {
    const q = new URLSearchParams({ period });
    return taxpayerRequest<TaxpayerForeignInvoiceList>(
      "GET",
      `/v1/taxpayer/foreign-invoices?${q.toString()}`,
      { taxpayer, silent: true },
    );
  },
  /** Fotoğraf ya da PDF + isteğe bağlı not; ofise taslak olarak gider. */
  async upload(
    taxpayer: string,
    file: File,
    note: string,
  ): Promise<TaxpayerForeignInvoiceUpload> {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const form = new FormData();
    form.append("file", file);
    if (note.trim()) form.append("note", note.trim());
    const response = await fetch(`${base}/v1/taxpayer/foreign-invoices`, {
      method: "POST",
      credentials: "include",
      headers: { Accept: "application/json", [TAXPAYER_HEADER]: taxpayer },
      body: form,
    });
    const payload = await response.json().catch(() => undefined);
    return unwrap<TaxpayerForeignInvoiceUpload>({ data: payload, response });
  },
};

/** İçinde bulunulan ay (Europe/Istanbul), YYYY-MM. */
export function currentForeignPeriod(now = new Date()): string {
  const parts = new Intl.DateTimeFormat("en-CA", {
    timeZone: "Europe/Istanbul",
    year: "numeric",
    month: "2-digit",
  }).formatToParts(now);
  const y = parts.find((p) => p.type === "year")?.value ?? "";
  const m = parts.find((p) => p.type === "month")?.value ?? "";
  return `${y}-${m}`;
}
