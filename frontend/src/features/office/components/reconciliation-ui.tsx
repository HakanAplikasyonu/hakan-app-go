"use client";

import { useState } from "react";

import { StatusPill, type StatusTone } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Textarea } from "@/components/ui/textarea";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

export const resultTone: Record<string, StatusTone> = {
  matched: "ok",
  difference: "warn",
  missing_in_ledger: "danger",
  missing_in_sources: "danger",
  no_ledger_account: "neutral",
};

export const decisionTone: Record<string, StatusTone> = {
  open: "warn",
  explained: "info",
  in_review: "info",
  resolved: "ok",
  ignored: "neutral",
};

export const babsTone: Record<string, StatusTone> = {
  not_required: "neutral",
  ok: "ok",
  missing_in_form: "danger",
  extra_in_form: "warn",
  amount_mismatch: "danger",
  count_mismatch: "warn",
  no_declaration: "neutral",
};

export const letterTone: Record<string, StatusTone> = {
  draft: "neutral",
  queued: "info",
  sent: "info",
  failed: "danger",
  responded: "ok",
};

/** Sonuç / durum rozeti: değer `office.reconciliation.{group}_{value}`. */
export function Pill({
  group,
  value,
  tones,
}: {
  group: string;
  value: string;
  tones: Record<string, StatusTone>;
}) {
  const { t } = useLocale();
  return (
    <StatusPill tone={tones[value] ?? "neutral"}>
      {t(`office.reconciliation.${group}_${value}`)}
    </StatusPill>
  );
}

export function errText(err: unknown, fallback: string) {
  return isApiError(err) ? err.message : fallback;
}

/** Not isteyen karar (açıkla, incelemeye gönder) için küçük diyalog. */
export function NoteDialog({
  open,
  onOpenChange,
  title,
  description,
  initial,
  required,
  confirmLabel,
  pending,
  onConfirm,
}: {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  title: string;
  description?: string;
  initial?: string;
  required?: boolean;
  confirmLabel: string;
  pending?: boolean;
  onConfirm: (note: string) => void;
}) {
  const { t } = useLocale();
  const [note, setNote] = useState(initial ?? "");
  return (
    <Dialog
      open={open}
      onOpenChange={(o) => {
        if (o) setNote(initial ?? "");
        onOpenChange(o);
      }}
    >
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{title}</DialogTitle>
          {description ? (
            <DialogDescription>{description}</DialogDescription>
          ) : null}
        </DialogHeader>
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="font-medium">{t("office.reconciliation.note")}</span>
          <Textarea
            value={note}
            maxLength={2000}
            rows={4}
            onChange={(e) => setNote(e.target.value)}
          />
        </label>
        <DialogFooter>
          <Button variant="ghost" onClick={() => onOpenChange(false)}>
            {t("office.reconciliation.cancel")}
          </Button>
          <Button
            disabled={pending || (required && note.trim() === "")}
            onClick={() => onConfirm(note.trim())}
          >
            {confirmLabel}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
