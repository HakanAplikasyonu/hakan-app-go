import { routes } from "@/config/routes";

import type {
  RelatedRecord,
  RelatedSourceKind,
} from "../services/related.service";

/**
 * Mükellef detayında bir kaydı seçili gösteren URL parametresi (#469):
 * `?kayit=<tür>:<uuid>`. Düzen, sekmenin üstünde o kaydın ilişkili
 * kayıtlarını gösterir; sekme çubuğu yerinde kalır.
 */
export const FOCUS_PARAM = "kayit";

const SOURCE_KINDS: RelatedSourceKind[] = [
  "invoice",
  "bank_transaction",
  "journal_draft",
  "warning",
  "expense",
  "retail_sale",
  "payroll_run",
  "financial_transaction",
];

/** `?kayit=` değerini çözer; geçersizse null. */
export function parseFocus(
  value: string | null | undefined,
): { kind: RelatedSourceKind; uuid: string } | null {
  if (!value) return null;
  const i = value.indexOf(":");
  if (i < 0) return null;
  const kind = value.slice(0, i) as RelatedSourceKind;
  const uuid = value.slice(i + 1);
  if (!SOURCE_KINDS.includes(kind)) return null;
  if (
    !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(
      uuid,
    )
  )
    return null;
  return { kind, uuid };
}

function focus(base: string, kind: string, uuid: string) {
  return `${base}?${FOCUS_PARAM}=${encodeURIComponent(`${kind}:${uuid}`)}`;
}

/**
 * İlişkili kaydın mükellef detayındaki adresi: fatura kendi detayına, diğer
 * kayıtlar sekmelerine (`?kayit=` ile seçili); uyarının kuralı Uyarılar
 * sekmesinde o koda süzülür. Cari hesabın sayfası yok: null.
 */
export function relatedHref(
  slug: string,
  engagement: string,
  item: Pick<RelatedRecord, "kind" | "uuid" | "label">,
): string | null {
  const t = routes.tenant;
  const id = item.uuid;
  switch (item.kind) {
    case "invoice":
      return id ? t.engagementInvoice(slug, engagement, id) : null;
    case "bank_transaction":
      return id
        ? focus(t.engagementBank(slug, engagement), item.kind, id)
        : null;
    case "journal_draft":
      return id
        ? focus(t.engagementJournals(slug, engagement), item.kind, id)
        : null;
    case "warning":
      return id
        ? focus(t.engagementWarnings(slug, engagement), item.kind, id)
        : null;
    case "expense":
      return id
        ? focus(t.engagementExpenses(slug, engagement), item.kind, id)
        : null;
    case "retail_sale":
      return id
        ? focus(t.engagementZReports(slug, engagement), item.kind, id)
        : null;
    case "payroll_run":
      return id
        ? focus(t.engagementPayroll(slug, engagement), item.kind, id)
        : null;
    case "financial_transaction":
      return id
        ? focus(t.engagementEasytrade(slug, engagement), item.kind, id)
        : null;
    case "suggestion":
      return t.engagementReview(slug, engagement);
    case "rule":
      return `${t.engagementWarnings(slug, engagement)}?code=${encodeURIComponent(item.label)}`;
    default:
      return null;
  }
}

/** Mutabakat farkının kalem türü (reconciliation `unposted.kind`) → kayıt türü. */
const RECONCILIATION_KIND: Record<string, RelatedSourceKind> = {
  invoice: "invoice",
  bank: "bank_transaction",
  expense: "expense",
  zreport: "retail_sale",
  payroll: "payroll_run",
};

/**
 * Mutabakat farkını oluşturan kaydın adresi (#469); elle girilen kalemin
 * (`manual`) ya da uuid'siz kalemin bağlantısı yok.
 */
export function reconciliationItemHref(
  slug: string,
  engagement: string,
  item: { kind: string; id: string; label: string },
): string | null {
  const kind = RECONCILIATION_KIND[item.kind];
  if (!kind || !item.id) return null;
  return relatedHref(slug, engagement, {
    kind,
    uuid: item.id,
    label: item.label,
  });
}

/** Beyanname kontrol listesi maddesi → ilgili kayıtların sekmesi. */
const CHECKLIST_TAB: [RegExp, keyof typeof CHECKLIST_ROUTES][] = [
  [
    /^(PENDING_EXPENSES|FOREIGN_CURRENCY_EXPENSES|NOT_DEDUCTIBLE_EXPENSES)$/,
    "expenses",
  ],
  [/^(PENDING_Z_REPORTS|Z_WITHOUT_BREAKDOWN)$/, "zreports"],
  [
    /^(NO_PAYROLL|PAYROLL_ISSUES|SGK_ACCRUAL_MISSING|SGK_ACCRUAL_MISMATCH|MISSING_SGK_NO|MISSING_DAYS|MASKED_NATIONAL_IDS|INCOME_WITHHOLDING)$/,
    "payroll",
  ],
  [
    /^(MISSING_COUNTERPARTY_.*|INVALID_COUNTERPARTY_ID|MISSING_EXCHANGE_RATE|DRAFT_INVOICES|HEADER_ESTIMATE|EXPORT_REGISTERED|RESPONSIBLE_VAT|BELOW_THRESHOLD)$/,
    "invoices",
  ],
];

const CHECKLIST_ROUTES = {
  invoices: routes.tenant.engagementInvoices,
  expenses: routes.tenant.engagementExpenses,
  zreports: routes.tenant.engagementZReports,
  payroll: routes.tenant.engagementPayroll,
};

/**
 * Beyanname kontrol listesi maddesinin ilgili kayıtları (#469). Maddeler
 * kayıt kimliği taşımaz; kod, kayıtların bulunduğu sekmeyi belirler.
 * Eşlemesi olmayan madde (ör. NO_DATA, CARRY_UNKNOWN) için null.
 */
export function checklistHref(
  slug: string,
  engagement: string,
  code: string,
): string | null {
  const hit = CHECKLIST_TAB.find(([re]) => re.test(code));
  return hit ? CHECKLIST_ROUTES[hit[1]](slug, engagement) : null;
}

/** Uyarının kayıt türü → mükellef detayındaki kayıt türü (#528). */
const WARNING_KIND: Record<string, RelatedSourceKind> = {
  invoice: "invoice",
  invoice_item: "invoice",
  bank_transaction: "bank_transaction",
  expense: "expense",
  retail_sale: "retail_sale",
  financial_transaction: "financial_transaction",
};

/**
 * Uyarının kaydı ilişkili kayıt türü olarak (#528, yan panel); fiş ve
 * dönem uyarısında null.
 */
export function warningRecordKind(
  entityType: string,
): RelatedSourceKind | null {
  return WARNING_KIND[entityType] ?? null;
}

/**
 * Uyarının işaret ettiği kaydın adresi (#528): fatura (kalem uyarısında
 * faturası) detayına, diğerleri sekmesinde `?kayit=` ile seçili; fişler
 * inceleme kuyruğuna. Kaydı olmayan (dönem) uyarıda null.
 */
export function warningRecordHref(
  slug: string,
  w: {
    engagement_uuid: string;
    entity_type: string;
    entity_uuid?: string | null;
  },
): string | null {
  if (!w.entity_uuid) return null;
  if (w.entity_type === "receipt_capture")
    return routes.tenant.engagementReview(slug, w.engagement_uuid);
  const kind = WARNING_KIND[w.entity_type];
  if (!kind) return null;
  return relatedHref(slug, w.engagement_uuid, {
    kind,
    uuid: w.entity_uuid,
    label: "",
  });
}
