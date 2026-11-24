"use client";

import { Play, Plus, Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

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
import { useEngagementOptions } from "@/features/office/hooks/use-office-invoices";
import {
  type VATReportRule,
  useRunVATReportRule,
  useSaveVATReportRules,
  useVATReportRules,
} from "@/features/office/hooks/use-vat-tracking";
import { useLocale } from "@/providers/locale-provider";

const DAYS = Array.from({ length: 31 }, (_, i) => i + 1);
const BEFORE = Array.from({ length: 28 }, (_, i) => i);

function newRule(): VATReportRule {
  return {
    trigger_kind: "day",
    day: 15,
    days_before: null,
    audience: "office",
    scope: "all",
    group_ref: null,
    engagement_uuids: [],
    label: null,
    enabled: true,
  };
}

function dateLabel(iso: string, locale: string) {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d)).toLocaleDateString(
    locale === "tr" ? "tr-TR" : "en-GB",
    { day: "numeric", month: "long", timeZone: "UTC" },
  );
}

/** Seçili mükellefler: arama kutulu onay listesi. */
function TaxpayerPicker({
  slug,
  value,
  onChange,
  disabled,
}: {
  slug: string;
  value: string[];
  onChange: (v: string[]) => void;
  disabled: boolean;
}) {
  const { t } = useLocale();
  const [open, setOpen] = useState(false);
  const [q, setQ] = useState("");
  const [picked, setPicked] = useState<string[]>(value);
  const { options } = useEngagementOptions(slug);
  const shown = options.filter((o) =>
    `${o.title} ${o.taxId}`
      .toLocaleLowerCase("tr")
      .includes(q.toLocaleLowerCase("tr")),
  );
  return (
    <>
      <Button
        type="button"
        size="sm"
        variant="outline"
        disabled={disabled}
        onClick={() => {
          setPicked(value);
          setOpen(true);
        }}
      >
        {t("tenant.vat_rules_pick", { count: value.length })}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("tenant.vat_rules_pick_title")}</DialogTitle>
            <DialogDescription>
              {t("tenant.vat_rules_pick_description")}
            </DialogDescription>
          </DialogHeader>
          <Input
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder={t("tenant.vat_rules_pick_search")}
            aria-label={t("tenant.vat_rules_pick_search")}
          />
          <ul className="max-h-72 space-y-1 overflow-y-auto">
            {shown.map((o) => {
              const id = `pick-${o.engagement}`;
              return (
                <li key={o.engagement} className="flex items-center gap-2">
                  <Checkbox
                    id={id}
                    checked={picked.includes(o.engagement)}
                    onCheckedChange={(c) =>
                      setPicked((p) =>
                        c
                          ? [...p, o.engagement]
                          : p.filter((x) => x !== o.engagement),
                      )
                    }
                  />
                  <Label htmlFor={id} className="font-normal">
                    {o.title || o.taxId}
                  </Label>
                </li>
              );
            })}
          </ul>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t("tenant.vat_rules_cancel")}
            </Button>
            <Button
              onClick={() => {
                onChange(picked);
                setOpen(false);
              }}
            >
              {t("tenant.vat_rules_pick_done", { count: picked.length })}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

/**
 * Ayarlar → KDV takibi → KDV raporu günleri (#780 B): ayın 29'u sabit
 * değil. Ofis birden çok gün (15'i ara rapor, 25'i, 29'u) ya da "ay
 * sonundan X gün önce" tanımlar; her satırda kime (yalnız ofis, ofis
 * onayıyla mükellef, doğrudan mükellef) ve hangi mükellefler seçilir.
 * Kural çalışınca "KDV raporu hazır" olayı üretilir.
 */
export function VATReportRules({
  slug,
  canWrite,
  disabled,
}: {
  slug: string;
  canWrite: boolean;
  disabled: boolean;
}) {
  const { t, locale } = useLocale();
  const query = useVATReportRules(slug);
  const save = useSaveVATReportRules(slug);
  const run = useRunVATReportRule(slug);
  const [rules, setRules] = useState<VATReportRule[]>(query.data?.items ?? []);
  const [seen, setSeen] = useState(query.data);
  const [error, setError] = useState<string | null>(null);
  // Sunucudan yeni liste gelince (ilk yükleme, kayıt) düzenleme ona döner.
  if (query.data !== seen) {
    setSeen(query.data);
    setRules(query.data?.items ?? []);
  }

  const busy = !canWrite || !query.data || save.isPending || disabled;
  const dirty =
    !!query.data && JSON.stringify(rules) !== JSON.stringify(query.data.items);

  const patch = (i: number, p: Partial<VATReportRule>) =>
    setRules((rs) => rs.map((r, j) => (j === i ? { ...r, ...p } : r)));

  const onSave = () => {
    setError(null);
    save.mutate(
      rules.map(({ next_run_on: _next, ...r }) => ({
        ...r,
        engagement_uuids: r.scope === "selected" ? r.engagement_uuids : [],
        label: r.label?.trim() ? r.label.trim() : null,
      })),
      {
        onSuccess: () => toast.success(t("tenant.vat_rules_saved")),
        onError: (e) =>
          setError(
            e instanceof Error && e.message
              ? e.message
              : t("tenant.vat_rules_save_error"),
          ),
      },
    );
  };

  const onRun = (key: string) =>
    run.mutate(key, {
      onSuccess: (res) =>
        toast.success(
          t("tenant.vat_rules_ran", {
            taxpayers: res.taxpayers,
            payable: res.payable,
          }),
        ),
      onError: (e) =>
        toast.error(
          e instanceof Error && e.message
            ? e.message
            : t("tenant.vat_rules_run_error"),
        ),
    });

  return (
    <div className="space-y-3 border-t pt-4" data-testid="vat-report-rules">
      <div className="space-y-1">
        <p className="text-sm font-medium">{t("tenant.vat_rules_title")}</p>
        <p className="text-text-2 text-sm">{t("tenant.vat_rules_hint")}</p>
      </div>
      <ul className="space-y-3">
        {rules.map((r, i) => (
          <li
            key={r.uuid ?? `new-${i}`}
            className="border-border space-y-2 rounded-md border p-3"
            data-testid="vat-report-rule"
          >
            <div className="grid gap-2 sm:grid-cols-2 lg:grid-cols-4">
              <div className="space-y-1">
                <Label htmlFor={`rule-${i}-when`}>
                  {t("tenant.vat_rules_when")}
                </Label>
                <Select
                  value={r.trigger_kind}
                  disabled={busy}
                  onValueChange={(v) =>
                    patch(
                      i,
                      v === "day"
                        ? { trigger_kind: "day", day: 29, days_before: null }
                        : {
                            trigger_kind: "before_month_end",
                            day: null,
                            days_before: 3,
                          },
                    )
                  }
                >
                  <SelectTrigger id={`rule-${i}-when`}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="day">
                      {t("tenant.vat_rules_kind_day")}
                    </SelectItem>
                    <SelectItem value="before_month_end">
                      {t("tenant.vat_rules_kind_before")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label htmlFor={`rule-${i}-day`}>
                  {r.trigger_kind === "day"
                    ? t("tenant.vat_rules_day")
                    : t("tenant.vat_rules_days_before")}
                </Label>
                <Select
                  value={String(
                    r.trigger_kind === "day"
                      ? (r.day ?? 29)
                      : (r.days_before ?? 0),
                  )}
                  disabled={busy}
                  onValueChange={(v) =>
                    patch(
                      i,
                      r.trigger_kind === "day"
                        ? { day: Number(v) }
                        : { days_before: Number(v) },
                    )
                  }
                >
                  <SelectTrigger id={`rule-${i}-day`}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {(r.trigger_kind === "day" ? DAYS : BEFORE).map((d) => (
                      <SelectItem key={d} value={String(d)}>
                        {r.trigger_kind === "day"
                          ? t(
                              d > 28
                                ? "tenant.vat_report_day_option_late"
                                : "tenant.vat_report_day_option",
                              { day: d },
                            )
                          : d === 0
                            ? t("tenant.vat_rules_month_end")
                            : t("tenant.vat_rules_days_before_option", {
                                count: d,
                              })}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label htmlFor={`rule-${i}-to`}>
                  {t("tenant.vat_rules_to")}
                </Label>
                <Select
                  value={r.audience}
                  disabled={busy}
                  onValueChange={(v) =>
                    patch(i, { audience: v as VATReportRule["audience"] })
                  }
                >
                  <SelectTrigger id={`rule-${i}-to`}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="office">
                      {t("tenant.vat_rules_to_office")}
                    </SelectItem>
                    <SelectItem value="taxpayer_approval">
                      {t("tenant.vat_rules_to_approval")}
                    </SelectItem>
                    <SelectItem value="taxpayer_direct">
                      {t("tenant.vat_rules_to_direct")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1">
                <Label htmlFor={`rule-${i}-who`}>
                  {t("tenant.vat_rules_who")}
                </Label>
                <Select
                  value={r.scope}
                  disabled={busy}
                  onValueChange={(v) =>
                    patch(i, { scope: v as VATReportRule["scope"] })
                  }
                >
                  <SelectTrigger id={`rule-${i}-who`}>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="all">
                      {t("tenant.vat_rules_who_all")}
                    </SelectItem>
                    <SelectItem value="payable_only">
                      {t("tenant.vat_rules_who_payable")}
                    </SelectItem>
                    <SelectItem value="selected">
                      {t("tenant.vat_rules_who_selected")}
                    </SelectItem>
                    <SelectItem value="group" disabled>
                      {t("tenant.vat_rules_who_group")}
                    </SelectItem>
                  </SelectContent>
                </Select>
              </div>
            </div>
            <div className="flex flex-wrap items-center gap-3">
              {r.scope === "selected" ? (
                <TaxpayerPicker
                  slug={slug}
                  value={r.engagement_uuids}
                  disabled={busy}
                  onChange={(v) => patch(i, { engagement_uuids: v })}
                />
              ) : null}
              <Input
                className="max-w-56"
                value={r.label ?? ""}
                maxLength={80}
                disabled={busy}
                placeholder={t("tenant.vat_rules_label")}
                aria-label={t("tenant.vat_rules_label")}
                onChange={(e) => patch(i, { label: e.target.value })}
              />
              <div className="flex items-center gap-2">
                <Switch
                  id={`rule-${i}-on`}
                  checked={r.enabled}
                  disabled={busy}
                  onCheckedChange={(v) => patch(i, { enabled: v })}
                />
                <Label htmlFor={`rule-${i}-on`} className="font-normal">
                  {t("tenant.vat_rules_enabled")}
                </Label>
              </div>
              {r.next_run_on && r.enabled ? (
                <span className="text-text-2 text-xs">
                  {t("tenant.vat_rules_next", {
                    date: dateLabel(r.next_run_on, locale),
                  })}
                </span>
              ) : null}
              <div className="ml-auto flex gap-2">
                {r.next_run_on && !dirty ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={busy || run.isPending}
                    onClick={() => onRun(r.uuid ?? "default")}
                  >
                    <Play className="size-4" aria-hidden />
                    {t("tenant.vat_rules_run")}
                  </Button>
                ) : null}
                <Button
                  type="button"
                  size="sm"
                  variant="ghost"
                  disabled={busy || rules.length <= 1}
                  aria-label={t("tenant.vat_rules_remove")}
                  onClick={() => setRules((rs) => rs.filter((_, j) => j !== i))}
                >
                  <Trash2 className="size-4" aria-hidden />
                </Button>
              </div>
            </div>
          </li>
        ))}
      </ul>
      {error ? (
        <p role="alert" className="text-danger text-sm">
          {error}
        </p>
      ) : null}
      {/* Yazma yetkisi yoksa düğmeler hiç çıkmaz (sayfadaki diğer kartlar gibi). */}
      {canWrite ? (
        <div className="flex flex-wrap gap-2">
          <Button
            type="button"
            size="sm"
            variant="outline"
            disabled={busy || rules.length >= 10}
            onClick={() => setRules((rs) => [...rs, newRule()])}
          >
            <Plus className="size-4" aria-hidden />
            {t("tenant.vat_rules_add")}
          </Button>
          <Button
            type="button"
            size="sm"
            disabled={busy || !dirty}
            onClick={onSave}
          >
            {save.isPending
              ? t("tenant.vat_rules_saving")
              : t("tenant.vat_rules_save")}
          </Button>
        </div>
      ) : null}
    </div>
  );
}
