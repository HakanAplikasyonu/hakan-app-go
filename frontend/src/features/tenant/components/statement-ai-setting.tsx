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
export const STATEMENT_AI_SETTING = "bank_statement_ai";

/**
 * Ayarlar → Yapay zekâ (#669): banka ekstresi yüklerken tanınmayan düzende
 * kolonların anlamını AI'a önerdirme. Kapalıyken yalnız kural tabanlı akış
 * (şablonlar, başlık eşanlamlıları, sezgi) çalışır; dosya AI'a gitmez.
 */
export function StatementAiSetting({
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
  const on = settings[STATEMENT_AI_SETTING] !== false;
  return (
    <EntitySectionCard title={t("tenant.statement_ai_title")}>
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="statement-ai">{t("tenant.statement_ai_label")}</Label>
          <p className="text-text-2 text-sm">
            {t("tenant.statement_ai_description")}
          </p>
        </div>
        <Switch
          id="statement-ai"
          checked={on}
          disabled={!canWrite || !query.data || patch.isPending}
          onCheckedChange={(v) =>
            patch.mutate({
              settings: { ...settings, [STATEMENT_AI_SETTING]: v },
            })
          }
        />
      </div>
    </EntitySectionCard>
  );
}
