"use client";

import { useQuery } from "@tanstack/react-query";
import { Skeleton } from "@/components/ui/skeleton";
import { apiConfig } from "@/config/api";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { ORGANIZATION_HEADER } from "@/lib/api/tenant-request";
import { useLocale } from "@/providers/locale-provider";

/**
 * Fişin fotoğrafı (BFF oturumu eklenir) tarayıcı adresi olarak; görsel yoksa
 * null. Adres sorgu önbelleğinde kalır (aynı fişin alanları arasında
 * gezilirken yeniden indirilmez).
 */
async function fetchReceiptImage(
  slug: string,
  engagement: string,
  receipt: string,
): Promise<string | null> {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const response = await fetch(
    `${base}/v1/tenant/engagements/${encodeURIComponent(engagement)}/receipts/${encodeURIComponent(receipt)}/image`,
    { credentials: "include", headers: { [ORGANIZATION_HEADER]: slug } },
  );
  if (response.status === 404) return null;
  if (!response.ok) throw new Error(`receipt image ${response.status}`);
  return URL.createObjectURL(await response.blob());
}

/**
 * İnceleme kuyruğunda fişten okunan alanın yanında fişin fotoğrafı (#569):
 * müşavir "Fiş no = 7776" doğru mu diye belgeye bakar. Görsele tıklayınca
 * tam boy açılır.
 */
export function ReviewReceiptImage({
  engagement,
  receipt,
}: {
  engagement: string;
  receipt: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const q = useQuery({
    queryKey: [
      "tenant",
      slug.toLowerCase(),
      "receipt-image",
      engagement,
      receipt,
    ],
    queryFn: () => fetchReceiptImage(slug, engagement, receipt),
    staleTime: 5 * 60_000,
    retry: false,
  });
  const url = q.data;

  if (q.isPending) return <Skeleton className="h-48 w-full" />;
  if (q.isError)
    return (
      <p className="text-muted-foreground text-sm">
        {t("office.review.receipt_image_failed")}
      </p>
    );
  if (!url)
    return (
      <p className="text-muted-foreground text-sm">
        {t("office.review.receipt_image_none")}
      </p>
    );
  return (
    <a
      href={url}
      target="_blank"
      rel="noreferrer"
      className="focus-visible:ring-ring block rounded-md focus-visible:ring-2 focus-visible:outline-none"
      title={t("office.review.receipt_image_open")}
      data-testid="review-receipt-image"
    >
      {/* Oturumlu uçtan gelen blob: next/image kullanılamaz. */}
      {/* eslint-disable-next-line @next/next/no-img-element */}
      <img
        src={url}
        alt={t("office.review.receipt_image_alt")}
        className="max-h-96 w-auto max-w-full rounded-md border object-contain"
      />
    </a>
  );
}
