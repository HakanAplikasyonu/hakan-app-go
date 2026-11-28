"use client";

import { useMutation } from "@tanstack/react-query";
import { FileSpreadsheet } from "lucide-react";
import { useSearchParams } from "next/navigation";
import { useId, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Label } from "@/components/ui/label";
import { MonthPicker } from "@/components/ui/month-picker";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { isPeriod } from "../period";
import {
  decodeLucaReport,
  lucaListsService,
  type LucaList,
  type LucaListReport,
  type LucaListReportRow,
} from "../services/luca-lists.service";
import { useLedgerType } from "./ledger-type";

/** Sunucunun gerekçe kodları; bilinmeyen kod genel metinle gösterilir. */
const REASONS = new Set([
  "ITEM_ACCOUNT",
  "COUNTERPARTY_ACCOUNT",
  "VAT_ACCOUNT",
  "BANK_ACCOUNT",
  "COUNTER_ACCOUNT",
  "MAIN_ACCOUNT",
  "UNBALANCED",
  "NO_DATE",
  "NO_AMOUNT",
  "FOREIGN_CURRENCY",
  "SAME_ACCOUNT",
  "Z_BREAKDOWN",
  "NOT_ACTIVE",
  "RETURN_INVOICE",
  "WITHHOLDING",
  "EXEMPTION",
  "SPECIAL_TAX",
  "UNSUPPORTED_TYPE",
  "NO_ITEMS",
  "TOTALS_MISMATCH",
  "DOCUMENT_ALREADY_EXPORTED",
]);

/**
 * "Luca'ya aktar (Excel)" listelerde (Oğuz'un geri bildirimi): seçilen
 * dönemin gelen / giden faturaları, banka hareketleri ya da Z raporları
 * Luca fiş dosyası olarak iner. Luca'nın API'si yok: dosyayı müşavir
 * Luca'ya kendisi yükler. Daha önce aktarılan belge, açıkça yeniden dahil
 * edilmedikçe dosyaya girmez (çift kayıt olmasın).
 */
export function LucaListExport({
  engagement,
  period,
  lists,
}: {
  engagement: string;
  period?: string | null;
  lists: readonly LucaList[];
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const { singleEntry } = useLedgerType(engagement);
  const id = useId();
  // Dönem: adresteki `?period=` (uyarı / kart bağlantısı), yoksa sayfanın
  // dönemi; pencerede değiştirilebilir.
  const search = useSearchParams();
  const fromUrl = search.get("period");
  const initialMonth = fromUrl && isPeriod(fromUrl) ? fromUrl : (period ?? "");
  const [open, setOpen] = useState(false);
  const [picked, setPicked] = useState<LucaList | null>(null);
  const [month, setMonth] = useState(initialMonth);
  const [reinclude, setReinclude] = useState(false);
  const [report, setReport] = useState<LucaListReport | null>(null);
  const [empty, setEmpty] = useState(false);
  const list: LucaList =
    picked && lists.includes(picked) ? picked : (lists[0] ?? "z-reports");

  const run = useMutation({
    mutationFn: () =>
      lucaListsService.export(slug, engagement, list, {
        period: month,
        include_exported: reinclude || undefined,
      }),
    onSuccess: (h) => {
      const rep = decodeLucaReport(h.get("X-Luca-Report")) ?? {
        exported: Number(h.get("X-Luca-Exported") ?? 0),
        check: [],
        skipped: [],
        already_exported: [],
      };
      setReport(rep);
      setEmpty(false);
      toast.success(
        t("office.luca_lists.done_count", { count: rep.exported }),
      );
    },
    onError: (e) => {
      setReport(null);
      if ((e as { status?: number }).status === 422) {
        setEmpty(true);
        return;
      }
      setEmpty(false);
      toast.error(t("office.luca_lists.failed"));
    },
  });

  if (
    !engagement ||
    lists.length === 0 ||
    singleEntry ||
    !can(permissions.tenant.journals.write)
  )
    return null;

  const reason = (code: string) =>
    REASONS.has(code)
      ? t(`office.luca_lists.reason_${code}`)
      : t("office.luca_lists.reason_other");
  const rowText = (r: LucaListReportRow) =>
    [r.document_no, r.date].filter(Boolean).join(" · ") +
    ": " +
    reason(r.reason) +
    (r.account ? ` (${r.account})` : "");

  return (
    <>
      <Button
        variant="outline"
        size="sm"
        className="max-md:min-h-11"
        onClick={() => {
          // Her açılışta temiz pencere: sayfanın dönemi, kutu boş.
          setMonth(initialMonth);
          setReinclude(false);
          setReport(null);
          setEmpty(false);
          setPicked(null);
          setOpen(true);
        }}
        data-testid="luca-list-open"
      >
        <FileSpreadsheet className="size-4" aria-hidden />
        {t("office.luca_lists.button")}
      </Button>
      <Dialog
        open={open}
        onOpenChange={setOpen}
      >
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{t("office.luca_lists.title")}</DialogTitle>
            <DialogDescription>
              {t(`office.luca_lists.hint_${list}`)}{" "}
              {t("office.luca_lists.no_api")}
            </DialogDescription>
          </DialogHeader>
          {lists.length > 1 ? (
            <div className="flex flex-col gap-1">
              <Label
                htmlFor={`${id}-list`}
                className="text-xs leading-normal font-normal"
              >
                {t("office.luca_lists.list")}
              </Label>
              <NativeSelect
                id={`${id}-list`}
                className="w-56"
                value={list}
                onChange={(e) => {
                  setPicked(e.target.value as LucaList);
                  setReport(null);
                  setEmpty(false);
                }}
                data-testid="luca-list-kind"
              >
                {lists.map((l) => (
                  <NativeSelectOption key={l} value={l}>
                    {t(`office.luca_lists.list_${l}`)}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
          ) : null}
          <div className="flex flex-col gap-1">
            <Label
              htmlFor={`${id}-period`}
              className="text-xs leading-normal font-normal"
            >
              {t("office.luca_lists.period")}
            </Label>
            <MonthPicker
              id={`${id}-period`}
              className="w-48"
              value={month}
              onChange={(m) => {
                setMonth(m);
                setReport(null);
                setEmpty(false);
              }}
            />
          </div>
          {/* Çift aktarım koruması: daha önce Luca'ya giden belge yalnız bu
              çıktı için açıkça istenirse yeniden girer. */}
          <Label className="flex items-start gap-2 text-sm leading-normal font-normal">
            <Checkbox
              checked={reinclude}
              onCheckedChange={(v) => setReinclude(v === true)}
              data-testid="luca-list-reinclude"
            />
            <span>
              {t("office.luca_lists.reinclude")}
              <span className="text-text-3 block text-xs">
                {t("office.luca_lists.reinclude_hint")}
              </span>
            </span>
          </Label>
          {empty ? (
            <p
              role="status"
              className="bg-warn-tint text-warn rounded-md px-3 py-2 text-sm"
              data-testid="luca-list-empty"
            >
              {t("office.luca_lists.empty")}
            </p>
          ) : null}
          {report ? (
            <div
              role="status"
              className="border-line space-y-2 rounded-md border px-3 py-2 text-sm"
              data-testid="luca-list-result"
            >
              <p className="font-medium">
                {t("office.luca_lists.done_count", { count: report.exported })}
              </p>
              <ReportRows
                title={t("office.luca_lists.check_title", {
                  count: report.check.length,
                })}
                rows={report.check}
                text={rowText}
              />
              <ReportRows
                title={t("office.luca_lists.skipped_title", {
                  count: report.skipped.length,
                })}
                rows={report.skipped}
                text={rowText}
              />
              <ReportRows
                title={t("office.luca_lists.already_title", {
                  count: report.already_exported.length,
                })}
                rows={report.already_exported}
                text={rowText}
              />
            </div>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setOpen(false)}>
              {t("office.luca_lists.close")}
            </Button>
            <Button
              disabled={!month || run.isPending}
              onClick={() => run.mutate()}
              data-testid="luca-list-download"
            >
              <FileSpreadsheet className="size-4" aria-hidden />
              {t("office.luca_lists.download")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}

function ReportRows({
  title,
  rows,
  text,
}: {
  title: string;
  rows: LucaListReportRow[];
  text: (r: LucaListReportRow) => string;
}) {
  if (rows.length === 0) return null;
  return (
    <div>
      <p className="text-text-2 text-xs font-medium">{title}</p>
      <ul className="max-h-32 list-disc overflow-y-auto pl-5 text-xs">
        {rows.map((r, i) => (
          <li key={`${r.document_no}-${r.reason}-${i}`}>{text(r)}</li>
        ))}
      </ul>
    </div>
  );
}
