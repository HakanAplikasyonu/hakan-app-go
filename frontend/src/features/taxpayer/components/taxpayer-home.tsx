"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Upload } from "lucide-react";
import Link from "next/link";
import { useRef, useState } from "react";
import { toast } from "sonner";

import { BosDurum, TacRozeti, type TacDurumu } from "@/components/hakan";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { OfficeRequestsCard } from "@/features/taxpayer-onboarding/components/office-requests-card";
import { HistorySharesCard } from "@/features/taxpayer/components/history-shares-card";
import { TaxpayerRequestRow } from "@/features/taxpayer/components/taxpayer-request-row";
import { TaxpayerVatSalesCard } from "@/features/taxpayer/components/taxpayer-vat-sales-card";
import {
  taxpayerPanelKeys,
  taxpayerPanelService,
  taxpayerTaskKeys,
  taxpayerTaskService,
  type TaxpayerContextResponse,
  type TaxpayerReceipt,
  type TaxpayerUploadJob,
} from "@/features/taxpayer/services/taxpayer.service";
import { cn } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

import { formatDate, formatMoney } from "./format";

/** Bırakma alanının kabul ettikleri: fiş görselleri ve e-fatura XML / ZIP. */
const DROP_ACCEPT =
  "image/jpeg,image/png,image/webp,application/pdf,.xml,.zip,text/xml,application/xml,application/zip";

type FileRoute = "receipt" | "ubl" | "pdf" | "unknown";

function routeFile(file: File): FileRoute {
  const name = file.name.toLowerCase();
  if (/^image\/(jpeg|png|webp|gif)$/.test(file.type)) return "receipt";
  if (name.endsWith(".xml") || name.endsWith(".zip")) return "ubl";
  if (file.type === "application/pdf" || name.endsWith(".pdf")) return "pdf";
  return "unknown";
}

/**
 * `/m/{uuid}` özeti (arayüz 8/9, #331; tuval MukellefWeb): müşavirin
 * istedikleri, belge bırakma alanı ve son gönderilenler. Ofis talepleri
 * (#7) ve geçmiş paylaşım onayları (#8) yalnız sahibe görünür.
 */
export function TaxpayerHome({
  context,
}: {
  context: TaxpayerContextResponse;
}) {
  const { t, tp, locale } = useLocale();
  const { user } = useAuth();
  const taxpayer = context.taxpayer.uuid;
  const tasks = useQuery({
    queryKey: taxpayerTaskKeys.list(taxpayer, "open"),
    queryFn: () => taxpayerTaskService.list(taxpayer, "open"),
  });
  const open = tasks.data?.items ?? [];
  const office = open.find((x) => x.organization_name)?.organization_name;
  const firstName = user?.name?.trim().split(/\s+/)[0] ?? "";
  const period = open[0]?.period;
  const month = period
    ? new Intl.DateTimeFormat(locale, { month: "long" }).format(
        new Date(`${period}-01T00:00:00`),
      )
    : "";

  let greeting: string;
  if (tasks.isLoading)
    greeting = t("taxpayer.home_greeting", { name: firstName });
  else if (open.length === 0)
    greeting = t("taxpayer.home_nothing_missing", { name: firstName });
  else
    greeting = tp("taxpayer.home_missing", open.length, {
      name: firstName,
      month,
    });

  return (
    <div className="space-y-6">
      <div>
        <p className="text-text-2 text-sm">
          {office
            ? t("taxpayer.home_office", { office })
            : context.taxpayer.title}
        </p>
        <h1 className="font-display mt-1 text-[26px] leading-tight font-bold tracking-tight md:text-[30px]">
          {greeting}
        </h1>
      </div>

      <OfficeRequestsCard taxpayer={taxpayer} />

      <TaxpayerVatSalesCard taxpayer={taxpayer} />

      <div className="grid gap-6 xl:grid-cols-[minmax(0,1fr)_400px]">
        <div className="min-w-0 space-y-6">
          <section
            aria-labelledby="tp-requests-title"
            className="bg-surface border-line rounded-xl border p-4 md:p-5"
          >
            <div className="mb-4 flex items-baseline justify-between gap-3">
              <h2
                id="tp-requests-title"
                className="font-display text-lg font-bold"
              >
                {t("taxpayer.home_requests_title")}
              </h2>
              <Link
                href={routes.taxpayer.requests(taxpayer)}
                className="text-brand-text inline-flex min-h-6 items-center text-sm font-medium hover:underline max-md:min-h-11"
              >
                {t("taxpayer.home_requests_all")}
              </Link>
            </div>
            {tasks.isLoading ? (
              <p className="text-text-3 text-sm">{t("common.loading")}</p>
            ) : tasks.isError ? (
              <p className="text-danger text-sm">{t("taxpayer.load_error")}</p>
            ) : open.length === 0 ? (
              <BosDurum
                className="border-0 py-6"
                baslik={t("taxpayer.home_requests_empty_title")}
                aciklama={t("taxpayer.home_requests_empty")}
              />
            ) : (
              <ul className="divide-line divide-y">
                {open.slice(0, 6).map((task) => (
                  <TaxpayerRequestRow
                    key={task.uuid}
                    taxpayer={taxpayer}
                    task={task}
                  />
                ))}
              </ul>
            )}
          </section>

          <DropZone taxpayer={taxpayer} hasTasks={open.length > 0} />
        </div>

        <RecentCard taxpayer={taxpayer} />
      </div>

      <HistorySharesCard taxpayer={taxpayer} />
    </div>
  );
}

/**
 * Bırakma alanı: görseller fiş olarak okunur, XML / ZIP e-fatura yüklemesine
 * gider. PDF'nin hangi isteğe ait olduğu bilinmediği için isteğe eklenir.
 */
function DropZone({
  taxpayer,
  hasTasks,
}: {
  taxpayer: string;
  hasTasks: boolean;
}) {
  const { t, tp } = useLocale();
  const qc = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const [over, setOver] = useState(false);
  const send = useMutation({
    mutationFn: async (files: File[]) => {
      let sent = 0;
      for (const file of files) {
        const kind = routeFile(file);
        if (kind === "receipt") {
          await taxpayerTaskService.receipt(taxpayer, file);
          sent++;
        } else if (kind === "ubl") {
          await taxpayerPanelService.upload(taxpayer, file);
          sent++;
        } else if (kind === "pdf") {
          toast.info(
            hasTasks
              ? t("taxpayer.drop_pdf_to_request", { name: file.name })
              : t("taxpayer.drop_pdf_no_request", { name: file.name }),
          );
        } else {
          toast.error(t("taxpayer.drop_unsupported", { name: file.name }));
        }
      }
      return sent;
    },
    onSuccess: (sent) => {
      if (sent > 0) toast.success(tp("taxpayer.drop_sent", sent));
    },
    onSettled: () => {
      void qc.invalidateQueries({
        queryKey: ["taxpayer", taxpayer.toLowerCase()],
      });
    },
  });
  const take = (list: FileList | null) => {
    const files = list ? Array.from(list) : [];
    if (files.length > 0) send.mutate(files);
  };
  return (
    <section
      aria-labelledby="tp-drop-title"
      onDragOver={(e) => {
        e.preventDefault();
        setOver(true);
      }}
      onDragLeave={() => setOver(false)}
      onDrop={(e) => {
        e.preventDefault();
        setOver(false);
        take(e.dataTransfer.files);
      }}
      className={cn(
        "flex flex-col items-center gap-3 rounded-xl border-2 border-dashed px-6 py-8 text-center transition-colors",
        over ? "border-brand bg-brand-tint" : "border-line-strong bg-surface",
      )}
    >
      <Upload
        aria-hidden
        className="text-brand-text size-7"
        strokeWidth={1.8}
      />
      <h2 id="tp-drop-title" className="font-display text-base font-bold">
        {t("taxpayer.drop_title")}
      </h2>
      <p className="text-text-2 max-w-md text-sm">{t("taxpayer.drop_hint")}</p>
      <input
        ref={input}
        type="file"
        multiple
        accept={DROP_ACCEPT}
        className="sr-only"
        tabIndex={-1}
        aria-hidden
        onChange={(e) => {
          take(e.target.files);
          e.target.value = "";
        }}
      />
      <Button
        type="button"
        variant="outline"
        className="min-h-11 md:min-h-9"
        disabled={send.isPending}
        onClick={() => input.current?.click()}
      >
        {send.isPending
          ? t("taxpayer.drop_sending")
          : t("taxpayer.drop_choose")}
      </Button>
    </section>
  );
}

type RecentItem = {
  key: string;
  title: string;
  at: string;
  durum: TacDurumu;
  label: string;
  amount?: string;
};

function receiptItem(
  r: TaxpayerReceipt,
  t: (k: string) => string,
  locale: string,
): RecentItem {
  const map: Record<TaxpayerReceipt["status"], [TacDurumu, string]> = {
    processing: ["yuklendi", "taxpayer.recent_receipt_processing"],
    draft: ["yuklendi", "taxpayer.recent_receipt_draft"],
    approved: ["onaylandi", "taxpayer.recent_receipt_approved"],
    failed: ["sorun", "taxpayer.recent_receipt_failed"],
    rejected: ["sorun", "taxpayer.recent_receipt_rejected"],
    duplicate: ["sorun", "taxpayer.recent_receipt_duplicate"],
  };
  const [durum, label] = map[r.status] ?? [
    "yuklendi",
    "taxpayer.recent_receipt_draft",
  ];
  return {
    key: `r:${r.uuid}`,
    title: r.vendor_name || t(`taxpayer.receipt_kind_${r.kind ?? "other"}`),
    at: r.created_at,
    durum,
    label: t(label),
    amount: r.total_amount
      ? formatMoney(r.total_amount, "TRY", locale)
      : undefined,
  };
}

function uploadItem(
  u: TaxpayerUploadJob,
  t: (k: string) => string,
): RecentItem {
  const map: Record<TaxpayerUploadJob["status"], [TacDurumu, string]> = {
    queued: ["yuklendi", "taxpayer.recent_upload_processing"],
    processing: ["yuklendi", "taxpayer.recent_upload_processing"],
    done: ["onaylandi", "taxpayer.recent_upload_done"],
    failed: ["sorun", "taxpayer.recent_upload_failed"],
  };
  const [durum, label] = map[u.status];
  return {
    key: `u:${u.uuid}`,
    title: u.file_name,
    at: u.created_at,
    durum,
    label: t(label),
  };
}

/** "Son gönderdiklerin": fişler ve e-fatura yüklemeleri, yeniden eskiye. */
function RecentCard({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const receipts = useQuery({
    queryKey: taxpayerPanelKeys.receipts(taxpayer, 0),
    queryFn: () =>
      taxpayerPanelService.receipts(taxpayer, { limit: 20, offset: 0 }),
  });
  const uploads = useQuery({
    queryKey: taxpayerPanelKeys.uploads(taxpayer),
    queryFn: () => taxpayerPanelService.uploads(taxpayer),
  });
  const items = [
    ...(receipts.data?.items ?? []).map((r) => receiptItem(r, t, locale)),
    ...(uploads.data?.items ?? []).map((u) => uploadItem(u, t)),
  ]
    .sort((a, b) => b.at.localeCompare(a.at))
    .slice(0, 8);
  const loading = receipts.isLoading || uploads.isLoading;
  return (
    <section
      aria-labelledby="tp-recent-title"
      className="bg-surface border-line h-fit rounded-xl border p-4 md:p-5"
    >
      <h2 id="tp-recent-title" className="font-display mb-4 text-lg font-bold">
        {t("taxpayer.recent_title")}
      </h2>
      {loading ? (
        <p className="text-text-3 text-sm">{t("common.loading")}</p>
      ) : items.length === 0 ? (
        <p className="text-text-3 text-sm">{t("taxpayer.recent_empty")}</p>
      ) : (
        <ul className="divide-line divide-y">
          {items.map((item) => (
            <li
              key={item.key}
              className="flex items-center gap-3 py-3 first:pt-0 last:pb-0"
            >
              <TacRozeti durum={item.durum} boyut={16} />
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">{item.title}</div>
                <div
                  className={cn(
                    "text-xs",
                    item.durum === "sorun"
                      ? "text-danger font-semibold"
                      : "text-text-3",
                  )}
                >
                  {item.label} · {formatDate(item.at, locale)}
                </div>
              </div>
              {item.amount ? (
                <span className="text-sm font-semibold tabular-nums">
                  {item.amount}
                </span>
              ) : null}
            </li>
          ))}
        </ul>
      )}
      <div className="mt-4 flex gap-4 text-sm">
        <Link
          href={routes.taxpayer.receipts(taxpayer)}
          className="text-brand-text inline-flex min-h-6 items-center font-medium hover:underline max-md:min-h-11"
        >
          {t("taxpayer.nav_receipts")}
        </Link>
        <Link
          href={routes.taxpayer.uploads(taxpayer)}
          className="text-brand-text inline-flex min-h-6 items-center font-medium hover:underline max-md:min-h-11"
        >
          {t("taxpayer.recent_uploads_link")}
        </Link>
      </div>
    </section>
  );
}
