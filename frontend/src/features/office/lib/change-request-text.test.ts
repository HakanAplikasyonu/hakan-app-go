import { describe, expect, it } from "vitest";

import office from "@/locales/tr/office.json";
import sources from "@/locales/tr/sources.json";

import {
  CHANGE_FIELDS,
  changeFieldLabel,
  changeFieldValue,
} from "./change-request-text";

const dict: Record<string, Record<string, string>> = { office, sources };

/** Gerçek Türkçe sözlükle t; {{x}} değişkenleri yerine konur. */
const t = (key: string, vars?: Record<string, string | number>) => {
  const [ns, ...rest] = key.split(".");
  const v = dict[ns]?.[rest.join(".")];
  if (v === undefined) return key;
  return v.replace(/\{\{(\w+)\}\}/g, (_, k) => String(vars?.[k] ?? ""));
};

/** Ekranda görünmemesi gereken ham kod: snake_case, i18n anahtarı, enum. */
const RAW =
  /\b[a-z]+(?:_[a-z0-9]+)+\b|\b(?:office|sources|common)\.[a-z_.]+|\b(?:SATIS|ALIS|IADE|TEVKIFAT|ISTISNA|TRY|incoming|outgoing)\b/;

describe("değişiklik talebi Türkçe (#710)", () => {
  it("sunucunun her alanının Türkçe adı var", () => {
    for (const f of CHANGE_FIELDS) {
      const label = changeFieldLabel(f, t);
      expect(label, f).not.toBe("Diğer bilgi");
      expect(label, f).not.toMatch(RAW);
    }
    expect(changeFieldLabel("yeni_alan", t)).toBe("Diğer bilgi");
  });

  it("değerler sade metin", () => {
    const v = (f: string, x: string) => changeFieldValue(f, x, t, "tr");
    expect(v("direction", "outgoing")).toBe("Satış");
    expect(v("direction", "incoming")).toBe("Alış");
    expect(v("type", "SATIS")).toBe("Satış");
    expect(v("currency", "TRY")).toBe("TL");
    expect(v("payable_amount", "492.00")).toBe("₺ 492,00");
    expect(v("period", "2026-08")).toBe("Ağustos 2026");
    expect(v("issue_date", "2026-08-14")).toBe("14.08.2026");
    expect(v("source", "manual_upload/ubl_upload")).toBe(
      "Elle yükleme · UBL dosyası",
    );
    expect(v("source", "entegrator_izibiz/api")).toBe(
      "Izibiz entegratörü · Otomatik bağlantı",
    );
    expect(v("source", "bank_ziraat/mt940")).toBe(
      "Ziraat bankası dosyası · MT940 ekstresi",
    );
    expect(v("tax_amount", "—")).toBe("—");
  });

  it("bilinmeyen kaynak da ham görünmez", () => {
    const out = changeFieldValue("source", "yeni_sistem/yeni_yol", t, "tr");
    expect(out).toBe("Diğer kaynak · Diğer yol");
    expect(out).not.toMatch(RAW);
  });
});
