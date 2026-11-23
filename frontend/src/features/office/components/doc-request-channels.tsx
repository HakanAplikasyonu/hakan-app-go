"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

import { StatusPill, type StatusTone } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { DatePicker } from "@/components/ui/date-picker";
import { FieldLegend, FieldSet } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Textarea } from "@/components/ui/textarea";
import { permissions } from "@/config/permissions";
import { useTenantSettings } from "@/features/tenant/hooks/use-tenant-admin";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { tenantRequest } from "@/lib/api/tenant-request";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

/** organizations.settings anahtarı; yoksa açık (backend ile aynı). */
export const DOC_REQUEST_CHANNELS_SETTING = "doc_request_channels";

export const DOC_CHANNELS = ["push", "sms", "email"] as const;
export type DocChannel = (typeof DOC_CHANNELS)[number];

export type DocTaskDelivery = {
  channel: DocChannel;
  kind: "request" | "reminder";
  status: string;
  code?: string;
  at: string | null;
};

/** Belge isteğinin kanal alanları (ofis görünümü). */
export type DocTaskChannelFields = {
  uuid: string;
  status: string;
  channels?: DocChannel[];
  remind_days_before?: number;
  remind_at?: string | null;
  channel_reminded_at?: string | null;
  deliveries?: DocTaskDelivery[];
};

const REMIND_DAYS = [0, 1, 2, 3, 5, 7];

const deliveryTone: Record<string, StatusTone> = {
  sent: "ok",
  delivered: "ok",
  dry_run: "neutral",
  pending: "info",
  uncertain: "warn",
  skipped: "warn",
  failed: "danger",
};

/** Ofis ayarı: belge isteğinde kanal seçimi ve hatırlatma açık mı. */
export function useDocRequestChannelsOn(): boolean {
  const { slug } = useTenant();
  const query = useTenantSettings(slug);
  const settings = (query.data?.settings ?? {}) as Record<string, unknown>;
  return settings[DOC_REQUEST_CHANNELS_SETTING] !== false;
}

function tasksKey(slug: string, engagement: string) {
  return ["tenant", slug.toLowerCase(), "doc-tasks", engagement];
}

function fail(t: (k: string) => string, err: unknown) {
  if (isApiError(err) && err.code === "RECENTLY_REMINDED") {
    toast.error(t("office.doc_request.recently_reminded"));
    return;
  }
  toast.error(isApiError(err) ? err.message : t("office.api.load_error"));
}

function dayText(iso: string, locale: string) {
  return new Date(iso).toLocaleDateString(locale === "tr" ? "tr-TR" : "en-GB", {
    timeZone: "Europe/Istanbul",
  });
}

/** Yerel takvimde bugünden n gün sonrası (YYYY-MM-DD). */
function dateAfter(days: number) {
  const d = new Date();
  d.setDate(d.getDate() + days);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/**
 * Mükellef özeti → "Belge iste" (Oğuz'un geri bildirimi): ofis mükelleften
 * belge ister; mükellefe hangi kanallardan haber verileceğini (uygulama
 * bildirimi, SMS, e-posta), son tarihi ve isteğe bağlı hatırlatmayı seçer.
 * Her gönderim iletişim geçmişine yazılır.
 */
export function DocRequestForm({
  engagement,
  period,
}: {
  engagement: string;
  period: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const queryClient = useQueryClient();
  const channelsOn = useDocRequestChannelsOn();
  const [open, setOpen] = useState(false);
  const [title, setTitle] = useState("");
  const [note, setNote] = useState("");
  const [due, setDue] = useState(() => dateAfter(7));
  const [channels, setChannels] = useState<DocChannel[]>(["push"]);
  const [remind, setRemind] = useState("");

  const create = useMutation({
    mutationFn: () =>
      tenantRequest<DocTaskChannelFields>(
        slug,
        "POST",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/document-tasks`,
        {
          body: {
            title: title.trim(),
            note: note.trim(),
            due_date: due,
            // İstek seçili dönemin listesinde görünsün.
            period,
            ...(channelsOn
              ? {
                  channels,
                  ...(remind !== "" && channels.length > 0
                    ? { remind_days_before: Number(remind) }
                    : {}),
                }
              : {}),
          },
          silent: true,
        },
      ),
    onSuccess: () => {
      toast.success(t("office.doc_request.created"));
      setOpen(false);
      setTitle("");
      setNote("");
      setRemind("");
      return queryClient.invalidateQueries({
        queryKey: tasksKey(slug, engagement),
      });
    },
    onError: (err) => fail(t, err),
  });

  if (!can(permissions.tenant.docTasks.write)) return null;
  if (!open) {
    return (
      <Button
        size="sm"
        variant="outline"
        onClick={() => setOpen(true)}
        data-testid="doc-request-open"
      >
        {t("office.doc_request.open")}
      </Button>
    );
  }
  const toggle = (c: DocChannel, on: boolean) =>
    setChannels((prev) =>
      DOC_CHANNELS.filter((x) => (x === c ? on : prev.includes(x))),
    );
  return (
    <form
      className="border-line mb-3 space-y-3 rounded-lg border p-3"
      data-testid="doc-request-form"
      onSubmit={(e) => {
        e.preventDefault();
        create.mutate();
      }}
    >
      <p className="text-text-2 text-sm">{t("office.doc_request.hint")}</p>
      <div className="space-y-1.5">
        <Label htmlFor="doc-request-title">
          {t("office.doc_request.title_label")}
        </Label>
        <Input
          id="doc-request-title"
          value={title}
          maxLength={200}
          required
          onChange={(e) => setTitle(e.target.value)}
          placeholder={t("office.doc_request.title_placeholder")}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="doc-request-note">
          {t("office.doc_request.note_label")}
        </Label>
        <Textarea
          id="doc-request-note"
          value={note}
          maxLength={1000}
          rows={2}
          onChange={(e) => setNote(e.target.value)}
        />
      </div>
      <div className="max-w-xs space-y-1.5">
        <Label htmlFor="doc-request-due">
          {t("office.doc_request.due_label")}
        </Label>
        <DatePicker
          id="doc-request-due"
          value={due}
          onChange={(v) => setDue(v || dateAfter(7))}
        />
      </div>
      {channelsOn ? (
        <>
          <FieldSet className="space-y-2">
            <FieldLegend variant="label" className="mb-0">
              {t("office.doc_request.channels_label")}
            </FieldLegend>
            <p className="text-text-2 text-xs">
              {t("office.doc_request.channels_hint")}
            </p>
            <div className="flex flex-wrap gap-4">
              {DOC_CHANNELS.map((c) => (
                <Label
                  key={c}
                  className="flex items-center gap-2 text-sm font-normal"
                  htmlFor={`doc-request-channel-${c}`}
                >
                  <Checkbox
                    id={`doc-request-channel-${c}`}
                    data-testid={`doc-request-channel-${c}`}
                    checked={channels.includes(c)}
                    onCheckedChange={(v) => toggle(c, v === true)}
                  />
                  {t(`office.doc_request.channel_${c}`)}
                </Label>
              ))}
            </div>
          </FieldSet>
          <div className="max-w-xs space-y-1.5">
            <Label htmlFor="doc-request-remind">
              {t("office.doc_request.remind_label")}
            </Label>
            <NativeSelect
              id="doc-request-remind"
              value={remind}
              disabled={channels.length === 0}
              onChange={(e) => setRemind(e.target.value)}
            >
              <NativeSelectOption value="">
                {t("office.doc_request.remind_none")}
              </NativeSelectOption>
              {REMIND_DAYS.map((d) => (
                <NativeSelectOption key={d} value={String(d)}>
                  {d === 0
                    ? t("office.doc_request.remind_due_day")
                    : t("office.doc_request.remind_days", { count: d })}
                </NativeSelectOption>
              ))}
            </NativeSelect>
          </div>
        </>
      ) : (
        <p className="text-text-2 text-xs">
          {t("office.doc_request.channels_off")}
        </p>
      )}
      <div className="flex justify-end gap-2">
        <Button
          type="button"
          size="sm"
          variant="ghost"
          onClick={() => setOpen(false)}
        >
          {t("office.doc_request.cancel")}
        </Button>
        <Button
          type="submit"
          size="sm"
          disabled={
            create.isPending ||
            title.trim() === "" ||
            (channelsOn && channels.length === 0)
          }
          data-testid="doc-request-submit"
        >
          {t("office.doc_request.submit")}
        </Button>
      </div>
    </form>
  );
}

/**
 * Belge isteği satırında kanal durumu (her kanalın son gönderimi),
 * hatırlatma günü ve "Hatırlat" düğmesi.
 */
export function DocTaskChannels({
  engagement,
  task,
}: {
  engagement: string;
  task: DocTaskChannelFields;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const queryClient = useQueryClient();
  const channelsOn = useDocRequestChannelsOn();
  const remind = useMutation({
    mutationFn: () =>
      tenantRequest<DocTaskChannelFields>(
        slug,
        "POST",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/document-tasks/${encodeURIComponent(task.uuid)}/send`,
        { body: { reminder: true }, silent: true },
      ),
    onSuccess: () => {
      toast.success(t("office.doc_request.reminded"));
      return queryClient.invalidateQueries({
        queryKey: tasksKey(slug, engagement),
      });
    },
    onError: (err) => fail(t, err),
  });
  const deliveries = task.deliveries ?? [];
  const canRemind =
    channelsOn &&
    task.status === "open" &&
    can(permissions.tenant.docTasks.write);
  if (deliveries.length === 0 && task.remind_days_before == null && !canRemind)
    return null;
  return (
    <span
      className="flex flex-wrap items-center gap-1.5 text-xs"
      data-testid="doc-task-channels"
    >
      {deliveries.map((d) => (
        <span
          key={d.channel}
          data-testid={`doc-task-delivery-${d.channel}`}
          data-status={d.status}
        >
          <StatusPill tone={deliveryTone[d.status] ?? "neutral"}>
            {t(`office.doc_request.channel_${d.channel}`)}:{" "}
            {t(`office.doc_request.status_${d.status}`)}
            {d.kind === "reminder"
              ? ` (${t("office.doc_request.kind_reminder")})`
              : ""}
          </StatusPill>
        </span>
      ))}
      {task.remind_days_before != null ? (
        <span className="text-text-3" data-testid="doc-task-reminder">
          {task.remind_at
            ? t("office.doc_request.reminder_at", {
                date: dayText(task.remind_at, locale),
              })
            : t("office.doc_request.reminder_done")}
        </span>
      ) : null}
      {canRemind ? (
        <Button
          size="sm"
          variant="link"
          className="h-auto p-0 text-xs"
          disabled={remind.isPending}
          onClick={() => remind.mutate()}
          data-testid="doc-task-remind"
        >
          {t("office.doc_request.remind_now")}
        </Button>
      ) : null}
    </span>
  );
}
