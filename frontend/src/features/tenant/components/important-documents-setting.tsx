"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { EntitySectionCard } from "@/components/entity";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  importantSettingsKey,
  importantSettingsService,
} from "@/features/office/services/archive-documents.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * Ayarlar → Belgeler (#780 B): mükellef sayfasındaki ve mükellef panelindeki
 * "Önemli belgeler" bölümü. Varsayılan açık; kapalıyken bölüm gizlenir,
 * belgeler arşivde durur.
 */
export function ImportantDocumentsSetting({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: importantSettingsKey(slug),
    queryFn: () => importantSettingsService.get(slug),
  });
  const save = useMutation({
    mutationFn: (enabled: boolean) =>
      importantSettingsService.save(slug, enabled),
    onSuccess: (data) => qc.setQueryData(importantSettingsKey(slug), data),
  });
  const on = query.data?.enabled !== false;
  return (
    <EntitySectionCard title={t("tenant.important_docs_title")}>
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="important-docs-enabled">
            {t("tenant.important_docs_label")}
          </Label>
          <p className="text-text-2 text-sm">
            {on
              ? t("tenant.important_docs_on")
              : t("tenant.important_docs_off")}
          </p>
        </div>
        <Switch
          id="important-docs-enabled"
          checked={on}
          disabled={!canWrite || !query.data || save.isPending}
          onCheckedChange={(v) => save.mutate(v)}
        />
      </div>
    </EntitySectionCard>
  );
}
