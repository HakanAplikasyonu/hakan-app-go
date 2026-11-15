import type {
  TenantRole,
  TenantRoleKey,
} from "@/features/tenant/services/tenant.service";

/**
 * Template roles every office starts with (issue #5). Offices may add their
 * own roles on top; pickers use `GET /v1/tenant/roles` and fall back to this
 * list when the caller may not read roles.
 */
export const TENANT_TEMPLATE_ROLE_KEYS = [
  "owner",
  "admin",
  "accountant",
  "member",
  "intern",
] as const;

export type TenantTemplateRoleKey = (typeof TENANT_TEMPLATE_ROLE_KEYS)[number];

/** Static label keys so the i18n linter can verify each one. */
export const TENANT_ROLE_LABEL_KEYS: Record<TenantTemplateRoleKey, string> = {
  owner: "tenant.role_owner",
  admin: "tenant.role_admin",
  accountant: "tenant.role_accountant",
  member: "tenant.role_member",
  intern: "tenant.role_intern",
};

/** Key pattern the API enforces for office roles. */
export const TENANT_ROLE_KEY_PATTERN = /^[a-z][a-z0-9_]{1,31}$/;

/** Default role of a new invitation. */
export const DEFAULT_INVITE_ROLE_KEY: TenantRoleKey = "member";

export const INVITATION_STATUS_LABEL_KEYS: Record<string, string> = {
  pending: "tenant.invitations_status_pending",
  expired: "tenant.invitations_status_expired",
};

/** True for a template role key (one with a static i18n label). */
export function isTenantRoleKey(v: string): v is TenantTemplateRoleKey {
  return (TENANT_TEMPLATE_ROLE_KEYS as readonly string[]).includes(v);
}

/** A picker option: role key plus the label to show. */
export type TenantRoleOption = { key: TenantRoleKey; label: string };

/**
 * Label of a role key. The office's own role name wins (offices may rename
 * templates); otherwise templates use their i18n label and anything else the
 * fallback the API sent alongside the key.
 */
export function tenantRoleLabel(
  t: (k: string) => string,
  key: string,
  options?: { roles?: readonly TenantRole[]; fallback?: string },
) {
  const fromOffice = options?.roles?.find((r) => r.key === key)?.name;
  if (fromOffice) return fromOffice;
  if (isTenantRoleKey(key)) return t(TENANT_ROLE_LABEL_KEYS[key]);
  return options?.fallback || key;
}

/**
 * Roles a picker offers: the office's roles when loaded, the templates
 * otherwise (loading, or the caller lacks `tenant.roles.read`).
 */
export function tenantRoleOptions(
  t: (k: string) => string,
  roles: readonly TenantRole[] | undefined,
): TenantRoleOption[] {
  if (roles?.length) {
    return roles.map((r) => ({ key: r.key, label: r.name || r.key }));
  }
  return TENANT_TEMPLATE_ROLE_KEYS.map((key) => ({
    key,
    label: t(TENANT_ROLE_LABEL_KEYS[key]),
  }));
}

/**
 * Mirrors the API's owner-escalation rule (orgusecase.CheckRoleChange) so the
 * UI hides what the server would refuse: only owners hand out, take away or
 * remove the owner role.
 */
export function canChangeRole(actor: string, from: string, to: string) {
  if (actor === "owner") return true;
  return from !== "owner" && to !== "owner";
}
