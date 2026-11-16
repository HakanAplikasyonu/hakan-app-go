import type { components } from "@/generated/api";
import { platformRequest } from "@/lib/api/platform-request";
import { tenantRequest } from "@/lib/api/tenant-request";

export type TenantContextResponse = components["schemas"]["TenantContext"];

export const tenantQueryKeys = {
  all: ["tenant"] as const,
  /**
   * Scoped to the signed-in user as well: the payload is that user's role and
   * permissions, so another account on the same tab must never reuse it.
   */
  context: (slug: string, userUuid: string) =>
    ["tenant", userUuid, slug.toLowerCase(), "context"] as const,
};

/**
 * Authoritative membership check for one organization: the API resolves the
 * slug, the caller's membership, status and access window on every call.
 */
export function fetchTenantContext(slug: string) {
  return tenantRequest<TenantContextResponse>(
    slug,
    "GET",
    "/v1/tenant/context",
    { silent: true },
  );
}

// --- Members, invitations, settings (organization admin pages) ---

export type TenantMember = components["schemas"]["OrganizationMember"];
export type TenantMemberPage = components["schemas"]["OrganizationMemberPage"];
export type TenantInvitation = components["schemas"]["TenantInvitation"];
export type TenantInvitationPage =
  components["schemas"]["TenantInvitationPage"];
export type TenantSettings = components["schemas"]["TenantSettings"];
export type InvitationPreview = components["schemas"]["InvitationPreview"];
export type AcceptedInvitation = components["schemas"]["AcceptedInvitation"];
/** Office role key: a template (owner, admin, …) or an office's own role. */
export type TenantRoleKey = string;
export type TenantRole = components["schemas"]["TenantRole"];
export type TenantRoleList = components["schemas"]["TenantRoleList"];
export type TenantPermission = components["schemas"]["TenantPermission"];
export type TenantPermissionList =
  components["schemas"]["TenantPermissionList"];
export type CreateTenantRoleBody =
  components["schemas"]["CreateTenantRoleRequest"];
export type PatchTenantRoleBody =
  components["schemas"]["PatchTenantRoleRequest"];

export const tenantAdminKeys = {
  members: (slug: string) => ["tenant", slug.toLowerCase(), "members"] as const,
  invitations: (slug: string) =>
    ["tenant", slug.toLowerCase(), "invitations"] as const,
  settings: (slug: string) =>
    ["tenant", slug.toLowerCase(), "settings"] as const,
  roles: (slug: string) => ["tenant", slug.toLowerCase(), "roles"] as const,
  rolePermissions: (slug: string) =>
    ["tenant", slug.toLowerCase(), "role-permissions"] as const,
};

export const tenantService = {
  listMembers(slug: string) {
    return tenantRequest<TenantMemberPage>(slug, "GET", "/v1/tenant/members", {
      query: { limit: 100 },
    });
  },
  changeMemberRole(slug: string, userUuid: string, roleKey: TenantRoleKey) {
    return tenantRequest<TenantMember>(
      slug,
      "PATCH",
      `/v1/tenant/members/${userUuid}`,
      { body: { role_key: roleKey } },
    );
  },
  removeMember(slug: string, userUuid: string) {
    return tenantRequest<unknown>(
      slug,
      "DELETE",
      `/v1/tenant/members/${userUuid}`,
    );
  },
  leave(slug: string) {
    return tenantRequest<unknown>(slug, "POST", "/v1/tenant/leave");
  },
  listInvitations(slug: string) {
    return tenantRequest<TenantInvitationPage>(
      slug,
      "GET",
      "/v1/tenant/invitations",
      { query: { limit: 100 } },
    );
  },
  createInvitation(
    slug: string,
    body: { email: string; role_key: TenantRoleKey },
  ) {
    return tenantRequest<TenantInvitation>(
      slug,
      "POST",
      "/v1/tenant/invitations",
      { body },
    );
  },
  revokeInvitation(slug: string, uuid: string) {
    return tenantRequest<unknown>(
      slug,
      "DELETE",
      `/v1/tenant/invitations/${uuid}`,
    );
  },
  getSettings(slug: string) {
    return tenantRequest<TenantSettings>(slug, "GET", "/v1/tenant/settings");
  },
  patchSettings(slug: string, body: { name?: string }) {
    return tenantRequest<TenantSettings>(slug, "PATCH", "/v1/tenant/settings", {
      body,
    });
  },
  /** Office roles: the templates plus the office's own roles. */
  listRoles(slug: string, options?: { silent?: boolean }) {
    return tenantRequest<TenantRoleList>(slug, "GET", "/v1/tenant/roles", {
      silent: options?.silent,
    });
  },
  /** Every `tenant.*` permission a role may hold. */
  listRolePermissions(slug: string) {
    return tenantRequest<TenantPermissionList>(
      slug,
      "GET",
      "/v1/tenant/role-permissions",
    );
  },
  createRole(slug: string, body: CreateTenantRoleBody) {
    return tenantRequest<TenantRole>(slug, "POST", "/v1/tenant/roles", {
      body,
    });
  },
  patchRole(slug: string, roleKey: string, body: PatchTenantRoleBody) {
    return tenantRequest<TenantRole>(
      slug,
      "PATCH",
      `/v1/tenant/roles/${encodeURIComponent(roleKey)}`,
      { body },
    );
  },
  deleteRole(slug: string, roleKey: string) {
    return tenantRequest<unknown>(
      slug,
      "DELETE",
      `/v1/tenant/roles/${encodeURIComponent(roleKey)}`,
    );
  },
};

/**
 * Invitation-link calls: no organization header (the holder is not a member
 * yet). The token travels in the body, never in an API URL.
 */
export const invitationService = {
  /** `silent`: the caller renders a missing invitation itself (no toast). */
  preview(token: string, options?: { silent?: boolean }) {
    return platformRequest<InvitationPreview>(
      "POST",
      "/v1/invitations/preview",
      { body: { token }, silent: options?.silent },
    );
  },
  accept(token: string) {
    return platformRequest<AcceptedInvitation>(
      "POST",
      "/v1/invitations/accept",
      { body: { token } },
    );
  },
};
