"use client";

import { useMemo, useState } from "react";
import { toast } from "sonner";

import { AppForm, AppInput } from "@/components/forms";
import { Alert, AlertDescription } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import {
  createProfileSchema,
  type ProfileFormValues,
} from "@/features/auth/schemas";
import { isApiError } from "@/lib/api";
import { mapMeToAuthUser } from "@/lib/auth/types";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";
import { authService } from "@/services/auth.service";

export function ProfileEditForm() {
  const { t } = useLocale();
  const { user, setUser } = useAuth();
  const [pending, setPending] = useState(false);
  const [formError, setFormError] = useState<string | null>(null);
  const schema = useMemo(() => createProfileSchema(t), [t]);

  if (!user) return null;

  const onSubmit = async (values: ProfileFormValues) => {
    setFormError(null);
    setPending(true);
    try {
      const me = await authService.updateProfile({
        name: values.name,
        surname: values.surname,
        // Empty string clears the number.
        smmm_registry_no: values.smmm_registry_no.trim(),
      });
      setUser(mapMeToAuthUser(me));
      toast.success(t("auth.profile.save_success"));
    } catch (error) {
      const message =
        isApiError(error) && error.message
          ? error.message
          : t("common.error_generic");
      setFormError(message);
      toast.error(t("auth.profile.save_error"), { description: message });
    } finally {
      setPending(false);
    }
  };

  return (
    <AppForm
      schema={schema}
      defaultValues={{
        name: user.name,
        surname: user.surname,
        smmm_registry_no: user.smmmRegistryNo ?? "",
      }}
      onSubmit={onSubmit}
      className="space-y-4"
    >
      {formError ? (
        <Alert variant="destructive">
          <AlertDescription>{formError}</AlertDescription>
        </Alert>
      ) : null}

      <div className="grid gap-4 sm:grid-cols-2">
        <AppInput
          name="name"
          label={t("auth.profile.first_name")}
          autoComplete="given-name"
        />
        <AppInput
          name="surname"
          label={t("auth.profile.last_name")}
          autoComplete="family-name"
        />
      </div>

      <AppInput
        name="smmm_registry_no"
        label={t("auth.profile.smmm_registry_no")}
        description={t("auth.profile.smmm_registry_no_hint")}
        autoComplete="off"
        maxLength={32}
      />

      <Button type="submit" disabled={pending}>
        {pending ? t("auth.profile.saving") : t("auth.profile.save")}
      </Button>
    </AppForm>
  );
}
