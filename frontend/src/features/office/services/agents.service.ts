import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type DesktopAgent = components["schemas"]["DesktopAgent"];

export const agentKeys = {
  list: (slug: string) => ["tenant", slug.toLowerCase(), "agents"] as const,
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
  revoke(slug: string, uuid: string) {
    return tenantRequest<void>(
      slug,
      "DELETE",
      `/v1/tenant/agents/${encodeURIComponent(uuid)}`,
    );
  },
};
