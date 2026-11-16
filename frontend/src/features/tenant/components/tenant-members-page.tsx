"use client";

import { LogOut, UsersRound } from "lucide-react";
import { useRouter } from "next/navigation";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  InviteForm,
  useOfficeRoles,
} from "@/features/tenant/components/invite-form";
import {
  useChangeTenantMemberRole,
  useLeaveTenant,
  useRemoveTenantMember,
  useRevokeTenantInvitation,
  useTenantInvitations,
  useTenantMembers,
} from "@/features/tenant/hooks/use-tenant-admin";
import {
  INVITATION_STATUS_LABEL_KEYS,
  canChangeRole,
  tenantRoleLabel,
  type TenantRoleOption,
} from "@/features/tenant/lib/roles";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { TenantForbidden } from "@/features/tenant/components/tenant-forbidden";
import type {
  TenantMember,
  TenantRole,
} from "@/features/tenant/services/tenant.service";
import { defaultHomeForUser } from "@/lib/auth/types";
import { date } from "@/lib/utils/format";
import { useAuth } from "@/providers/auth-provider";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

function MemberRow({
  member,
  isSelf,
  actorRole,
  canWrite,
  roles,
  roleOptions,
}: {
  member: TenantMember;
  isSelf: boolean;
  actorRole: string;
  canWrite: boolean;
  roles: readonly TenantRole[] | undefined;
  roleOptions: readonly TenantRoleOption[];
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { confirmDelete } = useDialogs();
  const changeRole = useChangeTenantMemberRole(slug);
  const remove = useRemoveTenantMember(slug);
  const name = `${member.user.name} ${member.user.surname}`.trim();
  const current = member.role.key;
  // Options the caller may move this member to (the server enforces the same).
  const options = roleOptions.filter((o) =>
    canChangeRole(actorRole, current, o.key),
  );
  // Keep the member's current role selectable even if the list lacks it.
  if (!options.some((o) => o.key === current)) {
    options.unshift({
      key: current,
      label: tenantRoleLabel(t, current, { roles, fallback: member.role.name }),
    });
  }
  const editable = canWrite && !isSelf && options.length > 1;
  const removable =
    canWrite && !isSelf && canChangeRole(actorRole, current, "");

  const onRemove = async () => {
    const ok = await confirmDelete({
      title: t("tenant.members_remove_title", { name }),
      description: t("tenant.members_remove_description"),
      confirmLabel: t("tenant.members_remove"),
    });
    if (ok) remove.mutate(member.user.uuid);
  };

  return (
    <tr className="border-b last:border-0">
      <td className="py-2 pe-3">
        <div className="font-medium">
          {name}
          {isSelf ? (
            <span className="text-muted-foreground ms-1 text-xs font-normal">
              ({t("tenant.members_you")})
            </span>
          ) : null}
        </div>
        <div className="text-muted-foreground text-xs">{member.user.email}</div>
      </td>
      <td className="py-2 pe-3">
        {editable ? (
          <Select
            value={current}
            disabled={changeRole.isPending}
            onValueChange={(roleKey) =>
              changeRole.mutate({
                userUuid: member.user.uuid,
                roleKey,
              })
            }
          >
            <SelectTrigger
              className="h-8 w-36"
              aria-label={t("tenant.members_role")}
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {options.map((o) => (
                <SelectItem key={o.key} value={o.key}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ) : (
          tenantRoleLabel(t, current, { roles, fallback: member.role.name })
        )}
      </td>
      <td className="text-muted-foreground py-2 pe-3 text-sm">
        {date(member.created_at, "dd.MM.yyyy", locale)}
      </td>
      <td className="py-2 text-end">
        {removable ? (
          <Button
            type="button"
            size="sm"
            variant="ghost"
            disabled={remove.isPending}
            onClick={() => void onRemove()}
          >
            {t("tenant.members_remove")}
          </Button>
        ) : null}
      </td>
    </tr>
  );
}

function InvitationsSection({ canWrite }: { canWrite: boolean }) {
  const officeRoles = useOfficeRoles();
  const { t, locale } = useLocale();
  const { slug, role } = useTenant();
  const query = useTenantInvitations(slug);
  const revoke = useRevokeTenantInvitation(slug);
  const items = query.data?.items ?? [];

  return (
    <EntitySectionCard
      title={t("tenant.invitations_title")}
      badge={query.data?.total}
    >
      <p className="text-muted-foreground mb-4 text-sm">
        {t("tenant.invitations_description")}
      </p>
      {canWrite ? (
        <InviteForm actorRole={role.key} roleOptions={officeRoles.options} />
      ) : null}
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data && items.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("tenant.invitations_empty")}
        </p>
      ) : null}
      {items.length ? (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-muted-foreground border-b text-xs">
                <th className="py-2 pe-3 text-start font-medium">
                  {t("tenant.invitations_email")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("tenant.members_role")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("tenant.invitations_expires")}
                </th>
                <th className="py-2" />
              </tr>
            </thead>
            <tbody>
              {items.map((inv) => (
                <tr key={inv.uuid} className="border-b last:border-0">
                  <td className="py-2 pe-3">
                    <div className="font-medium">{inv.email}</div>
                    <StatusChip
                      label={t(
                        INVITATION_STATUS_LABEL_KEYS[inv.status] ??
                          INVITATION_STATUS_LABEL_KEYS.pending!,
                      )}
                      tone={inv.status === "expired" ? "danger" : "warning"}
                    />
                  </td>
                  <td className="py-2 pe-3">
                    {tenantRoleLabel(t, inv.role.key, {
                      roles: officeRoles.roles,
                      fallback: inv.role.name,
                    })}
                  </td>
                  <td className="text-muted-foreground py-2 pe-3">
                    {date(inv.expires_at, "dd.MM.yyyy", locale)}
                  </td>
                  <td className="py-2 text-end">
                    {/* Only owners may cancel an owner-role invitation. */}
                    {canWrite &&
                    (inv.role.key !== "owner" || role.key === "owner") ? (
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        disabled={revoke.isPending}
                        onClick={() => revoke.mutate(inv.uuid)}
                      >
                        {t("tenant.invitations_revoke")}
                      </Button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

/** `/t/{slug}/members` — members, invitations, leave. */
export function TenantMembersPage() {
  const { t } = useLocale();
  const router = useRouter();
  const tenant = useTenant();
  const { user, hydrateProfile } = useAuth();
  const { can } = usePermission();
  const { confirm } = useDialogs();
  const canWrite = can(permissions.tenant.members.write);
  const membersQuery = useTenantMembers(tenant.slug);
  const officeRoles = useOfficeRoles();
  const leave = useLeaveTenant(tenant.slug);
  const members = membersQuery.data?.items ?? [];

  const onLeave = async () => {
    const ok = await confirm({
      title: t("tenant.members_leave_title", {
        name: tenant.organization.name,
      }),
      description: t("tenant.members_leave_description"),
      confirmLabel: t("tenant.members_leave"),
    });
    if (!ok) return;
    try {
      await leave.mutateAsync();
    } catch {
      return; // shown as a toast (e.g. LAST_ORGANIZATION_OWNER)
    }
    // Membership is gone: go to the shell /me now points to.
    const refreshed = await hydrateProfile();
    router.replace(refreshed ? defaultHomeForUser(refreshed) : routes.cms.home);
  };

  return (
    <EntityPage
      title={t("tenant.members_title")}
      description={t("tenant.members_description")}
      permission={permissions.tenant.members.read}
      forbiddenFallback={<TenantForbidden />}
      breadcrumbs={[
        {
          label: tenant.organization.name,
          href: routes.tenant.home(tenant.slug),
        },
        { label: t("tenant.members_title") },
      ]}
      actions={
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={leave.isPending}
          onClick={() => void onLeave()}
        >
          <LogOut className="size-4" />
          {t("tenant.members_leave")}
        </Button>
      }
    >
      <div className="space-y-6">
        <EntitySectionCard
          title={t("tenant.members_title")}
          badge={membersQuery.data?.total}
        >
          {membersQuery.isLoading ? (
            <Loading label={t("common.loading")} />
          ) : null}
          {membersQuery.isError ? (
            <ErrorState
              title={t("common.error_generic")}
              onRetry={() => void membersQuery.refetch()}
              retryLabel={t("common.retry")}
            />
          ) : null}
          {membersQuery.data && members.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("tenant.members_empty")}
            </p>
          ) : null}
          {members.length ? (
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="text-muted-foreground border-b text-xs">
                    <th className="py-2 pe-3 text-start font-medium">
                      <UsersRound className="me-1 inline size-3.5" />
                      {t("tenant.members_user")}
                    </th>
                    <th className="py-2 pe-3 text-start font-medium">
                      {t("tenant.members_role")}
                    </th>
                    <th className="py-2 pe-3 text-start font-medium">
                      {t("tenant.members_since")}
                    </th>
                    <th className="py-2" />
                  </tr>
                </thead>
                <tbody>
                  {members.map((m) => (
                    <MemberRow
                      key={m.user.uuid}
                      member={m}
                      isSelf={m.user.uuid === user?.uuid}
                      actorRole={tenant.role.key}
                      canWrite={canWrite}
                      roles={officeRoles.roles}
                      roleOptions={officeRoles.options}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          ) : null}
        </EntitySectionCard>

        <InvitationsSection canWrite={canWrite} />
      </div>
    </EntityPage>
  );
}
