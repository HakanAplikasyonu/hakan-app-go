import type { Invoice, InvoiceLine, Suggestion } from "../types";

/**
 * Demo hesap kodu önericisi. Gerçek öneri motoru W4'te (kural seti + model).
 * Burada yalnızca inceleme kuyruğunu besleyecek kadar basit anahtar kelime
 * kuralları var; güven skoru ve köken (G12) alanları gerçek modelle aynı.
 */
type Rule = { match: RegExp; code: string; name: string; confidence: number };

const purchaseRules: Rule[] = [
  {
    match: /kırtasiye|kağıt|kağıd|toner|kalem/i,
    code: "770.01.003",
    name: "Kırtasiye giderleri",
    confidence: 0.94,
  },
  {
    match: /kira/i,
    code: "770.01.001",
    name: "Kira giderleri",
    confidence: 0.96,
  },
  {
    match: /akaryakıt|benzin|motorin|yakıt/i,
    code: "770.01.005",
    name: "Taşıt yakıt giderleri",
    confidence: 0.91,
  },
  {
    match: /yazılım|lisans|abonelik|bulut/i,
    code: "770.01.010",
    name: "Yazılım ve lisans giderleri",
    confidence: 0.72,
  },
  {
    match: /danışman|müşavir/i,
    code: "770.01.008",
    name: "Danışmanlık giderleri",
    confidence: 0.68,
  },
  {
    match: /bilgisayar|laptop|monitör|yazıcı/i,
    code: "255.01",
    name: "Demirbaşlar",
    confidence: 0.63,
  },
  {
    match: /elektrik|su|doğalgaz|telefon|internet/i,
    code: "770.01.004",
    name: "Enerji ve iletişim giderleri",
    confidence: 0.88,
  },
];

/** Eşik: altındaki öneriler inceleme kuyruğuna düşer (docs/01 §3). */
export const REVIEW_THRESHOLD = 0.85;

export function suggestForLine(
  invoice: Pick<Invoice, "id" | "taxpayerId" | "direction" | "typeCode">,
  line: InvoiceLine,
  makeId: () => string,
): Suggestion {
  const base = {
    id: makeId(),
    taxpayerId: invoice.taxpayerId,
    invoiceId: invoice.id,
    lineId: line.id,
  };
  if (invoice.direction === "sales") {
    const iade = invoice.typeCode === "IADE";
    return {
      ...base,
      accountCode: iade ? "610.01" : "600.01",
      accountName: iade ? "Satıştan iadeler" : "Yurt içi satışlar",
      confidence: 0.97,
      origin: "rule",
      reason: iade ? "rule:sales_return" : "rule:sales_default",
      status: "pending",
    };
  }
  const rule = purchaseRules.find((r) => r.match.test(line.name));
  if (rule) {
    return {
      ...base,
      accountCode: rule.code,
      accountName: rule.name,
      confidence: rule.confidence,
      origin: rule.confidence >= REVIEW_THRESHOLD ? "rule" : "model",
      reason:
        rule.confidence >= REVIEW_THRESHOLD
          ? "rule:keyword"
          : "model:similar_lines",
      status: "pending",
    };
  }
  return {
    ...base,
    accountCode: "153.01",
    accountName: "Ticari mallar",
    confidence: 0.42,
    origin: "model",
    reason: "model:fallback",
    status: "pending",
  };
}
