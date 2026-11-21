"use client";

import { useQuery } from "@tanstack/react-query";
import { Download, FileText, Star } from "lucide-react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { BosDurum } from "@/components/hakan";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import {
  taxpayerArchiveKeys,
  taxpayerArchiveService,
  type TaxpayerArchiveDocument,
} from "@/features/taxpayer/services/archive.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { formatDate } from "./format";

/** Kategorilerin gösterim sırası (ofis arşiviyle aynı). */
const CATEGORIES = [
  "general",
  "contract",
  "tax",
  "sgk",
  "trade_registry",
  "isg",
  "other",
] as const;

/** Geçerlilik: süresi geçen kırmızı, yaklaşan uyarı renginde. */
function Validity({ d, intl }: { d: TaxpayerArchiveDocument; intl: string }) {
  const { t } = useLocale();
  if (!d.valid_until) return null;
  const days = d.days_left ?? 0;
  const text =
    d.expiry_status === "expired"
      ? t("taxpayer.archive_expired", { count: -days })
      : d.expiry_status === "expiring"
        ? t("taxpayer.archive_expiring", { count: days })
        : t("taxpayer.archive_valid_until", {
            date: formatDate(d.valid_until, intl),
          });
  return (
    <span
      className={cn(
        d.expiry_status === "expired" && "text-status-blocking font-medium",
        d.expiry_status === "expiring" && "text-warn font-medium",
      )}
    >
      {` · ${text}`}
    </span>
  );
}

/**
 * `/m/{uuid}/archive` (#682): müşavirin belge arşivinden mükellefle
 * paylaştığı belgeler (sözleşme, vergi, SGK, İSG evrakı…), kategoriye göre
 * gruplu; yalnızca okuma ve indirme.
 */
export function TaxpayerArchivePage({ taxpayer }: { taxpayer: string }) {
  return (
    <div className="space-y-5">
      <ImportantDocuments taxpayer={taxpayer} />
      <SharedDocuments taxpayer={taxpayer} />
    </div>
  );
}

function DownloadButton({
  taxpayer,
  d,
}: {
  taxpayer: string;
  d: TaxpayerArchiveDocument;
}) {
  const { t } = useLocale();
  return (
    <Button
      size="sm"
      variant="outline"
      className="min-h-11 sm:min-h-9"
      aria-label={t("taxpayer.archive_download_named", { title: d.title })}
      onClick={() =>
        void taxpayerArchiveService
          .download(taxpayer, d)
          .catch(() => toast.error(t("taxpayer.archive_download_failed")))
      }
    >
      <Download aria-hidden className="size-4" />
      {/* Görünen metin erişilebilir adın parçası olsun ("… indir"; axe
          label-content-name-mismatch): küçük yazılır, CSS "İndir" gösterir. */}
      <span className="capitalize">{t("taxpayer.archive_download")}</span>
    </Button>
  );
}

/**
 * Önemli belgeler (#780 B): vergi levhası, imza sirküleri, ticaret sicil
 * gazetesi, kira sözleşmesi... Müşavirin "önemli" işaretleyip paylaştığı
 * belgeler; bitişi 30 gün içinde olan belge renkli. Ofis bölümü kapattıysa ya
 * da paylaşılan önemli belge yoksa görünmez.
 */
function ImportantDocuments({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const intl = locale === "tr" ? "tr-TR" : "en-GB";
  const q = useQuery({
    queryKey: taxpayerArchiveKeys.important(taxpayer),
    queryFn: () => taxpayerArchiveService.important(taxpayer),
  });
  const items = q.data?.items ?? [];
  if (!q.data?.enabled || items.length === 0) return null;
  return (
    <Card role="region" aria-labelledby="important-documents-title">
      <CardHeader>
        <CardTitle id="important-documents-title">
          {t("taxpayer.important_title")}
        </CardTitle>
        <CardDescription>{t("taxpayer.important_description")}</CardDescription>
      </CardHeader>
      <CardContent>
        <ul className="border-line divide-line divide-y rounded-lg border">
          {items.map((d) => {
            const urgent =
              d.expiry_status === "expiring" || d.expiry_status === "expired";
            return (
              <li
                key={d.uuid}
                data-urgent={urgent || undefined}
                className={cn(
                  "flex items-center gap-3 px-3 py-2.5",
                  d.expiry_status === "expiring" && "bg-warn-tint",
                  d.expiry_status === "expired" && "bg-danger-tint",
                )}
              >
                <Star
                  aria-hidden
                  className={cn(
                    "size-5 shrink-0",
                    urgent ? "text-warn" : "text-text-3",
                  )}
                />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">
                    {d.title}
                  </span>
                  <span className="text-text-3 block text-xs">
                    {d.important_kind
                      ? t(`taxpayer.important_kind_${d.important_kind}`)
                      : null}
                    <Validity d={d} intl={intl} />
                  </span>
                </span>
                <DownloadButton taxpayer={taxpayer} d={d} />
              </li>
            );
          })}
        </ul>
      </CardContent>
    </Card>
  );
}

function SharedDocuments({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const intl = locale === "tr" ? "tr-TR" : "en-GB";
  const list = useQuery({
    queryKey: taxpayerArchiveKeys.documents(taxpayer),
    queryFn: () => taxpayerArchiveService.documents(taxpayer),
  });
  const groups = new Map<string, TaxpayerArchiveDocument[]>();
  for (const c of CATEGORIES) groups.set(c, []);
  for (const d of list.data?.items ?? []) {
    groups.set(d.category, [...(groups.get(d.category) ?? []), d]);
  }
  const filled = [...groups.entries()].filter(([, docs]) => docs.length > 0);

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("taxpayer.archive_title")}</CardTitle>
        <CardDescription>{t("taxpayer.archive_description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-5">
        {list.isLoading ? (
          <div className="space-y-2">
            <Skeleton className="h-12 w-full" />
            <Skeleton className="h-12 w-full" />
          </div>
        ) : list.isError ? (
          <ErrorState
            title={t("taxpayer.archive_error")}
            description={list.error.message}
            onRetry={() => void list.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : filled.length === 0 ? (
          <BosDurum
            className="border-0"
            baslik={t("taxpayer.archive_empty_title")}
            aciklama={t("taxpayer.archive_empty")}
          />
        ) : (
          filled.map(([category, docs]) => (
            <section key={category} aria-labelledby={`archive-${category}`}>
              <h2
                id={`archive-${category}`}
                className="text-text-2 mb-2 text-sm font-semibold"
              >
                {t(`taxpayer.archive_category_${category}`)}
              </h2>
              <ul className="border-line divide-line divide-y rounded-lg border">
                {docs.map((d) => (
                  <li
                    key={d.uuid}
                    className="flex items-center gap-3 px-3 py-2.5"
                  >
                    <FileText
                      aria-hidden
                      className="text-text-3 size-5 shrink-0"
                    />
                    <span className="min-w-0 flex-1">
                      <span className="block truncate text-sm font-medium">
                        {d.title}
                      </span>
                      <span className="text-text-3 block text-xs">
                        {formatDate(d.document_date ?? d.created_at, intl)}
                        <Validity d={d} intl={intl} />
                      </span>
                    </span>
                    <DownloadButton taxpayer={taxpayer} d={d} />
                  </li>
                ))}
              </ul>
            </section>
          ))
        )}
      </CardContent>
    </Card>
  );
}
