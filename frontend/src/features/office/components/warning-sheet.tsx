"use client";

import { ChevronLeft, ChevronRight } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useCallback, useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { warningRecordKind } from "../lib/related-href";
import { sheetPosition } from "../lib/warning-sheet-position";
import { warningTransactions } from "../lib/warning-transactions";
import type { ApiDataWarning } from "../services/warnings.service";
import { Figure, SeverityBadge } from "./office-ui";
import {
  BALANCE_WARNING_CODE,
  BankBalanceCheckPanel,
} from "./bank-balance-check";
import { RecordDetail } from "./record-detail";
import type { OpenRelated } from "./related-records";
import { ApiWarningActions } from "./warning-actions";
import { WarningCodeFix } from "./warning-code-fix";
import { WarningTransactions } from "./warning-transactions";
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
  const { slug } = useTenant();
  const { openUuid, last, open, close } = sheet;
  // Sayfada birden çok liste (grup kartları) olabilir: panel yalnızca açık
  // uyarı bu listedeyse ya da buradan açıldıysa görünür.

  const { current, gone, index, prev, next } = sheetPosition(
    items,
    openUuid,
    last,
  );
  // İlişkili kayıtlar panelin içinde açılır (#564): açılanlar yığında,
  // "Geri" bir öncekine, en sonunda uyarıya döner. Başka uyarıya geçince
  // yığın kendiliğinden boşalır.
  const [trail, setTrail] = useState<{
    warning: string;
    items: Parameters<OpenRelated>[0][];
  }>({ warning: "", items: [] });
  const stack = current && trail.warning === current.uuid ? trail.items : [];
  const top = stack.at(-1) ?? null;
  const openRelated: OpenRelated = (target) => {
    if (!current) return;
    setTrail({ warning: current.uuid, items: [...stack, target] });
  };
  const back = () => setTrail((s) => ({ ...s, items: s.items.slice(0, -1) }));
  const backRef = useRef<HTMLButtonElement>(null);
  const depth = stack.length;
  useEffect(() => {
    if (depth > 0) backRef.current?.focus();
  }, [depth, top?.uuid]);

  // Karar verilen uyarı listeden düşecek; sıradakine geç.
  const afterDecided = () => {
    if (!current) return;
    const after = items[index + 1] ?? items[index - 1];
    if (after && after.uuid !== current.uuid) open(after, Math.max(index, 0));
    else close();
  };

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

  // Olası mükerrer (#570): aynı görünen öteki fatura(lar) panelden açılır.
  const duplicates =
    current?.code === "POSSIBLE_DUPLICATE" &&
    Array.isArray(current.params?.candidates)
      ? current.params.candidates.filter(
          (x): x is string => typeof x === "string",
        )
      : [];
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

        {current && top ? (
          <div className="mt-4 space-y-5" data-testid="warning-related-record">
            <div className="space-y-1">
              <Button
                ref={backRef}
                size="sm"
                variant="ghost"
                className="-ms-2"
                onClick={back}
                data-testid="warning-related-back"
              >
                <ChevronLeft className="size-4" aria-hidden />
                {depth > 1
                  ? t("office.warning_sheet.related_back_step")
                  : t("office.warning_sheet.related_back")}
              </Button>
              <h3 className="text-sm font-semibold">
                {t("office.warning_sheet.related_title", {
                  kind: t(`office.related.kind.${top.kind}`),
                  record: top.label || "—",
                })}
              </h3>
            </div>
            <RecordDetail
              key={`${top.kind}:${top.uuid}`}
              engagement={current.engagement_uuid}
              entityType={top.kind}
              kind={top.kind}
              uuid={top.uuid}
              onOpenRelated={openRelated}
            />
          </div>
        ) : !current ? null : (
          <div className="mt-4 space-y-5">
            <section
              className="border-line space-y-2 rounded-xl border p-3"
              aria-live="polite"
            >
              <div className="flex flex-wrap items-center gap-2">
                <SeverityBadge severity={current.severity} />
                {current.period ? (
                  <Figure className="text-text-3 text-xs">
                    {current.period}
                  </Figure>
                ) : null}
              </div>
              <p className="text-sm">{text(current)}</p>
              {duplicates.length > 0 && current.engagement_uuid ? (
                <p className="text-sm" data-testid="duplicate-candidates">
                  {t("office.warning_sheet.duplicate_candidates")}{" "}
                  {duplicates.map((id, i) => (
                    <Link
                      key={id}
                      href={routes.tenant.engagementInvoice(
                        slug,
                        current.engagement_uuid,
                        id,
                      )}
                      className="text-brand-text me-2 underline-offset-4 hover:underline"
                    >
                      {t("office.warning_sheet.duplicate_open", {
                        index: String(i + 1),
                      })}
                    </Link>
                  ))}
                </p>
              ) : null}
              {!gone && kind === "invoice" ? (
                <p
                  className="text-text-3 text-xs"
                  data-testid="invoice-fix-hint"
                >
                  {t("office.warning_sheet.invoice_hint")}
                </p>
              ) : null}
              {gone ? (
                <p className="text-ok text-sm font-medium">
                  {t("office.warning_sheet.closed")}
                </p>
              ) : (
                <ApiWarningActions
                  warning={current}
                  requestText={text(current)}
                  onDecided={afterDecided}
                />
              )}
              {gone && next ? (
                <Button size="sm" onClick={goNext}>
                  {t("office.warning_sheet.next")}
                  <ChevronRight className="size-4" aria-hidden />
                </Button>
              ) : null}
            </section>
            {/* Fatura kalemi uyarısı (#570): kalemin hesap kodu burada düzeltilir. */}
            {!gone ? (
              <WarningCodeFix
                key={current.uuid}
                warning={current}
                onDone={afterDecided}
              />
            ) : null}
            {current.code === BALANCE_WARNING_CODE &&
            current.period &&
            current.engagement_uuid ? (
              <BankBalanceCheckPanel
                engagement={current.engagement_uuid}
                period={current.period}
              />
            ) : null}
            {/* Dönem uyarısı (#570): işaret ettiği hareketler burada çözülür. */}
            {!gone && kind !== "bank_transaction" && current.engagement_uuid ? (
              <WarningTransactions
                key={current.uuid}
                engagement={current.engagement_uuid}
                items={warningTransactions(current.params)}
              />
            ) : null}
            <RecordDetail
              engagement={current.engagement_uuid}
              entityType={current.entity_type}
              kind={kind}
              uuid={current.entity_uuid ?? null}
              warning={current.uuid}
              onOpenRelated={openRelated}
            />
          </div>
        )}
      </SheetContent>
    </Sheet>
  );
}
