"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";

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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Textarea } from "@/components/ui/textarea";
import {
  RECEIPT_CORRECTION_FIELDS,
  buildCorrections,
  taxpayerPanelKeys,
  taxpayerPanelService,
  type ReceiptCorrection,
} from "@/features/taxpayer/services/taxpayer.service";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import { formatDate, formatMoney } from "./format";

const PAGE = 20;

const statusVariant: Record<
  string,
  "success" | "warning" | "danger" | "secondary"
> = {
  approved: "success",
  processing: "secondary",
  draft: "warning",
  duplicate: "secondary",
  failed: "danger",
  rejected: "danger",
};

/** `/m/{uuid}/receipts`: captured receipts (fiş) and their review status. */
export function TaxpayerReceiptsPage({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const [offset, setOffset] = useState(0);
  const q = useQuery({
    queryKey: taxpayerPanelKeys.receipts(taxpayer, offset),
    queryFn: () =>
      taxpayerPanelService.receipts(taxpayer, { limit: PAGE, offset }),
  });
  const data = q.data;
  const [fixing, setFixing] = useState<string | null>(null);
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("taxpayer.receipts_title")}</CardTitle>
      </CardHeader>
      <CardContent>
        {q.isLoading ? (
          <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
        ) : q.isError ? (
          <p className="text-destructive text-sm">{t("taxpayer.load_error")}</p>
        ) : !data || data.items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("taxpayer.receipts_empty")}
          </p>
        ) : (
          <>
            <ul className="divide-y">
              {data.items.map((r) => (
                <li
                  key={r.uuid}
                  className="flex items-center justify-between gap-3 py-2 text-sm"
                >
                  <div>
                    <div>
                      {r.vendor_name ||
                        t(`taxpayer.receipt_kind_${r.kind ?? "other"}`)}
                    </div>
                    <div className="text-muted-foreground text-xs">
                      {formatDate(r.date || r.created_at, locale)}
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <span className="figure">
                      {r.total_amount
                        ? formatMoney(r.total_amount, "TRY", locale)
                        : "—"}
                    </span>
                    <Badge variant={statusVariant[r.status] ?? "secondary"}>
                      {t(`taxpayer.receipt_status_${r.status}`)}
                    </Badge>
                    {r.status === "draft" || r.status === "approved" ? (
                      <Button
                        variant="ghost"
                        size="sm"
                        onClick={() => setFixing(r.uuid)}
                      >
                        {t("taxpayer.receipt_fix")}
                      </Button>
                    ) : null}
                  </div>
                </li>
              ))}
            </ul>
            <div className="text-muted-foreground mt-4 flex items-center justify-between text-sm">
              <span>{t("taxpayer.total_count", { count: data.total })}</span>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={offset === 0}
                  onClick={() => setOffset(Math.max(0, offset - PAGE))}
                >
                  {t("taxpayer.prev")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={offset + PAGE >= data.total}
                  onClick={() => setOffset(offset + PAGE)}
                >
                  {t("taxpayer.next")}
                </Button>
              </div>
            </div>
          </>
        )}
      </CardContent>
      <CorrectionDialog
        taxpayer={taxpayer}
        receipt={fixing}
        onClose={() => setFixing(null)}
      />
    </Card>
  );
}

/**
 * Fişte yanlış okunan alanı mükellef düzeltir (#518 C1). Değerler fişe
 * yazılmaz: ofisin inceleme kuyruğuna öneri olarak gider.
 */
function CorrectionDialog({
  taxpayer,
  receipt,
  onClose,
}: {
  taxpayer: string;
  receipt: string | null;
  onClose: () => void;
}) {
  const { t } = useLocale();
  const [values, setValues] = useState<
    Partial<Record<ReceiptCorrection["field"], string>>
  >({});
  const [note, setNote] = useState("");
  const corrections = buildCorrections(values, note);
  const send = useMutation({
    mutationFn: () =>
      taxpayerPanelService.correctReceipt(taxpayer, receipt!, corrections),
    onSuccess: () => {
      toast.success(t("taxpayer.receipt_fix_sent"));
      setValues({});
      setNote("");
      onClose();
    },
    onError: (err) =>
      toast.error(
        isApiError(err) && err.status === 409
          ? t("taxpayer.receipt_fix_not_allowed")
          : t("taxpayer.receipt_fix_failed"),
      ),
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (corrections.length > 0) send.mutate();
  };
  return (
    <Dialog
      open={receipt !== null}
      onOpenChange={(v) => (v ? null : onClose())}
    >
      <DialogContent>
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t("taxpayer.receipt_fix_title")}</DialogTitle>
            <DialogDescription>
              {t("taxpayer.receipt_fix_hint")}
            </DialogDescription>
          </DialogHeader>
          <div className="grid gap-3 sm:grid-cols-2">
            {RECEIPT_CORRECTION_FIELDS.map((f) => (
              <div key={f} className="space-y-1.5">
                <Label htmlFor={`fix-${f}`}>
                  {t(`taxpayer.receipt_field_${f}`)}
                </Label>
                <Input
                  id={`fix-${f}`}
                  type={f === "date" ? "date" : "text"}
                  inputMode={
                    f === "total_amount" || f === "tax_amount"
                      ? "decimal"
                      : undefined
                  }
                  maxLength={200}
                  value={values[f] ?? ""}
                  onChange={(e) =>
                    setValues({ ...values, [f]: e.target.value })
                  }
                />
              </div>
            ))}
          </div>
          <div className="space-y-1.5">
            <Label htmlFor="fix-note">{t("taxpayer.receipt_fix_note")}</Label>
            <Textarea
              id="fix-note"
              rows={2}
              maxLength={500}
              value={note}
              onChange={(e) => setNote(e.target.value)}
            />
          </div>
          <DialogFooter>
            <Button type="button" variant="outline" onClick={onClose}>
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              disabled={corrections.length === 0 || send.isPending}
            >
              {t("taxpayer.receipt_fix_send")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
