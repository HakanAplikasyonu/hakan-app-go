"use client";

import { FileDown, Mail } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
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
import { useSendReport } from "@/features/reports/hooks/use-deliveries";
import {
  reportService,
  type ReportRange,
  type TaxpayerReport,
} from "@/features/reports/services/reports.service";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { download } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

function errorMessage(error: unknown, fallback: string) {
  return error instanceof Error && error.message ? error.message : fallback;
}

function fileName(report: TaxpayerReport) {
  return `rapor-${report.period}.pdf`;
}

/** PDF indirme ve tek tıkla e-posta (#386). */
export function ReportActions({
  engagement,
  report,
  range,
  canSend,
}: {
  engagement: string;
  report: TaxpayerReport;
  range: ReportRange;
  canSend: boolean;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [pdfBusy, setPdfBusy] = useState(false);
  const [open, setOpen] = useState(false);
  const [recipient, setRecipient] = useState("");
  const [error, setError] = useState<string | null>(null);
  const send = useSendReport(slug, engagement);

  const onPdf = async () => {
    setPdfBusy(true);
    try {
      const blob = await reportService.pdf(slug, engagement, range);
      download(blob, fileName(report));
    } catch (e) {
      toast.error(errorMessage(e, t("reports.pdf_error")));
    } finally {
      setPdfBusy(false);
    }
  };

  const onSend = () => {
    setError(null);
    send.mutate(
      { range, recipient: recipient.trim() || undefined },
      {
        onSuccess: (d) => {
          toast.success(t("reports.send_queued", { recipient: d.recipient }));
          setOpen(false);
          setRecipient("");
        },
        onError: (e) => setError(errorMessage(e, t("reports.send_error"))),
      },
    );
  };

  return (
    <>
      <Button variant="outline" onClick={onPdf} disabled={pdfBusy}>
        <FileDown aria-hidden className="size-4" />
        {pdfBusy ? t("reports.pdf_busy") : t("reports.pdf")}
      </Button>
      {canSend ? (
        <Button onClick={() => setOpen(true)}>
          <Mail aria-hidden className="size-4" />
          {t("reports.send")}
        </Button>
      ) : null}
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("reports.send_title")}</DialogTitle>
            <DialogDescription>
              {t("reports.send_description", {
                taxpayer: report.taxpayer.title,
                period: report.label,
              })}
            </DialogDescription>
          </DialogHeader>
          <div className="space-y-2">
            <Label htmlFor="report-recipient">
              {t("reports.send_recipient")}
            </Label>
            <Input
              id="report-recipient"
              type="email"
              autoComplete="email"
              value={recipient}
              placeholder={t("reports.send_recipient_placeholder")}
              onChange={(e) => setRecipient(e.target.value)}
              aria-invalid={error ? true : undefined}
              aria-describedby="report-recipient-hint"
            />
            <p id="report-recipient-hint" className="text-text-2 text-xs">
              {t("reports.send_recipient_hint")}
            </p>
            {report.commentary && report.commentary.status !== "approved" ? (
              <p className="text-warn text-xs">
                {t("reports.send_draft_note")}
              </p>
            ) : null}
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
              <Mail aria-hidden className="size-4" />
              {send.isPending
                ? t("reports.send_busy")
                : t("reports.send_confirm")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
