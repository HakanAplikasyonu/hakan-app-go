"use client";

import { useCallback, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Textarea } from "@/components/ui/textarea";
import { useLocale } from "@/providers/locale-provider";

import {
  MAX_REJECT_TEXT,
  REJECT_REASON_CODES,
  type ReceiptRejection,
  type RejectReasonCode,
  rejectionBody,
} from "../lib/reject-reason";

type Confirm = (reason: ReceiptRejection) => void;

/**
 * Ret gerekçesi penceresi (#780): ofis fişi, masrafı ya da Z raporunu
 * reddederken hazır gerekçelerden birini seçer, isterse açıklama yazar
 * ("Diğer"de zorunlu). Gerekçe mükellefe gider ve Gönderdiklerim'de görünür.
 *
 * `ask(onConfirm)` pencereyi açar; `dialog` sayfaya bir kez yerleştirilir.
 */
export function useRejectReason(pending = false) {
  const [open, setOpen] = useState(false);
  const confirm = useRef<Confirm | null>(null);
  const ask = useCallback((onConfirm: Confirm) => {
    confirm.current = onConfirm;
    setOpen(true);
  }, []);
  const dialog = (
    <RejectReasonDialog
      open={open}
      onOpenChange={setOpen}
      pending={pending}
      onConfirm={(r) => {
        setOpen(false);
        confirm.current?.(r);
      }}
    />
  );
  return { ask, dialog };
}

export function RejectReasonDialog({
  open,
  onOpenChange,
  onConfirm,
  pending,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: Confirm;
  pending?: boolean;
}) {
  const { t } = useLocale();
  const [code, setCode] = useState<RejectReasonCode | "">("");
  const [text, setText] = useState("");
  const [tried, setTried] = useState(false);
  const result = rejectionBody(code, text);
  const textError =
    tried && !result.ok && result.error === "text_required"
      ? t("office.receipts.reject_text_required")
      : null;
  const reset = (next: boolean) => {
    if (!next) {
      setCode("");
      setText("");
      setTried(false);
    }
    onOpenChange(next);
  };
  return (
    <Dialog open={open} onOpenChange={reset}>
      <DialogContent className="sm:max-w-md" data-testid="reject-reason-dialog">
        <DialogHeader>
          <DialogTitle>{t("office.receipts.reject_dialog_title")}</DialogTitle>
          <DialogDescription>
            {t("office.receipts.reject_dialog_desc")}
          </DialogDescription>
        </DialogHeader>
        <RadioGroup
          value={code}
          onValueChange={(v) => setCode(v as RejectReasonCode)}
          className="gap-2"
          aria-label={t("office.receipts.reject_dialog_title")}
        >
          {REJECT_REASON_CODES.map((c) => (
            <div key={c} className="flex min-h-9 items-center gap-2">
              <RadioGroupItem
                value={c}
                id={`reject-reason-${c}`}
                data-testid={`reject-reason-${c}`}
              />
              <Label htmlFor={`reject-reason-${c}`} className="font-normal">
                {t(`office.receipts.reject_reason_${c}`)}
              </Label>
            </div>
          ))}
        </RadioGroup>
        <div className="space-y-1">
          <Label htmlFor="reject-reason-text">
            {code === "diger"
              ? t("office.receipts.reject_text_label")
              : t("office.receipts.reject_text_optional")}
          </Label>
          <Textarea
            id="reject-reason-text"
            data-testid="reject-reason-text"
            value={text}
            maxLength={MAX_REJECT_TEXT}
            rows={3}
            placeholder={t("office.receipts.reject_text_placeholder")}
            aria-invalid={textError ? true : undefined}
            onChange={(e) => setText(e.target.value)}
          />
          <div className="flex justify-between gap-2 text-xs">
            <span className="text-destructive">{textError}</span>
            <span className="text-text-3">
              {t("office.receipts.reject_text_count", { count: text.length })}
            </span>
          </div>
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={() => reset(false)}>
            {t("office.receipts.reject_cancel")}
          </Button>
          <Button
            variant="destructive"
            data-testid="reject-reason-confirm"
            disabled={pending || !code}
            onClick={() => {
              setTried(true);
              if (!result.ok) return;
              onConfirm(result.body);
              setCode("");
              setText("");
              setTried(false);
            }}
          >
            {t("office.receipts.reject_confirm")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
