"use client";

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { BosDurum } from "@/components/hakan";
import { TaxpayerRequestRow } from "@/features/taxpayer/components/taxpayer-request-row";
import {
  taxpayerTaskKeys,
  taxpayerTaskService,
  type TaxpayerDocumentTaskStatus,
} from "@/features/taxpayer/services/taxpayer.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

const FILTERS: TaxpayerDocumentTaskStatus[] = [
  "open",
  "submitted",
  "matched",
  "all",
];

/** `/m/{uuid}/requests`: müşavirin istedikleri, duruma göre (#331). */
export function TaxpayerRequestsPage({ taxpayer }: { taxpayer: string }) {
  const { t } = useLocale();
  const [status, setStatus] = useState<TaxpayerDocumentTaskStatus>("open");
  const q = useQuery({
    queryKey: taxpayerTaskKeys.list(taxpayer, status),
    queryFn: () => taxpayerTaskService.list(taxpayer, status),
  });
  const items = q.data?.items ?? [];
  return (
    <div className="space-y-6">
      <div>
        <h1 className="font-display text-[26px] leading-tight font-bold tracking-tight md:text-[30px]">
          {t("taxpayer.requests_title")}
        </h1>
        <p className="text-text-2 mt-1 text-sm">
          {t("taxpayer.requests_description")}
        </p>
      </div>
      <div
        role="group"
        aria-label={t("taxpayer.requests_filter")}
        className="flex flex-wrap gap-2"
      >
        {FILTERS.map((f) => (
          <button
            key={f}
            type="button"
            aria-pressed={status === f}
            onClick={() => setStatus(f)}
            className={cn(
              "focus-visible:ring-ring min-h-11 rounded-full border px-4 text-sm font-medium transition-colors focus-visible:ring-2 focus-visible:outline-none md:min-h-9",
              status === f
                ? "border-brand bg-brand text-on-brand"
                : "border-line bg-surface text-text-2 hover:text-foreground",
            )}
          >
            {t(`taxpayer.requests_filter_${f}`)}
          </button>
        ))}
      </div>
      <section className="bg-surface border-line rounded-xl border p-4 md:p-5">
        {q.isLoading ? (
          <p className="text-text-3 text-sm">{t("common.loading")}</p>
        ) : q.isError ? (
          <p className="text-danger text-sm">{t("taxpayer.load_error")}</p>
        ) : items.length === 0 ? (
          <BosDurum
            className="border-0 py-6"
            baslik={t("taxpayer.home_requests_empty_title")}
            aciklama={t(`taxpayer.requests_empty_${status}`)}
          />
        ) : (
          <ul className="divide-line divide-y">
            {items.map((task) => (
              <TaxpayerRequestRow
                key={task.uuid}
                taxpayer={taxpayer}
                task={task}
              />
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}
