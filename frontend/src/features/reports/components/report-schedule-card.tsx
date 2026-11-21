"use client";

import { CalendarClock } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import {
  useReportSchedule,
  useSaveReportSchedule,
} from "@/features/reports/hooks/use-deliveries";
import type { ReportSchedule } from "@/features/reports/services/reports.service";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { monthLabel } from "./report-range-picker";

// 29-31: ay kısaysa son gün gönderilir (#780).
const DAYS = Array.from({ length: 31 }, (_, i) => i + 1);

function dateLabel(iso: string, locale: string) {
  const [y, m, d] = iso.split("-").map(Number);
  return new Date(Date.UTC(y, m - 1, d)).toLocaleDateString(
    locale === "tr" ? "tr-TR" : "en-GB",
    { day: "numeric", month: "long", year: "numeric", timeZone: "UTC" },
  );
}

function ScheduleForm({
  engagement,
  initial,
  canWrite,
}: {
  engagement: string;
  initial: ReportSchedule;
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const save = useSaveReportSchedule(slug, engagement);
  const [enabled, setEnabled] = useState(initial.enabled);
  const [day, setDay] = useState(initial.day_of_month || 1);
  const [recipient, setRecipient] = useState(initial.recipient ?? "");
  const [error, setError] = useState<string | null>(null);
  const dirty =
    enabled !== initial.enabled ||
    day !== initial.day_of_month ||
    recipient.trim() !== (initial.recipient ?? "");

  const onSave = () => {
    setError(null);
    save.mutate(
      {
        enabled,
        day_of_month: day,
        recipient: recipient.trim() || undefined,
      },
      {
        onSuccess: () => toast.success(t("reports.schedule.saved")),
        onError: (e) =>
          setError(
            e instanceof Error && e.message
              ? e.message
              : t("reports.schedule.save_error"),
          ),
      },
    );
  };

  return (
    <div className="space-y-4">
      <div className="flex items-start gap-3">
        <Switch
          id="report-schedule-enabled"
          checked={enabled}
          onCheckedChange={setEnabled}
          disabled={!canWrite}
        />
        <div className="space-y-0.5">
          <Label htmlFor="report-schedule-enabled">
            {t("reports.schedule.enabled")}
          </Label>
          <p className="text-text-2 text-xs">{t("reports.schedule.hint")}</p>
        </div>
      </div>
      <div className="grid gap-3 sm:grid-cols-2">
        <div className="space-y-1.5">
          <Label htmlFor="report-schedule-day">
            {t("reports.schedule.day")}
          </Label>
          <Select
            value={String(day)}
            onValueChange={(v) => setDay(Number(v))}
            disabled={!canWrite}
          >
            <SelectTrigger id="report-schedule-day">
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {DAYS.map((d) => (
                <SelectItem key={d} value={String(d)}>
                  {t(
                    d > 28
                      ? "reports.schedule.day_option_late"
                      : "reports.schedule.day_option",
                    { day: d },
                  )}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className="space-y-1.5">
          <Label htmlFor="report-schedule-recipient">
            {t("reports.send_recipient")}
          </Label>
          <Input
            id="report-schedule-recipient"
            type="email"
            value={recipient}
            placeholder={t("reports.send_recipient_placeholder")}
            onChange={(e) => setRecipient(e.target.value)}
            disabled={!canWrite}
          />
        </div>
      </div>
      {initial.enabled && initial.next_run && initial.next_period ? (
        <p className="text-text-2 text-sm">
          {t("reports.schedule.next", {
            date: dateLabel(initial.next_run, locale),
            period: monthLabel(initial.next_period, locale),
          })}
        </p>
      ) : null}
      {initial.last_error ? (
        <p className="text-danger text-sm">
          {t("reports.schedule.last_error", { error: initial.last_error })}
        </p>
      ) : null}
      {error ? (
        <p role="alert" className="text-danger text-sm">
          {error}
        </p>
      ) : null}
      {canWrite ? (
        <Button onClick={onSave} disabled={!dirty || save.isPending}>
          {save.isPending
            ? t("reports.schedule.saving")
            : t("reports.schedule.save")}
        </Button>
      ) : null}
    </div>
  );
}

/** Aylık raporun kendiliğinden gönderilmesi (#386, ay kapanınca). */
export function ReportScheduleCard({
  engagement,
  canWrite,
}: {
  engagement: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const q = useReportSchedule(slug, engagement);
  return (
    <Card>
      <CardHeader className="flex flex-row items-center gap-2">
        <CalendarClock aria-hidden className="text-text-2 size-4" />
        <CardTitle className="text-base">
          {t("reports.schedule.title")}
        </CardTitle>
      </CardHeader>
      <CardContent>
        {q.data ? (
          <ScheduleForm
            key={JSON.stringify(q.data)}
            engagement={engagement}
            initial={q.data}
            canWrite={canWrite}
          />
        ) : q.isError ? (
          <p className="text-danger text-sm">{t("reports.error_title")}</p>
        ) : (
          <Skeleton className="h-28 w-full" />
        )}
      </CardContent>
    </Card>
  );
}
