"use client";

import { MailPlus } from "lucide-react";
import { useMemo, useState } from "react";
import { z } from "zod";

import { AppForm, AppInput, AppSelect } from "@/components/forms";
import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import {
  useInviteTenantMember,
  useTenantRoles,
} from "@/features/tenant/hooks/use-tenant-admin";
import {
  DEFAULT_INVITE_ROLE_KEY,
  TENANT_ROLE_KEY_PATTERN,
  canChangeRole,
  tenantRoleOptions,
  type TenantRoleOption,
} from "@/features/tenant/lib/roles";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import type {
  TenantInvitation,
  TenantRoleKey,
} from "@/features/tenant/services/tenant.service";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/**
 * The office's roles for pickers and labels. Read only with
 * `tenant.roles.read`; without it (or while loading) the template list is
 * used, and the server still validates the chosen key.
 */
export function useOfficeRoles() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const query = useTenantRoles(slug, {
    enabled: can(permissions.tenant.roles.read),
    silent: true,
  });
  const roles = query.data?.items;
  const options = useMemo(() => tenantRoleOptions(t, roles), [t, roles]);
  return { roles, options };
}

/**
 * E-mail + role picker that sends an office invitation right away (members
 * page, office setup wizard). Roles above the caller's are left out.
 */
export function InviteForm({
  actorRole,
  roleOptions: allRoleOptions,
  id = "tenant-invite-form",
  submitLabel,
  onInvited,
}: {
  actorRole: string;
  roleOptions: readonly TenantRoleOption[];
  id?: string;
  submitLabel?: string;
  onInvited?: (invitation: TenantInvitation) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const invite = useInviteTenantMember(slug);
  const [formKey, setFormKey] = useState(0);
  const schema = useMemo(
    () =>
      z.object({
        email: z.email(t("tenant.invite_validation_email")),
        role_key: z.string().regex(TENANT_ROLE_KEY_PATTERN),
      }),
    [t],
  );
  const roleOptions = allRoleOptions
    .filter((o) => canChangeRole(actorRole, "", o.key))
    .map((o) => ({ value: o.key, label: o.label }));
  const defaultRole = roleOptions.some(
    (o) => o.value === DEFAULT_INVITE_ROLE_KEY,
  )
    ? DEFAULT_INVITE_ROLE_KEY
    : (roleOptions[0]?.value ?? DEFAULT_INVITE_ROLE_KEY);

  return (
    <AppForm
      key={formKey}
      id={id}
      schema={schema}
      defaultValues={{ email: "", role_key: defaultRole }}
      onSubmit={async (values: { email: string; role_key: TenantRoleKey }) => {
        const invitation = await invite.mutateAsync({
          email: values.email.trim(),
          role_key: values.role_key,
        });
        onInvited?.(invitation);
        setFormKey((k) => k + 1);
      }}
    >
      {(form) => (
        <div className="mb-6 grid items-end gap-3 sm:grid-cols-[1fr_12rem_auto]">
          <AppInput
            name="email"
            type="email"
            label={t("tenant.invite_email")}
            autoComplete="off"
          />
          <AppSelect
            name="role_key"
            label={t("tenant.invite_role")}
            options={roleOptions}
          />
          <Button
            type="submit"
            disabled={invite.isPending || form.formState.isSubmitting}
          >
            <MailPlus className="size-4" />
            {submitLabel ?? t("tenant.invite_submit")}
          </Button>
        </div>
      )}
    </AppForm>
  );
}
