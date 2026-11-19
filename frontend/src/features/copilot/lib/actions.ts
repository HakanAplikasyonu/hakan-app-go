import { Permission, permissions } from "@/config/permissions";
import { routes } from "@/config/routes";

import type { CopilotAction } from "../services/copilot.service";

/** Plan-step actions: they change data and need the user's approval. */
export const WRITE_ACTIONS = new Set<CopilotAction>([
  "bank_suggest",
  "bank_approve",
  "bank_drafts",
  "journals_approve",
  "export_transfer",
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
  journals_approve: [permissions.tenant.journals.write],
  export_transfer: [permissions.tenant.journals.export],
  report_send: [permissions.tenant.engagements.write],
  open_documents: [],
  open_review: [],
  open_journals: [],
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
    case "open_documents":
      return routes.tenant.engagementDocTasks(slug, engagement);
    case "open_review":
      return inEngagement
        ? routes.tenant.engagementReview(slug, engagement)
        : `${routes.tenant.review(slug)}?taxpayer=${encodeURIComponent(engagement)}`;
    case "bank_suggest":
    case "bank_approve":
      return routes.tenant.engagementBank(slug, engagement);
    case "report_send":
      return routes.tenant.engagementReport(slug, engagement);
    default:
      return inEngagement
        ? routes.tenant.engagementJournals(slug, engagement)
        : `${routes.tenant.journals(slug)}?taxpayer=${encodeURIComponent(engagement)}`;
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
