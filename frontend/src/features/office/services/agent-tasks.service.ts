import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type AgentTask = components["schemas"]["AgentTask"];
export type AgentTaskPage = components["schemas"]["AgentTaskPage"];
export type AgentTaskRun = components["schemas"]["AgentTaskRun"];
export type AgentTaskFile = components["schemas"]["AgentTaskFile"];
export type AgentTaskFileKind = components["schemas"]["AgentTaskFileKind"];
export type AgentTaskStatus = components["schemas"]["AgentTaskStatus"];
export type AgentTaskCounterparty =
  components["schemas"]["AgentTaskCounterparty"];
export type AgentTaskReportRow = components["schemas"]["AgentTaskReportRow"];
export type DesktopHelperSettings =
  components["schemas"]["DesktopHelperSettings"];
export type DesktopHelperSettingsInput =
  components["schemas"]["DesktopHelperSettingsInput"];
export type LucaExportInput = components["schemas"]["LucaExportInput"];
export type LucaHelperConsent = components["schemas"]["LucaHelperConsent"];
export type LucaHelperRunner = components["schemas"]["LucaHelperRunner"];

/** Yükleme sırası: banka, Z, fatura, masraf, fiş (sunucuyla aynı). */
export const AGENT_FILE_KINDS: AgentTaskFileKind[] = [
  "bank",
  "z",
  "incoming_invoices",
  "outgoing_invoices",
  "expenses",
  "journals",
];

/** Masaüstü yardımcısının henüz bitirmediği durumlar. */
export const OPEN_TASK_STATUSES = new Set<AgentTaskStatus>([
  "queued",
  "claimed",
  "waiting_user",
  "running",
]);

const root = "/v1/tenant/agent-tasks";
const task = (id: string) => `${root}/${encodeURIComponent(id)}`;

export const agentTaskKeys = {
  all: (slug: string) => ["tenant", slug.toLowerCase(), "agent-tasks"] as const,
  settings: (slug: string) =>
    ["tenant", slug.toLowerCase(), "desktop-helper-settings"] as const,
};

/**
 * Masaüstü yardımcısı görevleri: kullanıcının kendi Luca oturumunda tek
 * tıkla aktarım (cari eşitleme + banka, Z, fatura, masraf, fiş dosyaları).
 */
export const agentTaskService = {
  settings(slug: string) {
    return tenantRequest<DesktopHelperSettings>(
      slug,
      "GET",
      "/v1/tenant/desktop-helper-settings",
    );
  },
  saveSettings(slug: string, input: DesktopHelperSettingsInput) {
    return tenantRequest<DesktopHelperSettings>(
      slug,
      "PUT",
      "/v1/tenant/desktop-helper-settings",
      { body: input },
    );
  },
  list(slug: string) {
    return tenantRequest<AgentTaskPage>(slug, "GET", root, {
      query: { limit: 100 },
    });
  },
  startLucaExport(slug: string, input: LucaExportInput) {
    return tenantRequest<AgentTaskRun>(slug, "POST", `${root}/luca-export`, {
      body: input,
    });
  },
  approve(slug: string, id: string) {
    return tenantRequest<AgentTask>(slug, "POST", `${task(id)}/approve`);
  },
  cancel(slug: string, id: string) {
    return tenantRequest<AgentTask>(slug, "POST", `${task(id)}/cancel`);
  },
  resolve(slug: string, id: string, imported: boolean) {
    return tenantRequest<AgentTask>(slug, "POST", `${task(id)}/resolve`, {
      body: { imported },
    });
  },
};
