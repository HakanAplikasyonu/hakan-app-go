import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";
import { allActiveEngagements } from "@/features/office/services/office-invoices.service";

export type AutomationOverview = components["schemas"]["AutomationOverview"];
export type AutomationRule = components["schemas"]["AutomationRule"];
export type AutomationRuleInput = components["schemas"]["AutomationRuleInput"];
export type AutomationTrigger = components["schemas"]["AutomationTrigger"];
export type AutomationRun = components["schemas"]["AutomationRun"];
export type AutomationHistory = components["schemas"]["AutomationHistory"];
export type AutomationEvent = components["schemas"]["AutomationEvent"];
export type AutomationDecideResult =
  components["schemas"]["AutomationDecideResult"];
export type AutomationRunStatus = AutomationRun["status"];
export type EngagementPage = components["schemas"]["EngagementPage"];

export const automationKeys = {
  all: (slug: string) => ["tenant", slug.toLowerCase(), "automations"] as const,
  runs: (slug: string, status: string) =>
    ["tenant", slug.toLowerCase(), "automations", "runs", status] as const,
  history: (slug: string, uuid: string) =>
    ["tenant", slug.toLowerCase(), "automations", "history", uuid] as const,
  engagements: (slug: string) =>
    ["tenant", slug.toLowerCase(), "automations", "engagements"] as const,
};

/**
 * Otomasyon kuralları (#780 C, İletişim merkezi): kurallar, ana anahtar,
 * gönderim geçmişi ve onay bekleyen taslaklar.
 */
export const automationService = {
  overview(slug: string) {
    return tenantRequest<AutomationOverview>(
      slug,
      "GET",
      "/v1/tenant/automations",
      { silent: true },
    );
  },
  create(slug: string, body: AutomationRuleInput) {
    return tenantRequest<AutomationRule>(
      slug,
      "POST",
      "/v1/tenant/automations",
      { body },
    );
  },
  update(slug: string, uuid: string, body: AutomationRuleInput) {
    return tenantRequest<AutomationRule>(
      slug,
      "PUT",
      `/v1/tenant/automations/${encodeURIComponent(uuid)}`,
      { body },
    );
  },
  remove(slug: string, uuid: string) {
    return tenantRequest<void>(
      slug,
      "DELETE",
      `/v1/tenant/automations/${encodeURIComponent(uuid)}`,
    );
  },
  setActive(slug: string, uuid: string, active: boolean) {
    return tenantRequest<AutomationRule>(
      slug,
      "PUT",
      `/v1/tenant/automations/${encodeURIComponent(uuid)}/active`,
      { body: { active } },
    );
  },
  history(slug: string, uuid: string) {
    return tenantRequest<AutomationHistory>(
      slug,
      "GET",
      `/v1/tenant/automations/${encodeURIComponent(uuid)}/history`,
      { silent: true },
    );
  },
  setEnabled(slug: string, enabled: boolean) {
    return tenantRequest<{ enabled: boolean }>(
      slug,
      "PUT",
      "/v1/tenant/automation-settings",
      { body: { enabled } },
    );
  },
  runs(slug: string, status?: AutomationRunStatus, limit = 100) {
    const query: Record<string, string> = { limit: String(limit) };
    if (status) query.status = status;
    return tenantRequest<{ items: AutomationRun[] }>(
      slug,
      "GET",
      "/v1/tenant/automation-runs",
      { query, silent: true },
    );
  },
  decide(slug: string, uuids: string[], approve: boolean) {
    return tenantRequest<AutomationDecideResult>(
      slug,
      "POST",
      "/v1/tenant/automation-runs/decide",
      { body: { uuids, approve } },
    );
  },
  engagements(slug: string): Promise<EngagementPage> {
    // Bütün sayfalar: 100'den çok mükellefte eskiler de seçilebilsin.
    return allActiveEngagements(slug, { silent: true });
  },
};
