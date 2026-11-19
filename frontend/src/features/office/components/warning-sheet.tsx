"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { useLocale } from "@/providers/locale-provider";

import { warningRecordKind } from "../lib/related-href";
import { sheetPosition } from "../lib/warning-sheet-position";
import type { ApiDataWarning } from "../services/warnings.service";
import { Figure, SeverityBadge } from "./office-ui";
import { RecordDetail } from "./record-detail";
import { ApiWarningActions } from "./warning-actions";
import { useApiWarningText, useEntityLabel } from "./warnings-page";

/** Açık uyarının adres parametresi: dönünce kalınan yer korunur (#529). */
export const WARNING_PARAM = "uyari";

/**
 * Uyarı yan panelinin durumu (#529): hangi uyarının açık olduğu adreste
 * (`?uyari=<uuid>`), süzgeç ve sayfa parametreleri korunur. Açılan uyarı
 * ve sırası ayrıca tutulur: uyarı çözülüp listeden düşse de panel onu
 * gösterir ve "sıradaki" doğru yere gider.
 */
export function useWarningSheet() {
  const params = useSearchParams();
  const pathname = usePathname();
  const router = useRouter();
  const openUuid = params.get(WARNING_PARAM);
  const [last, setLast] = useState<{
    warning: ApiDataWarning;
    index: number;
  } | null>(null);
  const go = useCallback(
    (uuid: string | null) => {
      const next = new URLSearchParams(params.toString());
      if (uuid) next.set(WARNING_PARAM, uuid);
      else next.delete(WARNING_PARAM);
      const qs = next.toString();
      router.replace(`${pathname}${qs ? `?${qs}` : ""}`, { scroll: false });
    },
    [params, pathname, router],
  );
  const open = useCallback(
    (w: ApiDataWarning, index: number) => {
      setLast({ warning: w, index });
      go(w.uuid);
    },
    [go],
  );
  const close = useCallback(() => {
    setLast(null);
    go(null);
  }, [go]);
  return { openUuid, last, open, close };
}

/**
 * Uyarı yan paneli (#529): uyarının ne dediği, kaydın kendisi (banka
 * hareketinde eşleştirme, faturada görüntü), gerekçeli karar ve sıradaki
 * uyarıya geçiş. ← → (ya da k / j) ile gezinilir; karar verilince panel
 * sıradakini açar.
 */
export function WarningSheet({
  items,
  sheet,
}: {
  items: ApiDataWarning[];
  sheet: ReturnType<typeof useWarningSheet>;
}) {
  const { t } = useLocale();
  const text = useApiWarningText();
  const entity = useEntityLabel();
  const { openUuid, last, open, close } = sheet;
  // Sayfada birden çok liste (grup kartları) olabilir: panel yalnızca açık
  // uyarı bu listedeyse ya da buradan açıldıysa görünür.

  const { current, gone, index, prev, next } = sheetPosition(
    items,
    openUuid,
    last,
  );
  const goPrev = () => prev && open(prev, items.indexOf(prev));
  const goNext = () => next && open(next, items.indexOf(next));

  const onKey = (e: React.KeyboardEvent) => {
    const el = e.target as HTMLElement;
    if (el.closest("input, textarea, select, [contenteditable=true]")) return;
    if (e.key === "ArrowRight" || e.key === "j") {
      e.preventDefault();
      goNext();
    } else if (e.key === "ArrowLeft" || e.key === "k") {
      e.preventDefault();
      goPrev();
    }
  };

  const kind = current?.entity_uuid
    ? warningRecordKind(current.entity_type)
    : null;
  return (
    <Sheet open={current !== null} onOpenChange={(o) => !o && close()}>
      <SheetContent
        side="right"
        className="w-full overflow-y-auto sm:max-w-2xl"
        onKeyDown={onKey}
        data-testid="warning-sheet"
      >
        <SheetHeader className="text-left">
          <div className="flex items-center justify-between gap-2 pe-8">
            <SheetTitle>
              {current
                ? t("office.tax_returns.panel_title", {
                    record: entity(current.entity_type),
                  })
                : null}
            </SheetTitle>
            <div className="flex items-center gap-1">
              <span className="text-text-3 text-xs tabular-nums">
                {items.length > 0 && !gone
                  ? t("office.warning_sheet.position", {
                      index: index + 1,
                      total: items.length,
                    })
                  : null}
              </span>
              <Button
                size="icon"
                variant="ghost"
                aria-label={t("office.warning_sheet.prev")}
                aria-keyshortcuts="ArrowLeft"
                disabled={!prev}
                onClick={goPrev}
              >
                <ChevronLeft className="size-4" aria-hidden />
              </Button>
              <Button
                size="icon"
                variant="ghost"
                aria-label={t("office.warning_sheet.next")}
                aria-keyshortcuts="ArrowRight"
                disabled={!next}
                onClick={goNext}
              >
                <ChevronRight className="size-4" aria-hidden />
              </Button>
            </div>
          </div>
          <SheetDescription>{t("office.warning_sheet.hint")}</SheetDescription>
        </SheetHeader>

        {!current ? null : (
          <div className="mt-4 space-y-5">
            <section
              className="border-line space-y-2 rounded-xl border p-3"
              aria-live="polite"
            >
              <div className="flex flex-wrap items-center gap-2">
                <SeverityBadge severity={current.severity} />
                <Figure className="text-text-3 text-xs">
                  {[current.code, current.period].filter(Boolean).join(" · ")}
                </Figure>
              </div>
              <p className="text-sm">{text(current)}</p>
              {gone ? (
                <p className="text-ok text-sm font-medium">
                  {t("office.warning_sheet.closed")}
                </p>
              ) : (
                <ApiWarningActions
                  warning={current}
                  onDecided={() => {
                    // Karar verilen uyarı listeden düşecek; sıradakine geç.
                    const after = items[index + 1] ?? items[index - 1];
                    if (after && after.uuid !== current.uuid)
                      open(after, Math.max(index, 0));
                    else close();
                  }}
                />
              )}
              {gone && next ? (
                <Button size="sm" onClick={goNext}>
                  {t("office.warning_sheet.next")}
                  <ChevronRight className="size-4" aria-hidden />
                </Button>
              ) : null}
            </section>
            <RecordDetail
              engagement={current.engagement_uuid}
              entityType={current.entity_type}
              kind={kind}
              uuid={current.entity_uuid ?? null}
              warning={current.uuid}
            />
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}
