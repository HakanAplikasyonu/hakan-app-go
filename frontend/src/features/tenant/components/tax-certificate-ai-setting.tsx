"use client";

import { EntitySectionCard } from "@/components/entity";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  usePatchTenantSettings,
  useTenantSettings,
} from "@/features/tenant/hooks/use-tenant-admin";
import { useLocale } from "@/providers/locale-provider";

/** organizations.settings anahtarı; yoksa açık (backend ile aynı). */
export const TAX_CERTIFICATE_AI_SETTING = "tax_certificate_ai";

/**
 * Ayarlar → Yapay zekâ: mükellef eklerken vergi levhasını yapay zekâyla
 * okuyup formu doldurma. Kapalıyken "Vergi levhasından doldur" görünmez,
 * dosya yapay zekâya gitmez; form elle doldurulur.
 */
export function TaxCertificateAiSetting({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const query = useTenantSettings(slug);
  const patch = usePatchTenantSettings(slug);
  const settings = (query.data?.settings ?? {}) as Record<string, unknown>;
  const on = settings[TAX_CERTIFICATE_AI_SETTING] !== false;
  return (
    <EntitySectionCard title={t("tenant.tax_certificate_ai_title")}>
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="tax-certificate-ai">{t("tenant.tax_certificate_ai_label")}</Label>
          <p className="text-text-2 text-sm">
            {t("tenant.tax_certificate_ai_description")}
          </p>
        </div>
        <Switch
          id="tax-certificate-ai"
          checked={on}
          disabled={!canWrite || !query.data || patch.isPending}
          onCheckedChange={(v) =>
            patch.mutate({
              settings: { ...settings, [TAX_CERTIFICATE_AI_SETTING]: v },
            })
          }
        />
      </div>
    </EntitySectionCard>
  );
}
