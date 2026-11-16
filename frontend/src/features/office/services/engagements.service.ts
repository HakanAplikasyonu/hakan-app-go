import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type EngagementAssignment =
  components["schemas"]["EngagementAssignment"];
type AssignmentList =
  components["schemas"]["EnvelopeEngagementAssignmentList"]["data"];

export const engagementKeys = {
  assignments: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "assignments",
    ] as const,
};

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;

/** Engagement assignments (issue #2): which office members work on a taxpayer. */
export const engagementService = {
  assignments(slug: string, engagement: string) {
    return tenantRequest<AssignmentList>(
      slug,
      "GET",
      `${base(engagement)}/assignments`,
    );
  },
  assign(slug: string, engagement: string, user: string) {
    return tenantRequest<unknown>(
      slug,
      "PUT",
      `${base(engagement)}/assignments/${encodeURIComponent(user)}`,
    );
  },
  unassign(slug: string, engagement: string, user: string) {
    return tenantRequest<unknown>(
      slug,
      "DELETE",
      `${base(engagement)}/assignments/${encodeURIComponent(user)}`,
    );
  },
};
