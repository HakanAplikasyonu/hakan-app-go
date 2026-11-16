"use client";

import { useMutation, useQueryClient } from "@tanstack/react-query";
import { useRef } from "react";
import { toast } from "sonner";

import { TacRozeti, type TacDurumu } from "@/components/hakan";
import { Button } from "@/components/ui/button";
import {
  taxpayerTaskKeys,
  taxpayerTaskService,
  type TaxpayerDocumentTask,
} from "@/features/taxpayer/services/taxpayer.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatDate, formatMoney } from "./format";

/** Bir isteğe eklenebilen dosyalar (backend: PDF ve görseller). */
export const TASK_FILE_ACCEPT =
  "application/pdf,image/jpeg,image/png,image/webp,image/heic";

const BANK_KINDS = new Set(["undocumented_payment", "uninvoiced_collection"]);

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;
type TranslatePlural = (
  key: string,
  count: number,
  vars?: Record<string, string | number>,
) => string;

/**
 * İsteğin başlığı: banka görevlerinde türün adı, müşavirin elle açtığı
 * talepte istenen belge (mobil `taskTitle` ile aynı kural).
 */
export function taskTitle(task: TaxpayerDocumentTask, t: Translate) {
  if (BANK_KINDS.has(task.kind)) return t(`taxpayer.task_kind_${task.kind}`);
  return task.title?.trim() || t("taxpayer.task_kind_generic");
}

function taskDetail(task: TaxpayerDocumentTask, locale: string) {
  if (!BANK_KINDS.has(task.kind)) return task.note;
  const b = task.bank_transaction;
  return [
    formatMoney(b.amount, b.currency, locale),
    b.counterparty_name,
    formatDate(b.transaction_date, locale),
  ]
    .filter(Boolean)
    .join(" · ");
}

const DAY = 86_400_000;

function dueText(
  task: TaxpayerDocumentTask,
  t: Translate,
  tp: TranslatePlural,
  locale: string,
) {
  if (!task.due_at) return null;
  const due = new Date(task.due_at).getTime();
  if (Number.isNaN(due)) return null;
  const days = Math.ceil((due - Date.now()) / DAY);
  if (task.overdue || days < 0) {
    return {
      late: true,
      text: tp("taxpayer.task_overdue", Math.max(1, -days)),
    };
  }
  if (days <= 7) {
    return {
      late: false,
      text:
        days === 0
          ? t("taxpayer.task_due_today")
          : tp("taxpayer.task_due_in", days),
    };
  }
  return {
    late: false,
    text: t("taxpayer.task_due_on", {
      date: formatDate(task.due_at, locale),
    }),
  };
}

function taskCrown(task: TaxpayerDocumentTask): TacDurumu {
  switch (task.status) {
    case "submitted":
      return "yuklendi";
    case "matched":
      return "onaylandi";
    case "cancelled":
      return "sorun";
    default:
      return "bekleniyor";
  }
}

/**
 * "Müşavirinin istedikleri" satırı: taç, istenen belge, son gün ve dosya
 * seçme. Açık istekte dosya doğrudan isteğe eklenir.
 */
export function TaxpayerRequestRow({
  taxpayer,
  task,
}: {
  taxpayer: string;
  task: TaxpayerDocumentTask;
}) {
  const { t, tp, locale } = useLocale();
  const qc = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const upload = useMutation({
    mutationFn: (file: File) =>
      taxpayerTaskService.upload(taxpayer, task.uuid, file),
    onSuccess: () => {
      toast.success(t("taxpayer.task_uploaded"));
      void qc.invalidateQueries({ queryKey: taxpayerTaskKeys.all(taxpayer) });
    },
  });
  const title = taskTitle(task, t);
  const detail = taskDetail(task, locale);
  const due = task.status === "open" ? dueText(task, t, tp, locale) : null;
  const canUpload = task.status === "open" || task.status === "submitted";
  return (
    <li className="flex flex-col gap-3 py-4 first:pt-0 last:pb-0 sm:flex-row sm:items-center">
      <div className="flex min-w-0 flex-1 items-start gap-3">
        <TacRozeti durum={taskCrown(task)} boyut={30} className="mt-0.5" />
        <div className="min-w-0">
          <div className="font-semibold">{title}</div>
          {detail ? (
            <div className="text-text-2 truncate text-sm">{detail}</div>
          ) : null}
          <div className="mt-0.5 text-xs">
            {due ? (
              <span
                className={cn(
                  due.late ? "text-danger font-semibold" : "text-text-3",
                )}
              >
                {due.text}
              </span>
            ) : (
              <span className="text-text-3">
                {t(`taxpayer.task_status_${task.status}`)}
              </span>
            )}
            {task.upload_count > 0 ? (
              <span className="text-text-3">
                {" · "}
                {tp("taxpayer.task_files", task.upload_count)}
              </span>
            ) : null}
          </div>
        </div>
      </div>
      {canUpload ? (
        <>
          <input
            ref={input}
            type="file"
            accept={TASK_FILE_ACCEPT}
            className="sr-only"
            tabIndex={-1}
            aria-hidden
            onChange={(e) => {
              const file = e.target.files?.[0];
              e.target.value = "";
              if (file) upload.mutate(file);
            }}
          />
          <Button
            type="button"
            variant={task.status === "open" ? "default" : "outline"}
            className="min-h-11 shrink-0 md:min-h-9"
            disabled={upload.isPending}
            aria-label={`${t("taxpayer.task_choose_file")}: ${title}`}
            onClick={() => input.current?.click()}
          >
            {upload.isPending
              ? t("taxpayer.task_uploading")
              : task.status === "open"
                ? t("taxpayer.task_choose_file")
                : t("taxpayer.task_add_file")}
          </Button>
        </>
      ) : null}
    </li>
  );
}
