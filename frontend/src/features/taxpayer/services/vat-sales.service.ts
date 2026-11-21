import type { components } from "@/generated/api";
import { ApiError } from "@/lib/api";

import { taxpayerRequest } from "./taxpayer.service";

export type TaxpayerVatSales = components["schemas"]["TaxpayerVATSales"];

/** Ofis bölümü kapattığında uç noktanın döndüğü kod (#780). */
export const VAT_PANEL_DISABLED = "TAXPAYER_VAT_PANEL_DISABLED";

export const taxpayerVatSalesKeys = {
  get: (uuid: string, period: string) =>
    ["taxpayer", uuid.toLowerCase(), "vat-sales", period] as const,
};

/**
 * Bölüm gösterilmez: ofis kapattı (403) ya da mükellefin aktif ofisi yok
 * (404). İkisinde de hata mesajı yerine kart hiç çizilmez.
 */
export function isVatPanelHidden(error: unknown) {
  return (
    error instanceof ApiError &&
    (error.code === VAT_PANEL_DISABLED || error.status === 404)
  );
}

export const taxpayerVatSalesService = {
  /** Bu ayın (ya da seçilen ayın) anlık KDV'si ve satış raporu. */
  get(taxpayer: string, period: string) {
    const q = new URLSearchParams({ period });
    return taxpayerRequest<TaxpayerVatSales>(
      "GET",
      `/v1/taxpayer/vat-sales?${q.toString()}`,
      { taxpayer, silent: true },
    );
  },
};
