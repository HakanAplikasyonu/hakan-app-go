import { describe, expect, it } from "vitest";

import trOffice from "@/locales/tr/office.json";

import { resolveWarningMessage } from "./office-ui";

const messages = trOffice as Record<string, string>;

/** Yerel katalogla basit t(): `office.` öneki, {{x}} doldurma. */
function t(key: string, params?: Record<string, string | number>): string {
  const raw = messages[key.replace(/^office\./, "")];
  if (raw === undefined) return key;
  return raw.replace(/\{\{(\w+)\}\}/g, (m, k: string) =>
    params && params[k] !== undefined ? String(params[k]) : m,
  );
}

describe("resolveWarningMessage", () => {
  it("olağandışı tutarda cari karşılaştırmasını gösterir", () => {
    const msg = resolveWarningMessage(t, "UNUSUAL_AMOUNT", {
      variant: "party_incoming",
      max: "12.400,00",
      median: "10.250,00",
      count: "6",
      amount: "1.000.000,00",
    });
    expect(msg).toBe(
      "Bu cariden son 12 ayda en fazla 12.400,00 TL fatura geldi (ortanca 10.250,00 TL, 6 fatura); bu fatura 1.000.000,00 TL.",
    );
  });

  it("varyantın metni yoksa kodun metnine döner", () => {
    const msg = resolveWarningMessage(t, "NEW_PARTY_HIGH_AMOUNT", {
      variant: "incoming",
      amount: "250.000,00",
      limit: "100.000,00",
    });
    expect(msg).toContain("ilk kez görülüyor");
    expect(msg).toContain("250.000,00 TL");
  });

  it("eksik parametrede kısa ada döner, bilinmeyen kodda null", () => {
    expect(resolveWarningMessage(t, "OUT_OF_PERIOD", { variant: "lock" })).toBe(
      "Dönem dışı belge",
    );
    expect(resolveWarningMessage(t, "NO_SUCH_CODE", {})).toBeNull();
  });
});
