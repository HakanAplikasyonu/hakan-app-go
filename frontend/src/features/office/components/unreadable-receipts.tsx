"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ImageOff, MessageSquareWarning, X } from "lucide-react";
import { useEffect, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { createDocumentRequest } from "../services/engagement-documents.service";
import {
  type ReceiptCapture,
  receiptKeys,
  receiptsService,
} from "../services/receipts.service";
import type { ReceiptRejection } from "../lib/reject-reason";
import { useRejectReason } from "./reject-reason-dialog";

/**
 * Okunamayan fiş fotoğrafları (#568): yapay zekâ okuyamadığı için taslak
 * oluşmayan yüklemeler. Ofis fotoğrafa bakar, mükelleften yenisini ister ve
 * bu yüklemeyi kapatır; hiçbiri sessizce kaybolmaz.
 */
export function UnreadableReceipts({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const key = receiptKeys.captures(slug, engagement, "failed");
  const list = useQuery({
    queryKey: key,
    queryFn: () => receiptsService.captures(slug, engagement, "failed"),
  });
  const close = useMutation({
    mutationFn: (v: { capture: string; reason: ReceiptRejection }) =>
      receiptsService.closeCapture(slug, engagement, v.capture, v.reason),
    onSuccess: () => {
      toast.success(t("office.receipts.unreadable_closed"));
      void qc.invalidateQueries({ queryKey: key });
    },
    onError: () => toast.error(t("office.receipts.decide_failed")),
  });
  const reject = useRejectReason(close.isPending);
  const items = list.data?.items ?? [];
  if (items.length === 0) return null;
  return (
    <section
      aria-labelledby="unreadable-receipts"
      className="border-warn/40 bg-warn-tint/40 space-y-3 rounded-xl border p-4"
    >
      <div>
        <h3 id="unreadable-receipts" className="text-sm font-semibold">
          {t("office.receipts.unreadable_title", {
            count: list.data?.total ?? items.length,
          })}
        </h3>
        <p className="text-text-2 text-sm">
          {t("office.receipts.unreadable_hint")}
        </p>
      </div>
      <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
        {items.map((c) => (
          <UnreadableItem
            key={c.uuid}
            engagement={engagement}
            capture={c}
            canRequest={can(permissions.tenant.docTasks.write)}
            canClose={can(permissions.tenant.receipts.approve)}
            closing={close.isPending && close.variables?.capture === c.uuid}
            onClose={() =>
              reject.ask((reason) => close.mutate({ capture: c.uuid, reason }))
            }
          />
        ))}
      </ul>
      {reject.dialog}
    </section>
  );
}

function UnreadableItem({
  engagement,
  capture,
  canRequest,
  canClose,
  closing,
  onClose,
}: {
  engagement: string;
  capture: ReceiptCapture;
  canRequest: boolean;
  canClose: boolean;
  closing: boolean;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const [src, setSrc] = useState<string | null>(null);
  useEffect(() => {
    let url: string | null = null;
    let alive = true;
    void receiptsService
      .captureImage(slug, engagement, capture.uuid)
      .then((u) => {
        url = u;
        if (alive) setSrc(u);
        else if (u) URL.revokeObjectURL(u);
      })
      .catch(() => undefined);
    return () => {
      alive = false;
      if (url) URL.revokeObjectURL(url);
    };
  }, [slug, engagement, capture.uuid]);
  const when = new Date(capture.created_at).toLocaleDateString(
    locale === "tr" ? "tr-TR" : "en-GB",
    { day: "numeric", month: "short", year: "numeric" },
  );
  const request = useMutation({
    mutationFn: () =>
      createDocumentRequest(slug, engagement, {
        title: t("office.receipts.unreadable_request_title", { date: when }),
        note: t("office.receipts.unreadable_request_note"),
        due_days: 3,
      }),
    onSuccess: () => toast.success(t("office.receipts.unreadable_requested")),
    onError: () => toast.error(t("office.api.load_error")),
  });
  return (
    <li className="bg-surface border-line flex gap-3 rounded-lg border p-3">
      <div className="bg-surface-3 flex size-20 shrink-0 items-center justify-center overflow-hidden rounded-md">
        {src ? (
          // eslint-disable-next-line @next/next/no-img-element -- blob URL
          <img
            src={src}
            alt={t("office.receipts.unreadable_photo", { date: when })}
            className="size-full object-cover"
          />
        ) : (
          <ImageOff className="text-text-3 size-6" aria-hidden />
        )}
      </div>
      <div className="flex min-w-0 flex-1 flex-col gap-2 text-sm">
        <span className="font-medium">
          {t("office.receipts.unreadable_uploaded", { date: when })}
        </span>
        <div className="flex flex-wrap gap-2">
          {canRequest ? (
            <Button
              size="sm"
              variant="outline"
              className="max-md:min-h-11"
              disabled={request.isPending || request.isSuccess}
              onClick={() => request.mutate()}
            >
              <MessageSquareWarning className="size-4" aria-hidden />
              {request.isSuccess
                ? t("office.receipts.unreadable_request_sent")
                : t("office.receipts.unreadable_request")}
            </Button>
          ) : null}
          {canClose ? (
            <Button
              size="sm"
              variant="ghost"
              className="max-md:min-h-11"
              disabled={closing}
              onClick={onClose}
              data-testid="unreadable-close"
            >
              <X className="size-4" aria-hidden />
              {t("office.receipts.unreadable_close")}
            </Button>
          ) : null}
        </div>
      </div>
    </li>
  );
}
