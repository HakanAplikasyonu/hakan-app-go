"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useMemo, useRef, useState } from "react";

import { StatusPill } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DatePicker } from "@/components/ui/date-picker";
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
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Switch } from "@/components/ui/switch";
import { TabStrip, TabStripItem } from "@/components/ui/tab-strip";
import { Textarea } from "@/components/ui/textarea";
import {
  AUTOMATION_VARIABLES,
  renderVariables,
  SAMPLE_VARS,
  smsLength,
} from "@/features/tenant/lib/sms-length";
import {
  automationKeys,
  automationService,
  type AutomationEvent,
  type AutomationRule,
  type AutomationRuleInput,
  type AutomationTrigger,
} from "@/features/tenant/services/automation.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Channel = "sms" | "email" | "inapp";

const emptyTrigger: AutomationTrigger = {
  mode: "days",
  days: [],
  hour: 9,
  before_due_days: 1,
  tax: "",
  current_period: false,
  event: "",
  delay_min_hours: 0,
  delay_max_hours: 0,
  resend_after_days: 0,
  payment_date: "",
  payment_amount: "",
  payment_days_before: 0,
};

function initialForm(rule?: AutomationRule): AutomationRuleInput {
  if (!rule) {
    return {
      name: "",
      trigger_kind: "calendar",
      trigger: { ...emptyTrigger },
      target_kind: "all",
      target_engagements: [],
      only_due: false,
      channels: ["sms"],
      sms_if_unopened_hours: null,
      subject: "",
      body: "",
      approval_mode: "auto",
      active: true,
    };
  }
  return {
    name: rule.name,
    trigger_kind: rule.trigger_kind,
    trigger: { ...emptyTrigger, ...rule.trigger },
    target_kind: rule.target_kind,
    target_group_uuid: rule.target_group_uuid,
    target_engagements: rule.target_engagements,
    only_due: rule.only_due,
    channels: rule.channels,
    sms_if_unopened_hours: rule.sms_if_unopened_hours,
    subject: rule.subject,
    body: rule.body,
    approval_mode: rule.approval_mode,
    active: rule.active,
  };
}

function toInt(v: string, min: number, max: number): number {
  const n = Number.parseInt(v, 10);
  if (Number.isNaN(n)) return min;
  return Math.min(max, Math.max(min, n));
}

const DAYS = Array.from({ length: 31 }, (_, i) => i + 1);
const HOURS = Array.from({ length: 24 }, (_, i) => i);

/**
 * Otomasyon kuralı penceresi (#780 C): ad, tetikleyici (takvim / olay /
 * kişiye özel ödeme), hedef, kanallar, değişkenli metin ve SMS sayacı,
 * örnek mükellefle önizleme, onay modu ve etkin anahtarı.
 */
export function AutomationRuleDialog({
  slug,
  open,
  onOpenChange,
  rule,
  events,
}: {
  slug: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  rule?: AutomationRule;
  events: AutomationEvent[];
}) {
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        {open ? (
          <RuleForm
            key={rule?.uuid ?? "new"}
            slug={slug}
            rule={rule}
            events={events}
            onDone={() => onOpenChange(false)}
          />
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

function RuleForm({
  slug,
  rule,
  events,
  onDone,
}: {
  slug: string;
  rule?: AutomationRule;
  events: AutomationEvent[];
  onDone: () => void;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [form, setForm] = useState<AutomationRuleInput>(() =>
    initialForm(rule),
  );
  const [search, setSearch] = useState("");
  const bodyRef = useRef<HTMLTextAreaElement>(null);
  const engagements = useQuery({
    queryKey: automationKeys.engagements(slug),
    queryFn: () => automationService.engagements(slug),
    enabled: form.target_kind === "selected" || form.trigger_kind === "payment",
  });
  const save = useAppMutation({
    mutationFn: (body: AutomationRuleInput) =>
      rule
        ? automationService.update(slug, rule.uuid, body)
        : automationService.create(slug, body),
    onSuccess: () => {
      void qc.invalidateQueries({ queryKey: automationKeys.all(slug) });
      appToast.success(t("tenant.automation_saved"));
      onDone();
    },
  });

  const trig = form.trigger;
  const setTrig = (patch: Partial<AutomationTrigger>) =>
    setForm((f) => ({ ...f, trigger: { ...f.trigger, ...patch } }));
  const channels = form.channels as Channel[];
  const hasSms = channels.includes("sms");
  const hasEmail = channels.includes("email");
  const selected = form.target_engagements ?? [];
  const eventInfo = events.find((e) => e.code === trig.event);

  const preview = useMemo(
    () => renderVariables(form.body, SAMPLE_VARS),
    [form.body],
  );
  const subjectPreview = useMemo(
    () => renderVariables(form.subject ?? "", SAMPLE_VARS),
    [form.subject],
  );
  const length = smsLength(preview);

  function toggleChannel(c: Channel, on: boolean) {
    setForm((f) => {
      const next = on
        ? Array.from(new Set([...(f.channels as Channel[]), c]))
        : (f.channels as Channel[]).filter((x) => x !== c);
      const unopened =
        next.includes("sms") && next.includes("email")
          ? f.sms_if_unopened_hours
          : null;
      return { ...f, channels: next, sms_if_unopened_hours: unopened };
    });
  }

  function insertVariable(name: string) {
    const token = `:${name}`;
    const el = bodyRef.current;
    setForm((f) => {
      const start = el?.selectionStart ?? f.body.length;
      const end = el?.selectionEnd ?? f.body.length;
      const before = f.body.slice(0, start);
      const sep = before === "" || before.endsWith(" ") ? "" : " ";
      return {
        ...f,
        body: before + sep + token + f.body.slice(end),
      };
    });
    requestAnimationFrame(() => el?.focus());
  }

  function toggleDay(d: number) {
    const days = trig.days.includes(d)
      ? trig.days.filter((x) => x !== d)
      : [...trig.days, d].sort((a, b) => a - b);
    setTrig({ days });
  }

  function toggleTarget(uuid: string, on: boolean) {
    setForm((f) => {
      const cur = f.target_engagements ?? [];
      return {
        ...f,
        target_engagements: on
          ? Array.from(new Set([...cur, uuid]))
          : cur.filter((x) => x !== uuid),
      };
    });
  }

  function submit() {
    const body: AutomationRuleInput = { ...form };
    if (body.trigger_kind === "payment") {
      body.target_kind = "selected";
    }
    if (body.target_kind === "all") body.target_engagements = [];
    save.mutate(body);
  }

  const engItems = (engagements.data?.items ?? []).filter((e) =>
    e.taxpayer_title
      .toLocaleLowerCase("tr")
      .includes(search.trim().toLocaleLowerCase("tr")),
  );

  return (
    <div className="space-y-6" data-testid="automation-rule-dialog">
      <DialogHeader>
        <DialogTitle>
          {rule
            ? t("tenant.automation_edit_title")
            : t("tenant.automation_new_title")}
        </DialogTitle>
        <DialogDescription>
          {t("tenant.automation_dialog_description")}
        </DialogDescription>
      </DialogHeader>

      {/* 1. Temel bilgiler */}
      <section className="space-y-2">
        <Label htmlFor="automation-name">{t("tenant.automation_name")}</Label>
        <Input
          id="automation-name"
          value={form.name}
          maxLength={120}
          onChange={(e) => setForm((f) => ({ ...f, name: e.target.value }))}
          placeholder={t("tenant.automation_name_placeholder")}
        />
      </section>

      {/* 2. Tetikleyici ve zamanlama */}
      <section className="space-y-3">
        <h3 className="text-sm font-semibold">
          {t("tenant.automation_trigger")}
        </h3>
        <TabStrip aria-label={t("tenant.automation_trigger")}>
          {(["calendar", "event", "payment"] as const).map((k) => (
            <TabStripItem
              key={k}
              selected={form.trigger_kind === k}
              onClick={() =>
                setForm((f) => ({
                  ...f,
                  trigger_kind: k,
                  target_kind: k === "payment" ? "selected" : f.target_kind,
                }))
              }
            >
              {t(`tenant.automation_trigger_${k}`)}
            </TabStripItem>
          ))}
        </TabStrip>

        {form.trigger_kind === "calendar" ? (
          <div className="space-y-3">
            <RadioGroup
              value={trig.mode}
              onValueChange={(v) =>
                setTrig({ mode: v as "days" | "before_due" })
              }
              className="flex flex-wrap gap-4"
            >
              <div className="flex items-center gap-2">
                <RadioGroupItem value="days" id="automation-mode-days" />
                <Label htmlFor="automation-mode-days">
                  {t("tenant.automation_mode_days")}
                </Label>
              </div>
              <div className="flex items-center gap-2">
                <RadioGroupItem
                  value="before_due"
                  id="automation-mode-before"
                />
                <Label htmlFor="automation-mode-before">
                  {t("tenant.automation_mode_before_due")}
                </Label>
              </div>
            </RadioGroup>
            {trig.mode === "days" ? (
              <div className="space-y-1">
                <p className="text-text-2 text-xs">
                  {t("tenant.automation_days_help")}
                </p>
                <div
                  className="grid grid-cols-7 gap-1 sm:grid-cols-[repeat(16,minmax(0,1fr))]"
                  data-testid="automation-days"
                >
                  {DAYS.map((d) => (
                    <Button
                      key={d}
                      type="button"
                      size="sm"
                      variant={trig.days.includes(d) ? "default" : "outline"}
                      aria-pressed={trig.days.includes(d)}
                      onClick={() => toggleDay(d)}
                      className="px-0"
                    >
                      {d}
                    </Button>
                  ))}
                </div>
              </div>
            ) : (
              <div className="flex flex-wrap items-center gap-2 text-sm">
                <span>{t("tenant.automation_before_due_prefix")}</span>
                <Input
                  aria-label={t("tenant.automation_before_due_days")}
                  className="w-20"
                  inputMode="numeric"
                  value={String(trig.before_due_days)}
                  onChange={(e) =>
                    setTrig({ before_due_days: toInt(e.target.value, 0, 20) })
                  }
                />
                <span>{t("tenant.automation_before_due_suffix")}</span>
              </div>
            )}
            <div className="grid gap-3 sm:grid-cols-3">
              <div className="space-y-1">
                <Label htmlFor="automation-tax">
                  {t("tenant.automation_tax")}
                </Label>
                <NativeSelect
                  id="automation-tax"
                  value={trig.tax}
                  onChange={(e) =>
                    setTrig({ tax: e.target.value as AutomationTrigger["tax"] })
                  }
                  wrapperClassName="w-full"
                >
                  <NativeSelectOption value="">
                    {t("tenant.automation_tax_none")}
                  </NativeSelectOption>
                  <NativeSelectOption value="kdv">
                    {t("tenant.automation_tax_kdv")}
                  </NativeSelectOption>
                  <NativeSelectOption value="muhtasar">
                    {t("tenant.automation_tax_muhtasar")}
                  </NativeSelectOption>
                  <NativeSelectOption value="gecici">
                    {t("tenant.automation_tax_gecici")}
                  </NativeSelectOption>
                </NativeSelect>
              </div>
              <div className="space-y-1">
                <Label htmlFor="automation-hour">
                  {t("tenant.automation_hour")}
                </Label>
                <NativeSelect
                  id="automation-hour"
                  value={String(trig.hour)}
                  onChange={(e) => setTrig({ hour: Number(e.target.value) })}
                  wrapperClassName="w-full"
                >
                  {HOURS.map((h) => (
                    <NativeSelectOption key={h} value={String(h)}>
                      {String(h).padStart(2, "0")}:00
                    </NativeSelectOption>
                  ))}
                </NativeSelect>
              </div>
              <div className="flex items-end gap-2 pb-1">
                <Switch
                  id="automation-current"
                  checked={trig.current_period}
                  onCheckedChange={(v) => setTrig({ current_period: v })}
                />
                <Label htmlFor="automation-current" className="text-sm">
                  {t("tenant.automation_current_period")}
                </Label>
              </div>
            </div>
          </div>
        ) : null}

        {form.trigger_kind === "event" ? (
          <div className="space-y-3">
            <div className="space-y-1">
              <Label htmlFor="automation-event">
                {t("tenant.automation_event")}
              </Label>
              <NativeSelect
                id="automation-event"
                value={trig.event}
                onChange={(e) => setTrig({ event: e.target.value })}
                wrapperClassName="w-full"
              >
                <NativeSelectOption value="">
                  {t("tenant.automation_event_pick")}
                </NativeSelectOption>
                {events.map((ev) => (
                  <NativeSelectOption
                    key={ev.code}
                    value={ev.code}
                    disabled={!ev.available}
                  >
                    {t(`tenant.automation_event_${ev.code.replace(".", "_")}`)}
                    {ev.available ? "" : ` (${t("tenant.automation_soon")})`}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <div className="grid gap-3 sm:grid-cols-3">
              <div className="space-y-1">
                <Label htmlFor="automation-delay-min">
                  {t("tenant.automation_delay_min")}
                </Label>
                <Input
                  id="automation-delay-min"
                  inputMode="numeric"
                  value={String(trig.delay_min_hours)}
                  onChange={(e) =>
                    setTrig({ delay_min_hours: toInt(e.target.value, 0, 720) })
                  }
                />
              </div>
              <div className="space-y-1">
                <Label htmlFor="automation-delay-max">
                  {t("tenant.automation_delay_max")}
                </Label>
                <Input
                  id="automation-delay-max"
                  inputMode="numeric"
                  value={String(trig.delay_max_hours)}
                  onChange={(e) =>
                    setTrig({ delay_max_hours: toInt(e.target.value, 0, 720) })
                  }
                />
              </div>
              {eventInfo?.payment ? (
                <div className="space-y-1">
                  <Label htmlFor="automation-resend">
                    {t("tenant.automation_resend")}
                  </Label>
                  <Input
                    id="automation-resend"
                    inputMode="numeric"
                    value={String(trig.resend_after_days)}
                    onChange={(e) =>
                      setTrig({
                        resend_after_days: toInt(e.target.value, 0, 60),
                      })
                    }
                  />
                </div>
              ) : null}
            </div>
            <p className="text-text-2 text-xs">
              {t("tenant.automation_delay_help")}
            </p>
          </div>
        ) : null}

        {form.trigger_kind === "payment" ? (
          <div className="grid gap-3 sm:grid-cols-2">
            <div className="space-y-1 sm:col-span-2">
              <Label htmlFor="automation-payment-taxpayer">
                {t("tenant.automation_payment_taxpayer")}
              </Label>
              <NativeSelect
                id="automation-payment-taxpayer"
                value={selected[0] ?? ""}
                onChange={(e) =>
                  setForm((f) => ({
                    ...f,
                    target_kind: "selected",
                    target_engagements: e.target.value ? [e.target.value] : [],
                  }))
                }
                wrapperClassName="w-full"
              >
                <NativeSelectOption value="">
                  {t("tenant.automation_payment_taxpayer_pick")}
                </NativeSelectOption>
                {(engagements.data?.items ?? []).map((e) => (
                  <NativeSelectOption key={e.uuid} value={e.uuid}>
                    {e.taxpayer_title}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <div className="space-y-1">
              <Label htmlFor="automation-payment-date">
                {t("tenant.automation_payment_date")}
              </Label>
              <DatePicker
                id="automation-payment-date"
                value={trig.payment_date}
                onChange={(v) => setTrig({ payment_date: v })}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="automation-payment-amount">
                {t("tenant.automation_payment_amount")}
              </Label>
              <Input
                id="automation-payment-amount"
                inputMode="decimal"
                value={trig.payment_amount}
                onChange={(e) => setTrig({ payment_amount: e.target.value })}
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="automation-payment-before">
                {t("tenant.automation_payment_days_before")}
              </Label>
              <Input
                id="automation-payment-before"
                inputMode="numeric"
                value={String(trig.payment_days_before)}
                onChange={(e) =>
                  setTrig({
                    payment_days_before: toInt(e.target.value, 0, 30),
                  })
                }
              />
            </div>
            <div className="space-y-1">
              <Label htmlFor="automation-payment-hour">
                {t("tenant.automation_hour")}
              </Label>
              <NativeSelect
                id="automation-payment-hour"
                value={String(trig.hour)}
                onChange={(e) => setTrig({ hour: Number(e.target.value) })}
                wrapperClassName="w-full"
              >
                {HOURS.map((h) => (
                  <NativeSelectOption key={h} value={String(h)}>
                    {String(h).padStart(2, "0")}:00
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
          </div>
        ) : null}
      </section>

      {/* 3. Hedef */}
      {form.trigger_kind !== "payment" ? (
        <section className="space-y-3">
          <h3 className="text-sm font-semibold">
            {t("tenant.automation_target")}
          </h3>
          <RadioGroup
            value={form.target_kind}
            onValueChange={(v) =>
              setForm((f) => ({
                ...f,
                target_kind: v as AutomationRuleInput["target_kind"],
              }))
            }
            className="flex flex-wrap gap-4"
          >
            <div className="flex items-center gap-2">
              <RadioGroupItem value="all" id="automation-target-all" />
              <Label htmlFor="automation-target-all">
                {t("tenant.automation_target_all")}
              </Label>
            </div>
            {/* TODO(#780 B): mükellef grupları paralel PR'da; o zamana dek kapalı. */}
            <div className="flex items-center gap-2">
              <RadioGroupItem
                value="group"
                id="automation-target-group"
                disabled
              />
              <Label htmlFor="automation-target-group" className="text-text-3">
                {t("tenant.automation_target_group")} (
                {t("tenant.automation_soon")})
              </Label>
            </div>
            <div className="flex items-center gap-2">
              <RadioGroupItem
                value="selected"
                id="automation-target-selected"
              />
              <Label htmlFor="automation-target-selected">
                {t("tenant.automation_target_selected")}
              </Label>
            </div>
          </RadioGroup>
          {form.target_kind === "selected" ? (
            <div className="border-border space-y-2 rounded-md border p-3">
              <Input
                aria-label={t("tenant.automation_target_search")}
                placeholder={t("tenant.automation_target_search")}
                value={search}
                onChange={(e) => setSearch(e.target.value)}
              />
              <p className="text-text-2 text-xs">
                {t("tenant.automation_target_count", {
                  count: String(selected.length),
                })}
              </p>
              <ul className="max-h-48 space-y-1 overflow-y-auto">
                {engItems.map((e) => (
                  <li key={e.uuid} className="flex items-center gap-2 text-sm">
                    <Checkbox
                      id={`automation-eng-${e.uuid}`}
                      checked={selected.includes(e.uuid)}
                      onCheckedChange={(v) => toggleTarget(e.uuid, v === true)}
                    />
                    <Label htmlFor={`automation-eng-${e.uuid}`}>
                      {e.taxpayer_title}
                    </Label>
                  </li>
                ))}
              </ul>
            </div>
          ) : null}
          <div className="flex items-center gap-2">
            <Switch
              id="automation-only-due"
              checked={form.only_due ?? false}
              onCheckedChange={(v) => setForm((f) => ({ ...f, only_due: v }))}
            />
            <Label htmlFor="automation-only-due" className="text-sm">
              {t("tenant.automation_only_due")}
            </Label>
          </div>
        </section>
      ) : null}

      {/* 4. Kanal */}
      <section className="space-y-3">
        <h3 className="text-sm font-semibold">
          {t("tenant.automation_channels")}
        </h3>
        <div className="flex flex-wrap gap-4">
          {(["sms", "email", "inapp"] as const).map((c) => (
            <div key={c} className="flex items-center gap-2">
              <Checkbox
                id={`automation-channel-${c}`}
                checked={channels.includes(c)}
                onCheckedChange={(v) => toggleChannel(c, v === true)}
              />
              <Label htmlFor={`automation-channel-${c}`}>
                {t(`tenant.automation_channel_${c}`)}
              </Label>
            </div>
          ))}
          <div className="flex items-center gap-2">
            <Checkbox id="automation-channel-whatsapp" disabled />
            <Label
              htmlFor="automation-channel-whatsapp"
              className="text-text-3"
            >
              {t("tenant.automation_channel_whatsapp")} (
              {t("tenant.automation_soon")})
            </Label>
          </div>
        </div>
        {hasSms && hasEmail ? (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <Checkbox
              id="automation-unopened"
              checked={form.sms_if_unopened_hours != null}
              onCheckedChange={(v) =>
                setForm((f) => ({
                  ...f,
                  sms_if_unopened_hours: v === true ? 24 : null,
                }))
              }
            />
            <Label htmlFor="automation-unopened">
              {t("tenant.automation_unopened")}
            </Label>
            {form.sms_if_unopened_hours != null ? (
              <>
                <Input
                  aria-label={t("tenant.automation_unopened_hours")}
                  className="w-20"
                  inputMode="numeric"
                  value={String(form.sms_if_unopened_hours)}
                  onChange={(e) =>
                    setForm((f) => ({
                      ...f,
                      sms_if_unopened_hours: toInt(e.target.value, 1, 168),
                    }))
                  }
                />
                <span>{t("tenant.automation_unopened_hours")}</span>
              </>
            ) : null}
          </div>
        ) : null}
      </section>

      {/* 5. Metin */}
      <section className="space-y-3">
        <h3 className="text-sm font-semibold">{t("tenant.automation_text")}</h3>
        {hasEmail ? (
          <div className="space-y-1">
            <Label htmlFor="automation-subject">
              {t("tenant.automation_subject")}
            </Label>
            <Input
              id="automation-subject"
              value={form.subject ?? ""}
              maxLength={200}
              onChange={(e) =>
                setForm((f) => ({ ...f, subject: e.target.value }))
              }
            />
          </div>
        ) : null}
        <div className="space-y-1">
          <p className="text-text-2 text-xs">
            {t("tenant.automation_variables_help")}
          </p>
          <div className="flex flex-wrap gap-1" data-testid="automation-vars">
            {AUTOMATION_VARIABLES.map((v) => (
              <Button
                key={v}
                type="button"
                size="sm"
                variant="secondary"
                className="rounded-full font-mono"
                onClick={() => insertVariable(v)}
              >
                :{v}
              </Button>
            ))}
          </div>
        </div>
        <div className="space-y-1">
          <Label htmlFor="automation-body">{t("tenant.automation_body")}</Label>
          <Textarea
            id="automation-body"
            ref={bodyRef}
            rows={4}
            maxLength={2000}
            value={form.body}
            onChange={(e) => setForm((f) => ({ ...f, body: e.target.value }))}
          />
          {hasSms ? (
            <p
              className="text-text-2 text-xs"
              data-testid="automation-sms-length"
            >
              {t("tenant.automation_sms_length", {
                chars: String(length.chars),
                parts: String(length.parts),
              })}
            </p>
          ) : null}
        </div>
        <div className="bg-surface-2 space-y-1 rounded-md p-3 text-sm">
          <p className="text-text-2 text-xs font-medium">
            {t("tenant.automation_preview")}
          </p>
          {hasEmail && subjectPreview ? (
            <p className="font-medium">{subjectPreview}</p>
          ) : null}
          <p className="whitespace-pre-wrap" data-testid="automation-preview">
            {preview || t("tenant.automation_preview_empty")}
          </p>
        </div>
      </section>

      {/* 6. Onay modu ve 7. etkin */}
      <section className="space-y-3">
        <h3 className="text-sm font-semibold">
          {t("tenant.automation_approval")}
        </h3>
        <RadioGroup
          value={form.approval_mode ?? "auto"}
          onValueChange={(v) =>
            setForm((f) => ({ ...f, approval_mode: v as "auto" | "draft" }))
          }
          className="flex flex-col gap-2"
        >
          <div className="flex items-center gap-2">
            <RadioGroupItem value="auto" id="automation-approval-auto" />
            <Label htmlFor="automation-approval-auto">
              {t("tenant.automation_approval_auto")}
            </Label>
          </div>
          <div className="flex items-center gap-2">
            <RadioGroupItem value="draft" id="automation-approval-draft" />
            <Label htmlFor="automation-approval-draft">
              {t("tenant.automation_approval_draft")}
            </Label>
          </div>
        </RadioGroup>
        <div className="flex items-center gap-2">
          <Switch
            id="automation-active"
            checked={form.active ?? false}
            onCheckedChange={(v) => setForm((f) => ({ ...f, active: v }))}
          />
          <Label htmlFor="automation-active">
            {t("tenant.automation_active")}
          </Label>
          {form.active ? (
            <StatusPill tone="ok">
              {t("tenant.automation_status_active")}
            </StatusPill>
          ) : (
            <StatusPill tone="neutral">
              {t("tenant.automation_status_passive")}
            </StatusPill>
          )}
        </div>
      </section>

      <DialogFooter>
        <Button type="button" variant="outline" onClick={onDone}>
          {t("common.cancel")}
        </Button>
        <Button
          type="button"
          onClick={submit}
          disabled={save.isPending || form.name.trim() === ""}
        >
          {t("tenant.automation_save")}
        </Button>
      </DialogFooter>
    </div>
  );
}
