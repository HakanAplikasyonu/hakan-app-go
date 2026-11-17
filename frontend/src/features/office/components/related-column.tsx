"use client";

import type { ColumnDef, RowData } from "@tanstack/react-table";
import { Link2 } from "lucide-react";
import Link from "next/link";
import { usePathname, useSearchParams } from "next/navigation";
import { useCallback, useMemo } from "react";

import { createColumn } from "@/components/tables";
import { useLocale } from "@/providers/locale-provider";

import { useEngagementScope } from "../engagement-scope";
import { FOCUS_PARAM } from "../lib/related-href";
import type { RelatedSourceKind } from "../services/related.service";

/**
 * Aynı sekmede bir kaydı seçili gösteren adres (`?kayit=<tür>:<uuid>`);
 * süzgeç ve sayfa parametreleri korunur (#469).
 */
export function useFocusHref() {
  const pathname = usePathname();
  const params = useSearchParams();
  return useCallback(
    (kind: RelatedSourceKind, uuid: string) => {
      const next = new URLSearchParams(params.toString());
      next.set(FOCUS_PARAM, `${kind}:${uuid}`);
      return `${pathname}?${next.toString()}`;
    },
    [pathname, params],
  );
}

/**
 * Mükellef detayındaki listelerde satırın "İlişkili kayıtlar" bağlantısı:
 * kayıt sekmenin üstünde seçili açılır (oluşan fiş, uyarılar, kaynak
 * belge). Genel (ofis düzeyi) sayfada sütun yok.
 */
export function useRelatedColumn<T extends RowData>(
  kind: RelatedSourceKind,
  getUuid: (row: T) => string,
  getLabel: (row: T) => string,
): ColumnDef<T, unknown>[] {
  const { t } = useLocale();
  const scope = useEngagementScope();
  const focusHref = useFocusHref();
  return useMemo<ColumnDef<T, unknown>[]>(
    () =>
      scope
        ? [
            createColumn<T>({
              id: "related",
              labelKey: "office.related.title",
              enableSorting: false,
              enableHiding: false,
              meta: { srOnlyHeader: true },
              size: 44,
              cell: ({ row }) => (
                <Link
                  href={focusHref(kind, getUuid(row.original))}
                  scroll={false}
                  onClick={(e) => e.stopPropagation()}
                  aria-label={`${t("office.related.title")}: ${getLabel(row.original)}`}
                  title={t("office.related.title")}
                  className="text-text-2 hover:text-foreground focus-visible:ring-ring inline-flex size-11 items-center justify-center rounded-md focus-visible:ring-2 focus-visible:outline-none md:size-8"
                >
                  <Link2 className="size-4" aria-hidden />
                </Link>
              ),
            }),
          ]
        : [],
    // getUuid / getLabel satır alanlarını okur; tür ve adres yeter.
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [scope, kind, t, focusHref],
  );
}
