"use client";

import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Switch } from "@/components/ui/switch";
import {
  type OfficeVATWatchEvent,
  type VATTrackingSettings,
  useOfficeVATWatchEvents,
  useSaveVATTrackingSettings,
} from "@/features/office/hooks/use-vat-tracking";
import { formatMoney } from "@/lib/decimal";
import { useLocale } from "@/providers/locale-provider";

/** "10000.00" → "10000"; "2500.50" → "2500,50" (giriş kutusu için). */
function toInput(v: string | null | undefined): string {
  if (!v) return "";
  const [int, frac] = v.split(".");
  return !frac || /^0+$/.test(frac) ? int : `${int},${frac}`;
}

/**
 * Girilen tutarı API biçimine çevirir: "10.000" / "10000" / "10.000,50" →
 * "10000.00". Boş: null (eşik yok). Geçersiz ya da sıfır: undefined.
 */
function parseLimitInput(raw: string): string | null | undefined {
  const s = raw.trim().replace(/\s|₺|TL/gi, "");
  if (s === "") return null;
  const normalized = s.replace(/\./g, "").replace(",", ".");
  if (!/^\d+(\.\d{1,2})?$/.test(normalized)) return undefined;
  const [int, frac = ""] = normalized.split(".");
  const out = `${int.replace(/^0+(?=\d)/, "")}.${frac.padEnd(2, "0")}`;
  return Number(out) > 0 ? out : undefined;
}

/**
 * Ayarlar → KDV takibi → KDV nöbetçisi (#780 B): aç/kapa, "ödenecek KDV şu
 * tutarı geçerse uyar" eşiği (uyarı kurallarındaki VAT_PAYABLE_THRESHOLD;
 * nöbetçi ve Uyarılar sayfası aynı eşiği okur) ve bildirim kanalı.
 */
export function VATWatchSettings({
  slug,
  canWrite,
  settings,
}: {
  slug: string;
  canWrite: boolean;
  settings: VATTrackingSettings | undefined;
}) {
  const { t } = useLocale();
  const save = useSaveVATTrackingSettings(slug);
  const on = settings?.watch_enabled !== false;
  const email = settings?.watch_email === true;
  const serverLimit = settings?.payable_limit ?? null;
  const [seenLimit, setSeenLimit] = useState(serverLimit);
  const [draft, setDraft] = useState(toInput(serverLimit));
  if (seenLimit !== serverLimit) {
    setSeenLimit(serverLimit);
    setDraft(toInput(serverLimit));
  }
  const busy = !canWrite || !settings || save.isPending;
  const base = {
    report_enabled: settings?.report_enabled !== false,
    report_day: settings?.report_day ?? 29,
  };
  const parsed = parseLimitInput(draft);
  const dirty = parsed !== undefined && parsed !== serverLimit;

  const saveLimit = () => {
    if (parsed === undefined) {
      toast.error(t("tenant.vat_watch_limit_invalid"));
      return;
    }
    save.mutate(
      { ...base, payable_limit: parsed },
      {
        onSuccess: () => toast.success(t("tenant.vat_watch_limit_saved")),
        onError: () => toast.error(t("tenant.vat_watch_save_error")),
      },
    );
  };

  return (
    <div className="space-y-4 border-t pt-4" data-testid="vat-watch-settings">
      <div className="flex items-start justify-between gap-4">
        <div className="space-y-1">
          <Label htmlFor="vat-watch-enabled">
            {t("tenant.vat_watch_label")}
          </Label>
          <p className="text-text-2 text-sm">
            {on ? t("tenant.vat_watch_on") : t("tenant.vat_watch_off")}
          </p>
        </div>
        <Switch
          id="vat-watch-enabled"
          checked={on}
          disabled={busy}
          onCheckedChange={(v) => save.mutate({ ...base, watch_enabled: v })}
        />
      </div>
      <div className="grid gap-4 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="vat-watch-limit">
            {t("tenant.vat_watch_limit_label")}
          </Label>
          <div className="flex items-center gap-2">
            <Input
              id="vat-watch-limit"
              inputMode="decimal"
              placeholder={t("tenant.vat_watch_limit_placeholder")}
              value={draft}
              disabled={busy || !on}
              onChange={(e) => setDraft(e.target.value)}
              onKeyDown={(e) => {
                if (e.key === "Enter" && dirty) saveLimit();
              }}
              className="max-w-40"
            />
            <span className="text-text-2 text-sm">TL</span>
            <Button
              size="sm"
              onClick={saveLimit}
              disabled={busy || !on || !dirty}
            >
              {t("tenant.vat_watch_limit_save")}
            </Button>
          </div>
          <p className="text-text-2 text-xs">
            {serverLimit
              ? t("tenant.vat_watch_limit_hint_set", {
                  amount: formatMoney(serverLimit),
                })
              : t("tenant.vat_watch_limit_hint_empty")}
          </p>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="vat-watch-channel">
            {t("tenant.vat_watch_channel_label")}
          </Label>
          <Select
            value={email ? "email" : "inapp"}
            onValueChange={(v) =>
              save.mutate({ ...base, watch_email: v === "email" })
            }
            disabled={busy || !on}
          >
            <SelectTrigger id="vat-watch-channel">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="inapp">
                {t("tenant.vat_watch_channel_inapp")}
              </SelectItem>
              <SelectItem value="email">
                {t("tenant.vat_watch_channel_email")}
              </SelectItem>
            </SelectContent>
          </Select>
          <p className="text-text-2 text-xs">
            {t("tenant.vat_watch_channel_hint")}
          </p>
        </div>
      </div>
      <RecentVATAlerts slug={slug} />
    </div>
  );
}

const EVENT_KEYS: Record<string, string> = {
  "vat.threshold_exceeded": "tenant.vat_alert_threshold",
  "vat.status_changed": "tenant.vat_alert_changed",
  "vat.report_ready": "tenant.vat_alert_report",
};

function monthLabel(period: string, locale: string) {
  const [y, m] = period.split("-").map(Number);
  if (!y || !m) return period;
  return new Date(Date.UTC(y, m - 1, 1)).toLocaleDateString(
    locale === "tr" ? "tr-TR" : "en-GB",
    { month: "long", year: "numeric", timeZone: "UTC" },
  );
}

function payloadAmount(e: OfficeVATWatchEvent): string | null {
  const v = e.payload.payable;
  return typeof v === "string" && v !== "" ? v : null;
}

/** Son KDV uyarıları: ofis genelindeki son nöbetçi olayları. */
function RecentVATAlerts({ slug }: { slug: string }) {
  const { t, locale } = useLocale();
  const query = useOfficeVATWatchEvents(slug, 10);
  const items = query.data?.items ?? [];
  return (
    <div className="space-y-2" data-testid="vat-recent-alerts">
      <div className="space-y-0.5">
        <h3 className="text-sm font-medium">{t("tenant.vat_alerts_title")}</h3>
        <p className="text-text-2 text-xs">{t("tenant.vat_alerts_hint")}</p>
      </div>
      {query.isError ? (
        <p className="text-text-2 text-sm">{t("tenant.vat_alerts_error")}</p>
      ) : items.length === 0 ? (
        <p className="text-text-2 text-sm">
          {query.isLoading
            ? t("tenant.vat_alerts_loading")
            : t("tenant.vat_alerts_empty")}
        </p>
      ) : (
        <ul className="divide-y rounded-md border">
          {items.map((e) => {
            const amount = payloadAmount(e);
            return (
              <li
                key={e.uuid}
                className="flex flex-wrap items-center justify-between gap-2 px-3 py-2 text-sm"
                data-testid="vat-alert-row"
              >
                <div className="min-w-0 space-y-0.5">
                  <Link
                    href={`/t/${slug}/engagements/${e.engagement_uuid}/report`}
                    className="font-medium hover:underline"
                  >
                    {e.taxpayer_title}
                  </Link>
                  <p className="text-text-2 text-xs">
                    {t(EVENT_KEYS[e.event] ?? "tenant.vat_alert_other")} ·{" "}
                    {monthLabel(e.period, locale)}
                  </p>
                </div>
                <div className="text-right">
                  {amount ? (
                    <p className="tabular-nums">
                      {t("tenant.vat_alert_payable", {
                        amount: formatMoney(amount),
                      })}
                    </p>
                  ) : null}
                  <p className="text-text-2 text-xs">
                    {new Date(e.created_at).toLocaleString(
                      locale === "tr" ? "tr-TR" : "en-GB",
                      { dateStyle: "medium", timeStyle: "short" },
                    )}
                  </p>
                </div>
              </li>
            );
          })}
        </ul>
      )}
    </div>
  );
}
