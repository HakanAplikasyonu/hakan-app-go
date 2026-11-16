"use client";

import { Pencil, Plus, ShieldCheck, Trash2 } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { Controller, useFormContext } from "react-hook-form";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage, EntitySectionCard } from "@/components/entity";
import { AppForm, AppInput, AppTextarea } from "@/components/forms";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import {
  useCreateTenantRole,
  useDeleteTenantRole,
  usePatchTenantRole,
  useTenantRolePermissions,
  useTenantRoles,
} from "@/features/tenant/hooks/use-tenant-admin";
import { TENANT_ROLE_KEY_PATTERN } from "@/features/tenant/lib/roles";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { TenantForbidden } from "@/features/tenant/components/tenant-forbidden";
import type {
  TenantPermission,
  TenantRole,
} from "@/features/tenant/services/tenant.service";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** The owner role always holds every permission (the API refuses edits). */
const OWNER_ROLE_KEY = "owner";

type RoleFormValues = {
  key: string;
  name: string;
  description: string;
  permissions: string[];
};

type DialogState = { mode: "create" } | { mode: "edit"; role: TenantRole };

function PermissionChecklist({
  items,
  disabled,
  canGrant,
}: {
  items: readonly TenantPermission[];
  disabled: boolean;
  /** Permissions the user may add; the API refuses others (issue #167). */
  canGrant: (slug: string) => boolean;
}) {
  const { control } = useFormContext<RoleFormValues>();
  return (
    <Controller
      name="permissions"
      control={control}
      render={({ field }) => {
        const selected = new Set(field.value);
        const toggle = (slug: string, on: boolean) => {
          const next = new Set(selected);
          if (on) next.add(slug);
          else next.delete(slug);
          field.onChange(items.map((p) => p.slug).filter((s) => next.has(s)));
        };
        return (
          <ul className="max-h-72 space-y-1 overflow-y-auto rounded-md border p-2">
            {items.map((p) => {
              const id = `tenant-role-perm-${p.slug}`;
              return (
                <li key={p.slug} className="flex items-start gap-2 py-1">
                  <Checkbox
                    id={id}
                    checked={selected.has(p.slug)}
                    disabled={
                      disabled || (!selected.has(p.slug) && !canGrant(p.slug))
                    }
                    onCheckedChange={(v) => toggle(p.slug, v === true)}
                  />
                  <label
                    htmlFor={id}
                    className="grid cursor-pointer gap-0.5 text-sm leading-none"
                  >
                    <span>{p.name}</span>
                    <span className="text-muted-foreground font-mono text-xs">
                      {p.slug}
                    </span>
                  </label>
                </li>
              );
            })}
          </ul>
        );
      }}
    />
  );
}

function RoleDialog({
  state,
  onClose,
}: {
  state: DialogState;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const catalog = useTenantRolePermissions(slug);
  const create = useCreateTenantRole(slug);
  const patch = usePatchTenantRole(slug);
  const editing = state.mode === "edit" ? state.role : null;
  const isOwner = editing?.key === OWNER_ROLE_KEY;
  // Only permissions the user holds can be added; ones the role already has
  // may stay or be removed (issue #167).
  const tenant = useTenant();
  const canGrant = useCallback(
    (perm: string) =>
      tenant.role.key === OWNER_ROLE_KEY ||
      tenant.permissions.includes(perm) ||
      (editing?.permissions.includes(perm) ?? false),
    [tenant.role.key, tenant.permissions, editing],
  );

  const schema = useMemo(
    () =>
      z.object({
        key: editing
          ? z.string()
          : z
              .string()
              .trim()
              .regex(TENANT_ROLE_KEY_PATTERN, t("tenant.roles_validation_key")),
        name: z
          .string()
          .trim()
          .min(1, t("tenant.roles_validation_name"))
          .max(100),
        description: z.string().trim().max(500),
        permissions: z.array(z.string()),
      }),
    [editing, t],
  );

  const onSubmit = async (values: RoleFormValues) => {
    const description = values.description.trim();
    if (editing) {
      await patch.mutateAsync({
        roleKey: editing.key,
        body: {
          name: values.name.trim(),
          description,
          // Owner permissions are fixed; the API refuses a change.
          ...(isOwner ? {} : { permissions: values.permissions }),
        },
      });
    } else {
      await create.mutateAsync({
        key: values.key.trim(),
        name: values.name.trim(),
        ...(description ? { description } : {}),
        permissions: values.permissions,
      });
    }
    onClose();
  };

  const pending = create.isPending || patch.isPending;
  const items = catalog.data?.items ?? [];

  return (
    <Dialog open onOpenChange={(open) => (open ? undefined : onClose())}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {editing
              ? t("tenant.roles_edit_title", { name: editing.name })
              : t("tenant.roles_create_title")}
          </DialogTitle>
          <DialogDescription>
            {t("tenant.roles_dialog_description")}
          </DialogDescription>
        </DialogHeader>
        <AppForm
          id="tenant-role-form"
          schema={schema}
          defaultValues={{
            key: editing?.key ?? "",
            name: editing?.name ?? "",
            description: editing?.description ?? "",
            permissions: editing?.permissions ?? [],
          }}
          onSubmit={onSubmit}
          className="space-y-4"
        >
          {editing ? null : (
            <AppInput
              name="key"
              label={t("tenant.roles_key")}
              description={t("tenant.roles_key_hint")}
              autoComplete="off"
              spellCheck={false}
            />
          )}
          <AppInput
            name="name"
            label={t("tenant.roles_name")}
            autoComplete="off"
          />
          <AppTextarea
            name="description"
            label={t("tenant.roles_description_label")}
            rows={2}
          />
          <div className="space-y-2">
            <div className="text-sm font-medium">
              {t("tenant.roles_permissions")}
            </div>
            {isOwner ? (
              <p className="text-muted-foreground text-xs">
                {t("tenant.roles_owner_locked")}
              </p>
            ) : null}
            {catalog.isLoading ? <Loading label={t("common.loading")} /> : null}
            {catalog.isError ? (
              <ErrorState
                title={t("common.error_generic")}
                onRetry={() => void catalog.refetch()}
                retryLabel={t("common.retry")}
              />
            ) : null}
            {items.length ? (
              <>
                <PermissionChecklist
                  items={items}
                  disabled={isOwner}
                  canGrant={canGrant}
                />
                {isOwner ? null : (
                  <p className="text-muted-foreground text-xs">
                    {t("tenant.roles_grant_hint")}
                  </p>
                )}
              </>
            ) : null}
          </div>
        </AppForm>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>
            {t("common.cancel")}
          </Button>
          <Button
            type="submit"
            form="tenant-role-form"
            disabled={pending || catalog.isLoading}
          >
            {editing ? t("tenant.roles_save") : t("tenant.roles_create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function RoleRow({
  role,
  canWrite,
  onEdit,
}: {
  role: TenantRole;
  canWrite: boolean;
  onEdit: (role: TenantRole) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { confirmDelete } = useDialogs();
  const remove = useDeleteTenantRole(slug);
  const inUse = role.member_count > 0;

  const onDelete = async () => {
    const ok = await confirmDelete({
      title: t("tenant.roles_delete_title", { name: role.name }),
      description: t("tenant.roles_delete_description"),
      confirmLabel: t("tenant.roles_delete"),
    });
    if (ok) remove.mutate(role.key);
  };

  return (
    <tr className="border-b last:border-0">
      <td className="py-2 pe-3">
        <div className="flex flex-wrap items-center gap-2 font-medium">
          {role.name}
          {role.is_system ? (
            <Badge variant="secondary">{t("tenant.roles_system")}</Badge>
          ) : null}
        </div>
        <div className="text-muted-foreground font-mono text-xs">
          {role.key}
        </div>
        {role.description ? (
          <div className="text-muted-foreground mt-0.5 text-xs">
            {role.description}
          </div>
        ) : null}
      </td>
      <td className="py-2 pe-3 tabular-nums">{role.permissions.length}</td>
      <td className="py-2 pe-3 tabular-nums">{role.member_count}</td>
      <td className="py-2 text-end whitespace-nowrap">
        {canWrite ? (
          <>
            <Button
              type="button"
              size="sm"
              variant="ghost"
              onClick={() => onEdit(role)}
            >
              <Pencil className="size-4" />
              {t("tenant.roles_edit")}
            </Button>
            {role.is_system ? null : (
              <Button
                type="button"
                size="sm"
                variant="ghost"
                disabled={remove.isPending || inUse}
                title={inUse ? t("tenant.roles_delete_in_use") : undefined}
                onClick={() => void onDelete()}
              >
                <Trash2 className="size-4" />
                {t("tenant.roles_delete")}
              </Button>
            )}
          </>
        ) : null}
      </td>
    </tr>
  );
}

function RolesSection({
  canWrite,
  onEdit,
}: {
  canWrite: boolean;
  onEdit: (role: TenantRole) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const query = useTenantRoles(slug);
  const roles = query.data?.items ?? [];

  return (
    <EntitySectionCard title={t("tenant.roles_title")} badge={roles.length}>
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data && roles.length === 0 ? (
        <p className="text-muted-foreground text-sm">
          {t("tenant.roles_empty")}
        </p>
      ) : null}
      {roles.length ? (
        <div className="overflow-x-auto">
          <table className="w-full text-sm">
            <thead>
              <tr className="text-muted-foreground border-b text-xs">
                <th className="py-2 pe-3 text-start font-medium">
                  <ShieldCheck className="me-1 inline size-3.5" />
                  {t("tenant.roles_role")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("tenant.roles_permission_count")}
                </th>
                <th className="py-2 pe-3 text-start font-medium">
                  {t("tenant.roles_member_count")}
                </th>
                <th className="py-2" />
              </tr>
            </thead>
            <tbody>
              {roles.map((role) => (
                <RoleRow
                  key={role.key}
                  role={role}
                  canWrite={canWrite}
                  onEdit={onEdit}
                />
              ))}
            </tbody>
          </table>
        </div>
      ) : null}
    </EntitySectionCard>
  );
}

/** `/t/{slug}/roles` — the office's roles: templates plus its own (#5). */
export function TenantRolesPage() {
  const { t } = useLocale();
  const tenant = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.roles.write);
  const [dialog, setDialog] = useState<DialogState | null>(null);

  return (
    <EntityPage
      title={t("tenant.roles_title")}
      description={t("tenant.roles_page_description")}
      permission={permissions.tenant.roles.read}
      forbiddenFallback={<TenantForbidden />}
      breadcrumbs={[
        {
          label: tenant.organization.name,
          href: routes.tenant.home(tenant.slug),
        },
        { label: t("tenant.roles_title") },
      ]}
      actions={
        canWrite ? (
          <Button
            type="button"
            size="sm"
            onClick={() => setDialog({ mode: "create" })}
          >
            <Plus className="size-4" />
            {t("tenant.roles_new")}
          </Button>
        ) : null
      }
    >
      <RolesSection
        canWrite={canWrite}
        onEdit={(role) => setDialog({ mode: "edit", role })}
      />
      {dialog && canWrite ? (
        <RoleDialog
          key={dialog.mode === "edit" ? dialog.role.key : "create"}
          state={dialog}
          onClose={() => setDialog(null)}
        />
      ) : null}
    </EntityPage>
  );
}
