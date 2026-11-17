"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { LogOut } from "lucide-react";
import { useRouter } from "next/navigation";
import { useMemo } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusChip } from "@/components/common/status-chip";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import {
  DataTable,
  createActionsColumn,
  createColumn,
  createDateColumn,
} from "@/components/tables";
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
  TenantInvitation,
  TenantMember,
  TenantRole,
} from "@/features/tenant/services/tenant.service";
import { defaultHomeForUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** Gömülü küçük liste: araç çubuğu, sayfalama, seçim yok; sıralama açık. */
const EMBEDDED_FEATURES = {
  toolbar: false,
  pagination: false,
  rowSelection: false,
  columnFilters: false,
  globalFilter: false,
  facetedFilters: false,
  viewMode: false,
  columnVisibility: false,
  columnResizing: false,
  columnPinning: false,
  columnOrdering: false,
  density: false,
} as const;

type MemberRowContext = {
  selfUuid: string | undefined;
  actorRole: string;
  canWrite: boolean;
  roles: readonly TenantRole[] | undefined;
  roleOptions: readonly TenantRoleOption[];
};

function memberName(member: TenantMember) {
  return `${member.user.name} ${member.user.surname}`.trim();
}

function MemberRoleCell({
  member,
  ctx,
}: {
  member: TenantMember;
  ctx: MemberRowContext;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const changeRole = useChangeTenantMemberRole(slug);
  const { actorRole, canWrite, roles, roleOptions } = ctx;
  const isSelf = member.user.uuid === ctx.selfUuid;
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

  if (!editable) {
    return <>{tenantRoleLabel(t, current, { roles, fallback: member.role.name })}</>;
  }
  return (
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
      <SelectTrigger className="h-8 w-36" aria-label={t("tenant.members_role")}>
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
  );
}

function MemberRemoveButton({
  member,
  ctx,
}: {
  member: TenantMember;
  ctx: MemberRowContext;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { confirmDelete } = useDialogs();
  const remove = useRemoveTenantMember(slug);
  const isSelf = member.user.uuid === ctx.selfUuid;
  const removable =
    ctx.canWrite &&
    !isSelf &&
    canChangeRole(ctx.actorRole, member.role.key, "");
  if (!removable) return null;

  const onRemove = async () => {
    const ok = await confirmDelete({
      title: t("tenant.members_remove_title", { name: memberName(member) }),
      description: t("tenant.members_remove_description"),
      confirmLabel: t("tenant.members_remove"),
    });
    if (ok) remove.mutate(member.user.uuid);
  };

  return (
    <Button
      type="button"
      size="sm"
      variant="ghost"
      disabled={remove.isPending}
      onClick={() => void onRemove()}
    >
      {t("tenant.members_remove")}
    </Button>
  );
}

function MembersTable({
  members,
  ctx,
}: {
  members: TenantMember[];
  ctx: MemberRowContext;
}) {
  const { t } = useLocale();
  const columns = useMemo<ColumnDef<TenantMember, unknown>[]>(
    () => [
      createColumn<TenantMember>({
        id: "user",
        labelKey: "tenant.members_user",
        accessorFn: memberName,
        gridPrimary: true,
        cell: ({ row }) => {
          const member = row.original;
          return (
            <>
              <div className="font-medium">
                {memberName(member)}
                {member.user.uuid === ctx.selfUuid ? (
                  <span className="text-muted-foreground ms-1 text-xs font-normal">
                    ({t("tenant.members_you")})
                  </span>
                ) : null}
              </div>
              <div className="text-muted-foreground text-xs">
                {member.user.email}
              </div>
            </>
          );
        },
      }),
      createColumn<TenantMember>({
        id: "role",
        labelKey: "tenant.members_role",
        accessorFn: (m) => m.role.key,
        cell: ({ row }) => <MemberRoleCell member={row.original} ctx={ctx} />,
      }),
      createDateColumn<TenantMember>({
        id: "created_at",
        labelKey: "tenant.members_since",
        accessor: (m) => m.created_at,
        className: "text-muted-foreground",
      }),
      createActionsColumn<TenantMember>({
        cell: (member) => <MemberRemoveButton member={member} ctx={ctx} />,
      }),
    ],
    [ctx, t],
  );

  return (
    <DataTable<TenantMember>
      columns={columns}
      data={members}
      getRowId={(m) => m.user.uuid}
      bordered={false}
      features={EMBEDDED_FEATURES}
    />
  );
}

function InvitationsSection({ canWrite }: { canWrite: boolean }) {
  const officeRoles = useOfficeRoles();
  const { t } = useLocale();
  const { slug, role } = useTenant();
  const query = useTenantInvitations(slug);
  const revoke = useRevokeTenantInvitation(slug);
  const items = query.data?.items ?? [];

  const columns = useMemo<ColumnDef<TenantInvitation, unknown>[]>(
    () => [
      createColumn<TenantInvitation>({
        id: "email",
        labelKey: "tenant.invitations_email",
        accessorKey: "email",
        gridPrimary: true,
        cell: ({ row }) => {
          const inv = row.original;
          return (
            <>
              <div className="font-medium">{inv.email}</div>
              <StatusChip
                label={t(
                  INVITATION_STATUS_LABEL_KEYS[inv.status] ??
                    INVITATION_STATUS_LABEL_KEYS.pending!,
                )}
                tone={inv.status === "expired" ? "danger" : "warning"}
              />
            </>
          );
        },
      }),
      createColumn<TenantInvitation>({
        id: "role",
        labelKey: "tenant.members_role",
        accessorFn: (inv) => inv.role.key,
        cell: ({ row }) =>
          tenantRoleLabel(t, row.original.role.key, {
            roles: officeRoles.roles,
            fallback: row.original.role.name,
          }),
      }),
      createDateColumn<TenantInvitation>({
        id: "expires_at",
        labelKey: "tenant.invitations_expires",
        accessor: (inv) => inv.expires_at,
        className: "text-muted-foreground",
      }),
      createActionsColumn<TenantInvitation>({
        // Only owners may cancel an owner-role invitation.
        cell: (inv) =>
          canWrite && (inv.role.key !== "owner" || role.key === "owner") ? (
            <Button
              type="button"
              size="sm"
              variant="ghost"
              disabled={revoke.isPending}
              onClick={() => revoke.mutate(inv.uuid)}
            >
              {t("tenant.invitations_revoke")}
            </Button>
          ) : null,
      }),
    ],
    [canWrite, officeRoles.roles, revoke, role.key, t],
  );

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
        <DataTable<TenantInvitation>
          columns={columns}
          data={items}
          getRowId={(inv) => inv.uuid}
          bordered={false}
          features={EMBEDDED_FEATURES}
        />
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
  const memberCtx = useMemo<MemberRowContext>(
    () => ({
      selfUuid: user?.uuid,
      actorRole: tenant.role.key,
      canWrite,
      roles: officeRoles.roles,
      roleOptions: officeRoles.options,
    }),
    [
      user?.uuid,
      tenant.role.key,
      canWrite,
      officeRoles.roles,
      officeRoles.options,
    ],
  );

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
            <MembersTable members={members} ctx={memberCtx} />
          ) : null}
        </EntitySectionCard>

        <InvitationsSection canWrite={canWrite} />
      </div>
    </EntityPage>
  );
}
