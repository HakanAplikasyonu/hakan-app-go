"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { EntitySectionCard } from "@/components/entity";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  documentUploadKeys,
  documentUploadsService,
} from "@/features/office/services/document-uploads.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * Ayarlar → Belgeler: evrak yüklemede tür ve dönem seçimi (Oğuz'un demo geri
 * bildirimi). Varsayılan açık; kapalıyken "Evrak yükle" bölümü gizlenir.
 */
export function DocumentUploadSetting({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const query = useQuery({
    queryKey: documentUploadKeys.settings(slug),
    queryFn: () => documentUploadsService.getSettings(slug),
  });
  const save = useMutation({
    mutationFn: (enabled: boolean) =>
      documentUploadsService.saveSettings(slug, enabled),
    onSuccess: (data) => qc.setQueryData(documentUploadKeys.settings(slug), data),
  });
  const on = query.data?.enabled !== false;
  return (
    <EntitySectionCard title={t("tenant.doc_upload_title")}>
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="doc-upload-enabled">
            {t("tenant.doc_upload_label")}
          </Label>
          <p className="text-text-2 text-sm">
            {on ? t("tenant.doc_upload_on") : t("tenant.doc_upload_off")}
          </p>
        </div>
        <Switch
          id="doc-upload-enabled"
          checked={on}
          disabled={!canWrite || !query.data || save.isPending}
          onCheckedChange={(v) => save.mutate(v)}
        />
      </div>
    </EntitySectionCard>
  );
}
