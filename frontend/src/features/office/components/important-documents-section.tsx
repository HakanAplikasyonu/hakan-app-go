"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, FileText, Plus } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { BosDurum } from "@/components/hakan";
import { StatusPill } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { Switch } from "@/components/ui/switch";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  archiveKeys,
  archiveService,
  importantSettingsKey,
  importantSettingsService,
  type ArchiveDocument,
  type ArchiveQuery,
} from "../services/archive-documents.service";
import { UploadDialog } from "./engagement-archive-page";

const QUERY: ArchiveQuery = { important: true, limit: 100, offset: 0 };

function formatDay(value: string, locale: string) {
  const d = new Date(`${value}T00:00:00`);
  return Number.isNaN(d.getTime())
    ? value
    : d.toLocaleDateString(locale === "tr" ? "tr-TR" : "en-GB");
}

/**
 * Mükellef sayfasında "Önemli belgeler" bölümü (#780 B): vergi levhası, imza
 * sirküleri, ticaret sicil gazetesi, kira sözleşmesi, ruhsat, vekâletname...
 * Belge arşivinin "önemli" işaretli belgeleri; bitişi 30 gün içinde olan
 * (ya da geçmiş) belge renkli satırda. Belge bazında "Mükellef görebilir"
 * açılıp kapanır. Ofis ayarından bölüm kapatılırsa hiç görünmez. Tek
 * düğme ("Önemli belge ekle") ve tek kısa açıklama; arşivden işaretleme
 * bağlantısı açıklamanın içinde bir kez.
 */
export function ImportantDocumentsSection({
  engagement,
}: {
  engagement: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const [adding, setAdding] = useState(false);
  const canWrite = can(permissions.tenant.documents.write);

  const settings = useQuery({
    queryKey: importantSettingsKey(slug),
    queryFn: () => importantSettingsService.get(slug),
  });
  const enabled = settings.data?.enabled !== false;
  const list = useQuery({
    queryKey: archiveKeys.list(slug, engagement, QUERY),
    queryFn: () => archiveService.list(slug, engagement, QUERY),
    enabled: enabled && !settings.isLoading,
  });
  const toggle = useMutation({
    mutationFn: (v: { doc: ArchiveDocument; visible: boolean }) =>
      archiveService.update(slug, engagement, v.doc.uuid, {
        visible_to_taxpayer: v.visible,
      }),
    onSuccess: (doc) => {
      void qc.invalidateQueries({
        queryKey: archiveKeys.all(slug, engagement),
      });
      toast.success(
        doc.visible_to_taxpayer
          ? t("office.important.shown", { title: doc.title })
          : t("office.important.hidden", { title: doc.title }),
      );
    },
    onError: (err) => toast.error(err.message || t("office.archive.error")),
  });

  if (settings.isLoading || !enabled) return null;

  const items = list.data?.items ?? [];
  const soon = items.filter(
    (d) => d.expiry_status === "expiring" || d.expiry_status === "expired",
  ).length;

  return (
    <section
      aria-labelledby="important-docs-title"
      className="bg-surface border-line rounded-xl border p-4 md:p-5"
    >
      <div className="mb-3 flex flex-wrap items-start justify-between gap-3">
        <div className="space-y-1">
          <h2
            id="important-docs-title"
            className="font-display text-base font-bold"
          >
            {t("office.important.title")}
          </h2>
          {/* Tek kısa açıklama; arşivden işaretleme bağlantısı yalnız burada
              (Oğuz'un demo geri bildirimi). */}
          <p className="text-text-2 text-sm" data-testid="important-docs-hint">
            {t("office.important.description")}{" "}
            <Link
              href={routes.tenant.engagementArchive(slug, engagement)}
              className="text-brand font-semibold underline-offset-2 hover:underline"
            >
              {t("office.important.from_archive")}
            </Link>
          </p>
          {soon > 0 ? (
            <p className="text-warn text-sm font-semibold">
              {t("office.important.soon_summary", { count: soon })}
            </p>
          ) : null}
        </div>
        {canWrite ? (
          <Button size="sm" onClick={() => setAdding(true)}>
            <Plus className="size-4" aria-hidden />
            {t("office.important.add")}
          </Button>
        ) : null}
      </div>

      {list.isLoading ? (
        <Skeleton className="h-24 w-full" />
      ) : list.isError ? (
        <ErrorState
          title={t("office.important.error")}
          onRetry={() => void list.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : items.length === 0 ? (
        <BosDurum
          className="border-0 py-4"
          baslik={t("office.important.empty")}
          aciklama={t("office.important.empty_hint")}
        />
      ) : (
        <ul className="divide-line divide-y">
          {items.map((d) => {
            const urgent =
              d.expiry_status === "expiring" || d.expiry_status === "expired";
            return (
              <li
                key={d.uuid}
                data-urgent={urgent || undefined}
                className={cn(
                  "flex flex-wrap items-center gap-3 py-2.5",
                  urgent && "bg-warn-tint -mx-2 rounded-md px-2",
                  d.expiry_status === "expired" && "bg-danger-tint",
                )}
              >
                <FileText aria-hidden className="text-text-3 size-5 shrink-0" />
                <span className="min-w-0 flex-1">
                  <span className="block truncate text-sm font-medium">
                    {d.title}
                  </span>
                  <span className="text-text-3 block text-xs">
                    {d.important_kind
                      ? t(`office.important.kind_${d.important_kind}`)
                      : null}
                    {d.valid_until
                      ? ` · ${t("office.important.valid_until", {
                          date: formatDay(d.valid_until, locale),
                        })}`
                      : ""}
                    {d.note ? ` · ${d.note}` : ""}
                  </span>
                </span>
                {d.expiry_status === "expired" ? (
                  <StatusPill tone="danger">
                    {t("office.archive.expired", {
                      count: -(d.days_left ?? 0),
                    })}
                  </StatusPill>
                ) : d.expiry_status === "expiring" ? (
                  <StatusPill tone="warn">
                    {t("office.archive.expiring_in", {
                      count: d.days_left ?? 0,
                    })}
                  </StatusPill>
                ) : null}
                <Label className="flex items-center gap-2 text-xs font-normal">
                  <Switch
                    checked={d.visible_to_taxpayer}
                    disabled={!canWrite || toggle.isPending}
                    aria-label={t("office.important.visible_named", {
                      title: d.title,
                    })}
                    onCheckedChange={(v) =>
                      toggle.mutate({ doc: d, visible: v })
                    }
                  />
                  <span className="text-text-2">
                    {t("office.important.visible")}
                  </span>
                </Label>
                <Button
                  size="sm"
                  variant="ghost"
                  aria-label={t("office.archive.download", {
                    title: d.title,
                  })}
                  onClick={() =>
                    void archiveService
                      .download(slug, engagement, d)
                      .catch((err: Error) =>
                        toast.error(
                          err?.message || t("office.archive.file_error"),
                        ),
                      )
                  }
                >
                  <Download className="size-4" aria-hidden />
                </Button>
              </li>
            );
          })}
        </ul>
      )}

      {adding ? (
        <UploadDialog
          engagement={engagement}
          defaultCategory="general"
          defaultImportant="tax_certificate"
          onClose={() => setAdding(false)}
        />
      ) : null}
    </section>
  );
}
