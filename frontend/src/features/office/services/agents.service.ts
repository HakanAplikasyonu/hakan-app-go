import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type DesktopAgent = components["schemas"]["DesktopAgent"];
/** A file the agent picked up from a watched folder (#780 e). */
export type AgentDocument = components["schemas"]["AgentDocument"];
export type AgentDocumentKind = Exclude<AgentDocument["kind"], "unknown">;

export const agentKeys = {
  list: (slug: string) => ["tenant", slug.toLowerCase(), "agents"] as const,
  documents: (slug: string) =>
    ["tenant", slug.toLowerCase(), "agent-documents"] as const,
};

/** Desktop agents of the office (issue #285, backend #51). */
export const agentService = {
  list(slug: string) {
    return tenantRequest<{ items: DesktopAgent[] }>(
      slug,
      "GET",
      "/v1/tenant/agents",
    );
  },
  /** Files waiting for the office (needs_match, failed), newest first. */
  documents(slug: string) {
    return tenantRequest<{ items: AgentDocument[]; total: number }>(
      slug,
      "GET",
      "/v1/tenant/agent-documents",
      { query: { status: "open", limit: 100 } },
    );
  },
  resolveDocument(
    slug: string,
    uuid: string,
    body: { engagement_uuid: string; kind: AgentDocumentKind; period?: string },
  ) {
    return tenantRequest<AgentDocument>(
      slug,
      "POST",
      `/v1/tenant/agent-documents/${encodeURIComponent(uuid)}/resolve`,
      { body, silent: true },
    );
  },
  dismissDocument(slug: string, uuid: string) {
    return tenantRequest<AgentDocument>(
      slug,
      "POST",
      `/v1/tenant/agent-documents/${encodeURIComponent(uuid)}/dismiss`,
    );
  },
  revoke(slug: string, uuid: string) {
    return tenantRequest<void>(
      slug,
      "DELETE",
      `/v1/tenant/agents/${encodeURIComponent(uuid)}`,
    );
  },
};
