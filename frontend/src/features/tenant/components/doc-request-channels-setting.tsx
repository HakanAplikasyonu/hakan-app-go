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
export const DOC_REQUEST_CHANNELS_SETTING_KEY = "doc_request_channels";

/**
 * Ayarlar → Belge istekleri (Oğuz'un geri bildirimi): belge isterken kanal
 * seçimi (uygulama bildirimi, SMS, e-posta), son tarihten önce hatırlatma ve
 * "Hatırlat" düğmesi. Varsayılan açık; kapalıyken istek eskisi gibi
 * mükellefin uygulama kullanıcılarına bildirim ve e-posta olarak gider.
 */
export function DocRequestChannelsSetting({
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
  const on = settings[DOC_REQUEST_CHANNELS_SETTING_KEY] !== false;
  return (
    <EntitySectionCard title={t("tenant.doc_request_channels_title")}>
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="doc-request-channels">
            {t("tenant.doc_request_channels_label")}
          </Label>
          <p className="text-text-2 text-sm">
            {on
              ? t("tenant.doc_request_channels_on")
              : t("tenant.doc_request_channels_off")}
          </p>
        </div>
        <Switch
          id="doc-request-channels"
          checked={on}
          disabled={!canWrite || !query.data || patch.isPending}
          onCheckedChange={(v) =>
            patch.mutate({
              settings: { ...settings, [DOC_REQUEST_CHANNELS_SETTING_KEY]: v },
            })
          }
        />
      </div>
    </EntitySectionCard>
  );
}
