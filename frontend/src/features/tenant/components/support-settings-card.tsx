"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { LifeBuoy } from "lucide-react";
import { useMemo } from "react";
import { z } from "zod";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntitySectionCard } from "@/components/entity";
import { AppForm, AppInput } from "@/components/forms";
import { Button } from "@/components/ui/button";
import {
  supportKeys,
  supportService,
} from "@/features/tenant/services/support.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

/**
 * Ayarlar → Destek (#387): ofisin kendi ClickFix widget kimliği. Mükellefler
 * portaldaki Destek düğmesinden bu widget'a, yani doğrudan ofise yazar. Boşsa
 * platform varsayılanı kullanılır.
 */
export function SupportSettingsCard({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: supportKeys.settings(slug),
    queryFn: () => supportService.settings(slug),
  });
  const save = useAppMutation({
    mutationFn: (id: string) => supportService.putSettings(slug, id),
    onSuccess: (data) => {
      qc.setQueryData(supportKeys.settings(slug), data);
      appToast.success(t("tenant.support_saved"));
    },
  });
  const schema = useMemo(
    () =>
      z.object({
        clickfix_widget_id: z
          .string()
          .trim()
          .max(128, t("tenant.support_widget_invalid"))
          .regex(/^[A-Za-z0-9_-]*$/, t("tenant.support_widget_invalid")),
      }),
    [t],
  );

  return (
    <EntitySectionCard title={t("tenant.support_card_title")}>
      {query.isLoading ? <Loading label={t("common.loading")} /> : null}
      {query.isError ? (
        <ErrorState
          title={t("common.error_generic")}
          onRetry={() => void query.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : null}
      {query.data ? (
        <AppForm
          key={query.data.clickfix_widget_id}
          id="support-settings-form"
          schema={schema}
          defaultValues={{
            clickfix_widget_id: query.data.clickfix_widget_id,
          }}
          onSubmit={async (values: { clickfix_widget_id: string }) => {
            await save.mutateAsync(values.clickfix_widget_id.trim());
          }}
        >
          {(form) => (
            <div className="space-y-4">
              <p className="text-text-2 text-sm">
                {t("tenant.support_card_description")}
              </p>
              <AppInput
                name="clickfix_widget_id"
                label={t("tenant.support_widget_label")}
                description={t("tenant.support_widget_help")}
                placeholder={t("tenant.support_widget_placeholder")}
                startIcon={LifeBuoy}
                disabled={!canWrite}
                autoComplete="off"
                spellCheck={false}
              />
              {canWrite ? (
                <div className="flex justify-end">
                  <Button
                    type="submit"
                    disabled={
                      save.isPending ||
                      form.formState.isSubmitting ||
                      !form.formState.isDirty
                    }
                  >
                    {t("tenant.settings_save")}
                  </Button>
                </div>
              ) : null}
            </div>
          )}
        </AppForm>
      ) : null}
    </EntitySectionCard>
  );
}
