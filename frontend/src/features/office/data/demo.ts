/**
 * Ofis paneli demo verisi (D17 iç kontrol). Kişi ve firma adları uydurmadır;
 * VKN/TCKN'ler test amaçlı, gerçek bir mükellefe ait değil.
 * Backend tabloları (W2 #9) gelince bu dosya `make demo-seed` (#17) ile
 * aynı veriyi üreten seed'e taşınır.
 */

import { add, mul, percentOf, sub, sum } from "../lib/decimal";
import { REVIEW_THRESHOLD, suggestForLine } from "../lib/suggest";
import { severityOf } from "../lib/warnings";
import type {
  DataWarning,
  Invoice,
  InvoiceLine,
  OfficeMember,
  Suggestion,
  Taxpayer,
} from "../types";

export const DEMO_PERIOD = "2026-10";

export const demoMembers: OfficeMember[] = [
  { id: "demo-member-ayse", name: "Ayşe Demir", role: "Müşavir" },
  { id: "demo-member-can", name: "Can Kaya", role: "Stajyer" },
];

export const demoTaxpayers: Taxpayer[] = [
  {
    id: "tp-atlas",
    taxId: "9000000017",
    title: "Atlas Gıda Ticaret A.Ş.",
    taxOffice: "Kadıköy",
    kind: "company",
    assigneeIds: ["demo-member-ayse"],
    startedAt: "2025-01-01",
  },
  {
    id: "tp-selin",
    taxId: "10000000146",
    title: "Selin Yılmaz",
    taxOffice: "Çankaya",
    kind: "person",
    assigneeIds: ["demo-member-ayse", "demo-member-can"],
    startedAt: "2026-03-01",
  },
];

type LineSeed = [
  name: string,
  qty: string,
  unit: string,
  price: string,
  vat: string,
];

function lines(seeds: LineSeed[]): InvoiceLine[] {
  return seeds.map(([name, qty, unit, price, vat], i) => {
    const total = mul(qty, price);
    const vatAmount = percentOf(total, vat);
    return {
      id: String(i + 1),
      name,
      quantity: qty,
      unit,
      unitPrice: price,
      lineTotal: total,
      vatPercent: vat,
      vatAmount,
      accountCode: null,
    };
  });
}

function invoice(
  p: Omit<
    Invoice,
    | "taxes"
    | "totals"
    | "status"
    | "source"
    | "uploadedAt"
    | "exchangeRate"
    | "currency"
  > &
    Partial<Pick<Invoice, "currency" | "exchangeRate">> & {
      withholding?: string;
    },
): Invoice {
  const base = sum(p.lines.map((l) => l.lineTotal));
  const vat = sum(p.lines.map((l) => l.vatAmount));
  const byRate = new Map<string, InvoiceLine[]>();
  for (const l of p.lines)
    byRate.set(l.vatPercent, [...(byRate.get(l.vatPercent) ?? []), l]);
  const taxes = [...byRate].map(([percent, ls]) => ({
    code: "0015",
    percent,
    taxable: sum(ls.map((l) => l.lineTotal)),
    amount: sum(ls.map((l) => l.vatAmount)),
  }));
  const withholding = p.withholding ?? "0.00";
  if (p.withholding)
    taxes.push({
      code: "9015",
      percent: "50",
      taxable: vat,
      amount: withholding,
    });
  const inclusive = add(base, vat);
  return {
    currency: "TRY",
    exchangeRate: null,
    ...p,
    taxes,
    totals: {
      lineExtension: base,
      taxExclusive: base,
      taxInclusive: inclusive,
      payable: sub(inclusive, withholding),
    },
    status: "new",
    source: "demo",
    uploadedAt: "2026-11-02T09:12:00Z",
  };
}

export const demoInvoices: Invoice[] = [
  invoice({
    id: "inv-1",
    taxpayerId: "tp-atlas",
    ettn: "3f2a9c1e-7b44-4d0e-9a61-0c5e2b8d71aa",
    number: "ATL2026000000418",
    issueDate: "2026-10-03",
    profile: "TICARIFATURA",
    typeCode: "SATIS",
    direction: "sales",
    counterparty: { title: "Yıldız Market Ltd. Şti.", taxId: "9100000021" },
    lines: lines([
      ["Zeytinyağı 5 L", "120", "C62", "640.00", "1"],
      ["Kuru kayısı 1 kg", "80", "KGM", "310.00", "1"],
    ]),
  }),
  invoice({
    id: "inv-2",
    taxpayerId: "tp-atlas",
    ettn: "8b1d0f72-2c3e-4a5b-9e1f-4d6c7a8b9c01",
    number: "KRT2026000001207",
    issueDate: "2026-10-07",
    profile: "TEMELFATURA",
    typeCode: "SATIS",
    direction: "purchase",
    counterparty: {
      title: "Kalem Kırtasiye San. Tic. Ltd. Şti.",
      taxId: "9200000035",
    },
    lines: lines([
      ["A4 fotokopi kağıdı (koli)", "10", "C62", "720.00", "20"],
      ["Toner kartuşu", "4", "C62", "1450.00", "20"],
    ]),
  }),
  invoice({
    id: "inv-3",
    taxpayerId: "tp-atlas",
    ettn: "c4e5f6a7-b8c9-4d0e-8f1a-2b3c4d5e6f70",
    number: "BLT2026000000093",
    issueDate: "2026-10-11",
    profile: "TICARIFATURA",
    typeCode: "SATIS",
    direction: "purchase",
    counterparty: { title: "Bulut Yazılım A.Ş.", taxId: "9300000049" },
    lines: lines([
      ["Muhasebe yazılımı yıllık lisans", "1", "C62", "18500.00", "20"],
      ["Kurulum ve danışmanlık hizmeti", "6", "HUR", "1250.00", "20"],
    ]),
  }),
  invoice({
    id: "inv-4",
    taxpayerId: "tp-atlas",
    ettn: "0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d",
    number: "OPT2026000004410",
    issueDate: "2026-10-15",
    profile: "TEMELFATURA",
    typeCode: "SATIS",
    direction: "purchase",
    counterparty: { title: "Opet Akaryakıt İstasyonu", taxId: "9400000052" },
    lines: lines([["Motorin", "180.5", "LTR", "48.90", "20"]]),
  }),
  invoice({
    id: "inv-5",
    taxpayerId: "tp-atlas",
    ettn: "1b2c3d4e-5f6a-4b7c-9d8e-0f1a2b3c4d5e",
    number: "OPT2026000004417",
    issueDate: "2026-10-15",
    profile: "TEMELFATURA",
    typeCode: "SATIS",
    direction: "purchase",
    counterparty: { title: "Opet Akaryakıt İstasyonu", taxId: "9400000052" },
    lines: lines([["Motorin", "180.5", "LTR", "48.90", "20"]]),
  }),
  invoice({
    id: "inv-6",
    taxpayerId: "tp-atlas",
    ettn: "2c3d4e5f-6a7b-4c8d-8e9f-1a2b3c4d5e6f",
    number: "TMZ2026000000077",
    issueDate: "2026-10-20",
    profile: "TICARIFATURA",
    typeCode: "TEVKIFAT",
    direction: "purchase",
    counterparty: { title: "Temiz Ofis Hizmetleri", taxId: "940000005" },
    lines: lines([["Temizlik hizmeti Ekim", "1", "C62", "14000.00", "20"]]),
    withholding: "1400.00",
  }),
  invoice({
    id: "inv-7",
    taxpayerId: "tp-selin",
    ettn: "5d6e7f80-91a2-4b3c-8d4e-5f6a7b8c9d0e",
    number: "SLN2026000000012",
    issueDate: "2026-10-05",
    profile: "EARSIVFATURA",
    typeCode: "SATIS",
    direction: "sales",
    counterparty: {
      title: "Mavi Tasarım Ajansı Ltd. Şti.",
      taxId: "9500000066",
    },
    lines: lines([["Grafik tasarım hizmeti", "1", "C62", "32000.00", "20"]]),
  }),
  invoice({
    id: "inv-8",
    taxpayerId: "tp-selin",
    ettn: "6e7f8091-a2b3-4c4d-9e5f-6a7b8c9d0e1f",
    number: "KRA2026000000010",
    issueDate: "2026-10-01",
    profile: "TEMELFATURA",
    typeCode: "SATIS",
    direction: "purchase",
    counterparty: { title: "Ege Gayrimenkul Ltd. Şti.", taxId: "9600000070" },
    lines: lines([["Ofis kirası Ekim 2026", "1", "C62", "22000.00", "20"]]),
  }),
  invoice({
    id: "inv-9",
    taxpayerId: "tp-selin",
    ettn: "7f8091a2-b3c4-4d5e-8f60-7b8c9d0e1f2a",
    number: "TKN2026000088123",
    issueDate: "2026-10-18",
    profile: "EARSIVFATURA",
    typeCode: "SATIS",
    direction: "purchase",
    counterparty: { title: "Teknosa İç ve Dış Tic. A.Ş.", taxId: "9700000084" },
    lines: lines([["Dizüstü bilgisayar 14 inç", "1", "C62", "41500.00", "20"]]),
  }),
];

// inv-3: başlık KDV'si satırlardan 0,40 TL farklı (TAX_TOTAL_MISMATCH örneği).
{
  const inv = demoInvoices.find((i) => i.id === "inv-3")!;
  inv.taxes[0] = { ...inv.taxes[0], amount: add(inv.taxes[0].amount, "0.40") };
}

let seq = 0;
const nextId = () => `demo-${++seq}`;

function warning(
  invoiceId: string,
  code: string,
  params: Record<string, string> = {},
  status: DataWarning["status"] = "open",
): DataWarning {
  const inv = demoInvoices.find((i) => i.id === invoiceId)!;
  return {
    id: nextId(),
    taxpayerId: inv.taxpayerId,
    period: DEMO_PERIOD,
    entityType: "invoice",
    entityId: invoiceId,
    code,
    severity: severityOf(code),
    params,
    producer: "ubl_parser",
    status,
    createdAt: inv.uploadedAt,
  };
}

export const demoWarnings: DataWarning[] = [
  warning("inv-1", "PENDING_RESPONSE"),
  warning("inv-3", "TAX_TOTAL_MISMATCH", {
    expected: "4600.40",
    actual: "4600.00",
  }),
  warning("inv-5", "POSSIBLE_DUPLICATE", { other: "OPT2026000004410" }),
  warning("inv-6", "INVALID_TAX_ID", { value: "940000005" }),
  warning("inv-9", "PARTY_MISMATCH", {
    expected: "10000000146",
    actual: "10000000154",
  }),
];

/** Her satır için öneri; eşiğin üstündekiler onaylı başlar. */
export const demoSuggestions: Suggestion[] = demoInvoices.flatMap((inv) =>
  inv.lines.map((line) => {
    const s = suggestForLine(inv, line, nextId);
    if (s.confidence >= REVIEW_THRESHOLD) {
      line.accountCode = s.accountCode;
      return { ...s, status: "approved" as const, decidedAt: inv.uploadedAt };
    }
    return s;
  }),
);

for (const inv of demoInvoices) {
  const pending = demoSuggestions.some(
    (s) => s.invoiceId === inv.id && s.status === "pending",
  );
  inv.status = pending ? "in_review" : "approved";
}
