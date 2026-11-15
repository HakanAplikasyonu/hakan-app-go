/**
 * Ofis paneli veri tipleri. Alan adları planlanan backend modeline göre
 * (docs/01 §2, kaynak paket 01 / 05 / 31). Tutarlar ondalık string tutulur;
 * float'a çevrilmez (paket kuralı 6), hesap `lib/decimal.ts` ile kuruş üzerinden.
 */

export type Severity = "info" | "warning" | "blocker";
export type WarningStatus =
  "open" | "acknowledged" | "resolved" | "auto_resolved";

/** Ofis ↔ mükellef hizmet ilişkisi (engagement, W1 #2). */
export type Taxpayer = {
  id: string;
  /** VKN (10) veya TCKN (11). */
  taxId: string;
  title: string;
  taxOffice: string;
  kind: "company" | "person";
  /** Atanmış ofis üyeleri (engagement_assignment). */
  assigneeIds: string[];
  startedAt: string;
};

/** Atama listesi için ofis üyesi özeti. */
export type OfficeMember = {
  id: string;
  name: string;
  role: string;
};

export type InvoiceDirection = "sales" | "purchase";
export type InvoiceStatus = "new" | "in_review" | "approved";

export type TaxBreakdown = {
  /** GİB vergi kodu: 0015 KDV, 9015 KDV tevkifatı … */
  code: string;
  percent: string;
  taxable: string;
  amount: string;
};

export type InvoiceLine = {
  id: string;
  name: string;
  quantity: string;
  unit: string;
  unitPrice: string;
  lineTotal: string;
  vatPercent: string;
  vatAmount: string;
  /** Onaylanmış hesap kodu (ör. 770.01.003). */
  accountCode: string | null;
};

export type Invoice = {
  id: string;
  taxpayerId: string;
  /** ETTN (cbc:UUID). */
  ettn: string;
  number: string;
  issueDate: string;
  /** ProfileID: TEMELFATURA, TICARIFATURA, EARSIVFATURA … */
  profile: string;
  /** InvoiceTypeCode: SATIS, IADE, TEVKIFAT, ISTISNA … */
  typeCode: string;
  direction: InvoiceDirection;
  currency: string;
  /** Döviz ise TRY karşılığı için kur. */
  exchangeRate: string | null;
  counterparty: { title: string; taxId: string };
  lines: InvoiceLine[];
  taxes: TaxBreakdown[];
  totals: {
    lineExtension: string;
    taxExclusive: string;
    taxInclusive: string;
    payable: string;
  };
  status: InvoiceStatus;
  source: "upload" | "demo";
  /** Yüklenen ham UBL; görüntü bunun gömülü XSLT'sinden üretilir. */
  rawXml?: string;
  uploadedAt: string;
};

export type DataWarning = {
  id: string;
  taxpayerId: string;
  period: string;
  entityType: "invoice" | "invoice_item" | "period";
  entityId: string | null;
  code: string;
  severity: Severity;
  params: Record<string, string>;
  producer: string;
  status: WarningStatus;
  createdAt: string;
};

/** Hesap kodu önerisi; eşiğin altındakiler inceleme kuyruğuna düşer. */
export type Suggestion = {
  id: string;
  taxpayerId: string;
  invoiceId: string;
  lineId: string;
  accountCode: string;
  accountName: string;
  /** 0–1 */
  confidence: number;
  /** Önerinin kökeni (G12): kural, geçmiş onay, model. */
  origin: "rule" | "history" | "model";
  reason: string;
  status: "pending" | "approved" | "rejected";
  decidedAt?: string;
};

export type UploadFileResult = {
  fileName: string;
  outcome: "imported" | "duplicate" | "not_ubl" | "error";
  invoiceId?: string;
  ettn?: string;
  number?: string;
  warningCodes: string[];
  message?: string;
};

export type UploadReport = {
  id: string;
  taxpayerId: string;
  createdAt: string;
  files: UploadFileResult[];
};
