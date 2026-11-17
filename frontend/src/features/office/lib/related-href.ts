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
