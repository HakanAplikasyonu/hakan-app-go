"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";
import { toast } from "sonner";

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
import { FieldLegend, FieldSet } from "@/components/ui/field";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import {
  bankCoverageKeys,
  bankCoverageService,
} from "../services/bank-coverage.service";
import {
  DOC_CHANNELS,
  type DocChannel,
  useDocRequestChannelsOn,
} from "./doc-request-channels";

const REMIND_DAYS = [0, 1, 2, 3, 5, 7];

/** Yerel takvimde bugünden n gün sonrası (YYYY-MM-DD). */
function dateAfter(days: number) {
  const d = new Date();
  d.setDate(d.getDate() + days);
  const p = (n: number) => String(n).padStart(2, "0");
  return `${d.getFullYear()}-${p(d.getMonth() + 1)}-${p(d.getDate())}`;
}

/** İstenecek eksik ekstre: hesap, ay ve mükellefin göreceği başlık. */
export type StatementAsk = {
  cash_box_uuid: string;
  period: string;
  title: string;
};

/**
 * Eksik ekstreyi isteme (Oğuz'un geri bildirimi): belge isteği akışının
 * aynısı; başlık hazırdır ("Akbank TRY hesabı – Ekim 2026 ekstresi"), müşavir
 * son tarihi, kanalları ve hatırlatmayı seçer. `asks` boşsa ve `all`
 * verilirse mükellefin bütün eksik ekstreleri istenir.
 */
export function StatementRequestDialog({
  engagement,
  asks,
  all,
  taxpayer,
  open,
  onOpenChange,
}: {
  engagement: string;
  asks: StatementAsk[];
  /** Mükellefin bütün eksikleri (toplu istek). */
  all?: boolean;
  /** Ofis listesinde mükellefin adı. */
  taxpayer?: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const channelsOn = useDocRequestChannelsOn();
  const [due, setDue] = useState(() => dateAfter(5));
  const [channels, setChannels] = useState<DocChannel[]>(["push"]);
  const [remind, setRemind] = useState("");

  const send = useMutation({
    mutationFn: () =>
      bankCoverageService.request(slug, engagement, {
        items: all
          ? []
          : asks.map((a) => ({
              cash_box_uuid: a.cash_box_uuid,
              period: a.period,
            })),
        due_date: due,
        ...(channelsOn
          ? {
              channels,
              ...(remind !== "" && channels.length > 0
                ? { remind_days_before: Number(remind) }
                : {}),
            }
          : {}),
      }),
    onSuccess: (r) => {
      toast.success(
        t("office.bank.coverage.requested", {
          created: r.created.length,
          existing: r.existing.length,
        }),
      );
      onOpenChange(false);
      for (const key of [
        bankCoverageKeys.status(slug, engagement),
        bankCoverageKeys.gaps(slug),
        ["tenant", slug.toLowerCase(), "doc-tasks", engagement],
      ])
        void qc.invalidateQueries({ queryKey: key });
    },
    onError: (e) => {
      const code = isApiError(e) ? e.code : "";
      toast.error(
        code === "STATEMENT_NOT_MISSING" ||
          code === "BANK_STATEMENT_TRACKING_DISABLED"
          ? t(`office.bank.coverage.error_${code}`)
          : isApiError(e) && code === "VALIDATION_ERROR"
            ? t("office.bank.coverage.error_input")
            : t("office.bank.coverage.request_failed"),
      );
    },
  });

  const toggle = (c: DocChannel, on: boolean) =>
    setChannels((prev) =>
      DOC_CHANNELS.filter((x) => (x === c ? on : prev.includes(x))),
    );
  const shown = asks.slice(0, 6);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent data-testid="bank-statement-request">
        <DialogHeader>
          <DialogTitle>
            {!all
              ? t("office.bank.coverage.request_title")
              : taxpayer
                ? t("office.bank.coverage.request_all_title_for", { taxpayer })
                : t("office.bank.coverage.request_all_title")}
          </DialogTitle>
          <DialogDescription>
            {t("office.bank.coverage.request_description")}
          </DialogDescription>
        </DialogHeader>
        <form
          className="space-y-3"
          onSubmit={(e) => {
            e.preventDefault();
            send.mutate();
          }}
        >
          {shown.length > 0 ? (
            <ul
              className="space-y-1 text-sm"
              aria-label={t("office.bank.coverage.request_list")}
            >
              {shown.map((a) => (
                <li
                  key={`${a.cash_box_uuid}-${a.period}`}
                  className="font-medium"
                >
                  {a.title}
                </li>
              ))}
              {asks.length > shown.length ? (
                <li className="text-text-2">
                  {t("office.bank.coverage.request_more", {
                    count: asks.length - shown.length,
                  })}
                </li>
              ) : null}
            </ul>
          ) : null}
          <div className="max-w-xs space-y-1.5">
            <Label htmlFor="statement-request-due">
              {t("office.doc_request.due_label")}
            </Label>
            <DatePicker
              id="statement-request-due"
              value={due}
              onChange={(v) => setDue(v || dateAfter(5))}
            />
          </div>
          {channelsOn ? (
            <>
              <FieldSet className="space-y-2">
                <FieldLegend variant="label" className="mb-0">
                  {t("office.doc_request.channels_label")}
                </FieldLegend>
                <div className="flex flex-wrap gap-4">
                  {DOC_CHANNELS.map((c) => (
                    <Label
                      key={c}
                      className="flex items-center gap-2 text-sm font-normal"
                      htmlFor={`statement-request-channel-${c}`}
                    >
                      <Checkbox
                        id={`statement-request-channel-${c}`}
                        checked={channels.includes(c)}
                        onCheckedChange={(v) => toggle(c, v === true)}
                      />
                      {t(`office.doc_request.channel_${c}`)}
                    </Label>
                  ))}
                </div>
              </FieldSet>
              <div className="max-w-xs space-y-1.5">
                <Label htmlFor="statement-request-remind">
                  {t("office.doc_request.remind_label")}
                </Label>
                <NativeSelect
                  id="statement-request-remind"
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
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={send.isPending || (channelsOn && channels.length === 0)}
            >
              {t("office.bank.coverage.request_submit")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
