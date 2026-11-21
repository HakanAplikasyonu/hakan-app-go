"use client";

import { AlertTriangle, RefreshCw, Send } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import {
  useRunVATWatch,
  useSendVATNotice,
  useVATNotices,
  useVATWatchEvents,
} from "@/features/reports/hooks/use-vat-notices";
import type { VATNotice } from "@/features/reports/services/vat-notice.service";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { monthLabel } from "./report-range-picker";

function money(v: string, locale: string) {
  const n = Number(v);
  return new Intl.NumberFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    style: "currency",
    currency: "TRY",
  }).format(Number.isFinite(n) ? n : 0);
}

function when(iso: string | null, locale: string) {
  if (!iso) return null;
  return new Date(iso).toLocaleString(locale === "tr" ? "tr-TR" : "en-GB", {
    day: "numeric",
    month: "short",
    hour: "2-digit",
    minute: "2-digit",
  });
}

function NoticeRow({ n }: { n: VATNotice }) {
  const { t, locale } = useLocale();
  return (
    <li className="border-border space-y-1 rounded-md border p-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <span className="font-medium">{monthLabel(n.period, locale)}</span>
        <Badge variant={n.outcome === "payment" ? "warning" : "success"}>
          {n.outcome === "payment"
            ? t("reports.vat_notice.outcome_payment", {
                amount: money(n.payable, locale),
              })
            : t("reports.vat_notice.outcome_no_payment")}
        </Badge>
        <span className="text-text-2 text-xs">
          {t("reports.vat_notice.sent_by", {
            name: n.sent_by_name || "—",
            time: when(n.created_at, locale) ?? "",
          })}
        </span>
      </div>
      {n.message ? (
        <p className="text-text-2 text-xs italic">“{n.message}”</p>
      ) : null}
    </li>
  );
}

/**
 * Mükellefe tek tıkla KDV bildirimi (#780 B): "bu ay ödeme çıkacak /
 * çıkmayacak". Altında mükellefe ne söylendiği ve ne zaman listelenir.
 * Kanal bazında gönderim kanıtı iletişim merkezinin "İletişim geçmişi"
 * ekranındadır (#780 A).
 */
export function VATNoticeCard({
  engagement,
  canSend,
}: {
  engagement: string;
  canSend: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const list = useVATNotices(slug, engagement);
  const watch = useVATWatchEvents(slug, engagement);
  const run = useRunVATWatch(slug, engagement);
  const send = useSendVATNotice(slug, engagement);
  const [open, setOpen] = useState(false);
  const [message, setMessage] = useState("");
  const [error, setError] = useState<string | null>(null);

  const onSend = () => {
    setError(null);
    send.mutate(
      { message: message.trim() || undefined },
      {
        onSuccess: () => {
          toast.success(t("reports.vat_notice.sent"));
          setOpen(false);
          setMessage("");
        },
        onError: (e) =>
          setError(
            e instanceof Error && e.message
              ? e.message
              : t("reports.vat_notice.send_error"),
          ),
      },
    );
  };

  const items = list.data?.items ?? [];
  // KDV nöbetçisi (#780): son bildirimden sonra durum değiştiyse uyarı.
  const latest = items[0];
  const changed = latest
    ? (watch.data?.items ?? []).find(
        (e) =>
          e.event === "vat.status_changed" &&
          e.payload.notice_uuid === latest.uuid,
      )
    : undefined;

  const onRun = () =>
    run.mutate(undefined, {
      onSuccess: (data) =>
        toast.success(
          data.raised > 0
            ? t("reports.vat_watch.raised", { count: data.raised })
            : t("reports.vat_watch.nothing"),
        ),
      onError: (e) =>
        toast.error(
          e instanceof Error && e.message
            ? e.message
            : t("reports.vat_watch.error"),
        ),
    });

  return (
    <Card data-testid="vat-notice-card">
      <CardHeader className="flex flex-row flex-wrap items-center justify-between gap-2 space-y-0">
        <div>
          <CardTitle className="text-base">
            {t("reports.vat_notice.title")}
          </CardTitle>
          <p className="text-text-2 text-sm">{t("reports.vat_notice.hint")}</p>
        </div>
        {canSend ? (
          <div className="flex flex-wrap gap-2">
            <Button
              size="sm"
              variant="outline"
              onClick={onRun}
              disabled={run.isPending}
            >
              <RefreshCw className="size-4" aria-hidden />
              {t("reports.vat_watch.run")}
            </Button>
            <Button size="sm" onClick={() => setOpen(true)}>
              <Send className="size-4" aria-hidden />
              {t("reports.vat_notice.send")}
            </Button>
          </div>
        ) : null}
      </CardHeader>
      <CardContent className="space-y-3">
        {changed && latest ? (
          <Alert data-testid="vat-status-changed">
            <AlertTriangle className="size-4" aria-hidden />
            <AlertTitle>{t("reports.vat_watch.changed_title")}</AlertTitle>
            <AlertDescription className="space-y-2">
              <p>
                {t(
                  latest.outcome === "payment"
                    ? "reports.vat_watch.changed_was_payment"
                    : "reports.vat_watch.changed_was_no_payment",
                  { month: monthLabel(latest.period, locale) },
                )}{" "}
                {changed.payload.outcome === "payment"
                  ? t("reports.vat_watch.changed_now_payment", {
                      amount: money(String(changed.payload.payable), locale),
                    })
                  : t("reports.vat_watch.changed_now_no_payment")}
              </p>
              {canSend ? (
                <Button size="sm" onClick={() => setOpen(true)}>
                  {t("reports.vat_watch.resend")}
                </Button>
              ) : null}
            </AlertDescription>
          </Alert>
        ) : null}
        {list.isLoading ? (
          <Skeleton className="h-16 w-full" />
        ) : items.length === 0 ? (
          <p className="text-text-2 text-sm">{t("reports.vat_notice.empty")}</p>
        ) : (
          <ul className="space-y-2">
            {items.map((n) => (
              <NoticeRow key={n.uuid} n={n} />
            ))}
          </ul>
        )}
      </CardContent>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("reports.vat_notice.send_title")}</DialogTitle>
            <DialogDescription>
              {t("reports.vat_notice.send_description")}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="vat-notice-message">
              {t("reports.vat_notice.message")}
            </Label>
            <Textarea
              id="vat-notice-message"
              value={message}
              maxLength={1000}
              onChange={(e) => setMessage(e.target.value)}
              placeholder={t("reports.vat_notice.message_placeholder")}
            />
            {error ? (
              <p role="alert" className="text-danger text-sm">
                {error}
              </p>
            ) : null}
          </div>
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t("reports.cancel")}
            </Button>
            <Button onClick={onSend} disabled={send.isPending}>
              {send.isPending
                ? t("reports.vat_notice.sending")
                : t("reports.vat_notice.confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}
