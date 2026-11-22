import { Permission, permissions } from "@/config/permissions";
import {
  DRAFTS_ANCHOR,
  EXPORT_ANCHOR,
  EXPORT_HISTORY_ANCHOR,
  routes,
  VAT_NOTICE_ANCHOR,
} from "@/config/routes";

import type {
  CopilotAction,
  CopilotStepKey,
} from "../services/copilot.service";

/**
 * Plan-step actions: they change data and need the user's approval. Taslak
 * fiş onayı ve aktarım burada değil (#566): ikisi de önce kontrol ekranına
 * götürür; engelli fiş onaylanmaz, aktarılacaklar orada seçilir.
 */
export const WRITE_ACTIONS = new Set<CopilotAction>([
  "bank_suggest",
  "bank_approve",
  "bank_drafts",
  "report_send",
]);

export function isWriteAction(a: CopilotAction | null | undefined) {
  return !!a && WRITE_ACTIONS.has(a);
}

/** Permissions each action needs (the backend checks them too). */
export const ACTION_PERMISSIONS: Record<CopilotAction, string[]> = {
  bank_suggest: [Permission.TenantBankWrite],
  bank_approve: [Permission.TenantBankWrite],
  bank_drafts: [Permission.TenantBankWrite, permissions.tenant.journals.write],
  journals_approve: [permissions.tenant.journals.approve],
  export_transfer: [permissions.tenant.journals.export],
  report_send: [permissions.tenant.engagements.write],
  open_documents: [],
  open_review: [],
  open_journals: [],
  open_expenses: [],
  open_z_reports: [],
  open_export: [],
  open_payroll: [],
  open_trial_balance: [],
  open_reconciliation: [],
  open_vat: [],
  open_bank: [],
};

/** Screen an action opens (navigation actions, and "go and look"). */
export function actionHref(
  slug: string,
  action: CopilotAction,
  engagement: string,
  /**
   * Mükellef detayından açılınca inceleme ve fişler mükellef sekmesinde
   * açılır; sekme çubuğu kaybolmaz (#468).
   */
  inEngagement = false,
): string {
  switch (action) {
    // Eksik belge: dosya listesi değil, açık belge istekleri (#563).
    // Ofis listesinden: belge toplama ekranı, mükellefe süzülü (#800);
    // mükellef sayfasında açık belge istekleri kalır.
    case "open_documents":
      return inEngagement
        ? routes.tenant.engagementDocTasks(slug, engagement)
        : routes.tenant.documentCollection(slug, undefined, engagement);
    case "open_review":
      return inEngagement
        ? routes.tenant.engagementReview(slug, engagement)
        : `${routes.tenant.review(slug)}?taxpayer=${encodeURIComponent(engagement)}`;
    case "bank_suggest":
    case "bank_approve":
    // İşletme defteri: banka ekranında yalnız ekstre kontrolü.
    case "open_bank":
      return routes.tenant.engagementBank(slug, engagement);
    case "report_send":
      return routes.tenant.engagementReport(slug, engagement);
    // Ay sonu kapanış (#780): her adım işin yapıldığı mükellef sekmesine.
    case "open_expenses":
      return routes.tenant.engagementExpenses(slug, engagement);
    case "open_z_reports":
      return routes.tenant.engagementZReports(slug, engagement);
    case "open_payroll":
      return routes.tenant.engagementPayroll(slug, engagement);
    case "open_trial_balance":
      return routes.tenant.engagementTrialBalance(slug, engagement);
    case "open_reconciliation":
      return routes.tenant.engagementReconciliation(slug, engagement);
    case "open_vat":
      return `${routes.tenant.engagementReport(slug, engagement)}#${VAT_NOTICE_ANCHOR}`;
    case "open_export": {
      const base = inEngagement
        ? routes.tenant.engagementJournals(slug, engagement)
        : `${routes.tenant.journals(slug)}?taxpayer=${encodeURIComponent(engagement)}`;
      return `${base}#${EXPORT_HISTORY_ANCHOR}`;
    }
    case "journals_approve":
    case "export_transfer": {
      const anchor =
        action === "journals_approve" ? DRAFTS_ANCHOR : EXPORT_ANCHOR;
      const base = inEngagement
        ? routes.tenant.engagementJournals(slug, engagement)
        : `${routes.tenant.journals(slug)}?taxpayer=${encodeURIComponent(engagement)}`;
      return `${base}#${anchor}`;
    }
    default:
      return inEngagement
        ? routes.tenant.engagementJournals(slug, engagement)
        : `${routes.tenant.journals(slug)}?taxpayer=${encodeURIComponent(engagement)}`;
  }
}

/** Screen actions whose screen is the step's own (#780). */
const STEP_SCREEN_ACTIONS = new Set<CopilotAction>([
  "open_expenses",
  "open_z_reports",
  "open_export",
  "open_payroll",
  "open_trial_balance",
  "open_reconciliation",
  "open_vat",
  "open_bank",
]);

/**
 * Ekranı bir zincir adımının: adım rozeti oraya götürür (#562). Adımın
 * işi belli bir ekrandaysa (onay bekleyen masraf, Luca mizanı …) oraya
 * (#780).
 */
export function stepHref(
  slug: string,
  key: CopilotStepKey,
  engagement: string,
  inEngagement = false,
  action?: CopilotAction | null,
  /** Adımın işi tek bir kayıtsa (en eski açık belge isteği) onun kimliği. */
  target?: string | null,
  /** İşletme / serbest meslek defteri: fişler adımı masraf fişleridir. */
  singleEntry = false,
): string {
  if (action && STEP_SCREEN_ACTIONS.has(action))
    return actionHref(slug, action, engagement, inEngagement);
  if (singleEntry && key === "journals")
    return routes.tenant.engagementExpenses(slug, engagement);
  switch (key) {
    // Rozet bağlantısı açık belge isteğinin kendisine gider (satır vurgulu);
    // ofis listesindeki "Eksik belgelere bak" düğmesi o mükellefe süzülü
    // belge toplamaya gider (#800).
    case "documents":
      return target
        ? routes.tenant.engagementDocTask(slug, engagement, target)
        : routes.tenant.engagementDocTasks(slug, engagement);
    case "bank":
      return routes.tenant.engagementBank(slug, engagement);
    case "review":
      return actionHref(slug, "open_review", engagement, inEngagement);
    case "report":
      return routes.tenant.engagementReport(slug, engagement);
    case "luca_import":
      return routes.tenant.engagementTrialBalance(slug, engagement);
    case "vat":
      return actionHref(slug, "open_vat", engagement, inEngagement);
    default:
      return actionHref(slug, "open_journals", engagement, inEngagement);
  }
}

/** A step to approve and run: from a plan, a row or a bulk selection. */
export type RunnableStep = {
  index: number;
  action: CopilotAction;
  engagement_uuid: string;
  taxpayer_title: string;
  period: string;
  driver?: "luca" | "zirve";
};
