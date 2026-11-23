"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { Upload } from "lucide-react";
import { useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  bankPostingService,
  type BankStatementAnalysis,
  type BankStatementOverrides,
  type BankStatementReport,
} from "../services/bank-posting.service";
import {
  BankStatementPreview,
  type PreviewDecision,
} from "./bank-statement-preview";

/** Backend hata kodları: Türkçe ileti (ham İngilizce ileti gösterilmez). */
const UPLOAD_ERRORS = [
  "BANK_ACCOUNT_REQUIRED",
  "UNKNOWN_BANK_ACCOUNT",
  "UNSUPPORTED_FILE",
  "SCANNED_PDF",
  // #780: bozuk, şifreli ya da tablosuz PDF: Excel / CSV indirilir.
  "PDF_UNREADABLE",
  "STATEMENT_BALANCE_MISMATCH",
  "INVALID_OVERRIDES",
  // #780: okunamayan dosyada beklenen kolonlar söylenir.
  "STATEMENT_UNREADABLE",
  "STATEMENT_LAYOUT_UNKNOWN",
  "STATEMENT_XLS_LEGACY",
] as const;

/** Bu güven ve üstündeki hesap önerileri tek tıkla toplu onaylanabilir. */
export const HIGH_CONFIDENCE = 0.7;

/** Raporun yeni hareket eklenen dönemleri (YYYY-MM; en çok 12). */
export function periodsOf(rep: BankStatementReport): string[] {
  const out = new Set<string>();
  for (const a of rep.accounts) {
    if (a.inserted === 0 || !a.from || !a.to) continue;
    let [y, m] = a.from.slice(0, 7).split("-").map(Number);
    const end = a.to.slice(0, 7);
    for (let i = 0; i < 12; i++) {
      const p = `${y}-${String(m).padStart(2, "0")}`;
      out.add(p);
      if (p >= end) break;
      m++;
      if (m > 12) [y, m] = [y + 1, 1];
    }
  }
  return [...out].sort();
}

/**
 * Ekstre yükleme (#383, #669 ekstre). MT940 doğrudan okunur. Excel, CSV ve
 * metin PDF önce deneme okumasından geçer, sonra Excel benzeri önizlemede
 * gösterilir: kolonların anlamı, dışarıda bırakılacak satırlar, okunamayan
 * hücreler. Biçim tanınmadıysa kolonlar başlık adlarından (ve AI'dan)
 * önerilir. Onaylanan eşleme ofis şablonu olarak kaydedilir; aynı düzendeki
 * bir sonraki ekstrede kendiliğinden gelir. Yükleme bitince hesap önerisi
 * kendiliğinden çalışır; yüksek güvenli öneriler bildirimden tek tıkla
 * onaylanır, ardından taslak fiş oluşturulur.
 */
export function BankStatementUpload({
  engagement,
  variant = "outline",
}: {
  engagement: string;
  /** Sayfanın birincil eylemiyse "default" (#710: tek birincil eylem). */
  variant?: "default" | "outline";
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can("tenant.bank.write");
  const qc = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [analysis, setAnalysis] = useState<BankStatementAnalysis | null>(null);
  // Dosya hesap taşımıyorsa (#780) müşavir hesabı seçer; seçim bu dosyanın
  // deneme okumasına, önizlemesine ve içe aktarmasına gider.
  const [needBox, setNeedBox] = useState<File | null>(null);
  const [box, setBox] = useState("");
  const boxes = useQuery({
    queryKey: ["tenant", slug.toLowerCase(), "cash-boxes", engagement],
    queryFn: () => bankPostingService.cashBoxes(slug, engagement),
    enabled: needBox !== null,
  });
  const activeBoxes = (boxes.data?.items ?? []).filter(
    (b) => b.type === "bank_account" && b.active,
  );

  const uploadError = (e: unknown) => {
    const code = isApiError(e) ? e.code : "";
    return (UPLOAD_ERRORS as readonly string[]).includes(code)
      ? t(`office.bank.upload.error_${code}`)
      : t("office.bank.upload.failed");
  };

  const refresh = () => {
    for (const k of [
      "bank-transactions",
      "bank-postings",
      "bank-ledger-accounts",
    ])
      void qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase(), k, engagement],
      });
  };

  const makeDrafts = async (periods: string[]) => {
    try {
      let drafts = 0;
      let count = 0;
      for (const p of periods) {
        const r = await bankPostingService.drafts(slug, engagement, p);
        drafts += r.drafts.length;
        count += r.transactions;
      }
      toast.success(t("office.bank.posting.drafts_done", { drafts, count }));
      refresh();
    } catch (e) {
      toast.error(uploadError(e));
    }
  };

  const approveHigh = async (ids: string[], periods: string[]) => {
    try {
      const r = await bankPostingService.approve(slug, engagement, ids);
      toast.success(
        t("office.bank.posting.approve_done", {
          approved: r.approved,
          skipped: r.skipped,
        }),
        {
          duration: 30_000,
          action: {
            label: t("office.bank.upload.make_drafts"),
            onClick: () => void makeDrafts(periods),
          },
        },
      );
      refresh();
    } catch (e) {
      toast.error(uploadError(e));
    }
  };

  // Yüklemeden sonra hesap önerisi kendiliğinden çalışır (eşleşen cari ve
  // işlem türüne göre); yüksek güvenliler bildirimden toplu onaylanır.
  const suggestAfter = async (rep: BankStatementReport) => {
    const periods = periodsOf(rep);
    if (!canWrite || periods.length === 0) return;
    try {
      let count = 0;
      const high: string[] = [];
      for (const p of periods) {
        const r = await bankPostingService.suggest(slug, engagement, p);
        count += r.suggested + r.auto_approved;
        const page = await bankPostingService.list(slug, engagement, {
          period: p,
          status: "suggested",
          limit: 200,
          offset: 0,
        });
        for (const x of page.items)
          if (
            x.suggestion_status === "pending" &&
            Number(x.confidence ?? 0) >= HIGH_CONFIDENCE
          )
            high.push(x.uuid);
      }
      refresh();
      if (count === 0) return;
      toast.info(
        t("office.bank.upload.suggested", { count, high: high.length }),
        high.length > 0
          ? {
              duration: 30_000,
              action: {
                label: t("office.bank.upload.approve_high", {
                  count: high.length,
                }),
                onClick: () => void approveHigh(high, periods),
              },
            }
          : undefined,
      );
    } catch {
      // Öneri çalışmazsa yükleme yine tamamdır; Hesap eşleştirme
      // sekmesinden elle çalıştırılabilir.
    }
  };

  const done = (rep: BankStatementReport) => {
    const inserted = rep.accounts.reduce((n, a) => n + a.inserted, 0);
    const already = rep.accounts.reduce((n, a) => n + a.merged + a.skipped, 0);
    if (inserted === 0 && already > 0)
      // Aynı ekstre ikinci kez: hiçbir şey eklenmez, açıkça söylenir.
      toast.warning(t("office.bank.upload.already_loaded", { count: already }));
    else
      toast.success(
        t("office.bank.upload.done", {
          count: inserted,
          bank: rep.bank_id ? t(`office.bank.banks.${rep.bank_id}`) : "—",
        }),
      );
    setAnalysis(null);
    refresh();
    void suggestAfter(rep);
  };

  const importFile = async (
    f: File,
    o?: BankStatementOverrides,
    cashBox = box,
  ) => {
    done(
      await bankPostingService.importStatement(
        slug,
        engagement,
        f,
        o,
        false,
        cashBox || undefined,
      ),
    );
  };

  const open = async (f: File, cashBox = "") => {
    setBusy(true);
    setBox(cashBox);
    try {
      // Deneme okuması: hesap ve biçim hataları hemen söylenir.
      try {
        const dry = await bankPostingService.importStatement(
          slug,
          engagement,
          f,
          undefined,
          true,
          cashBox || undefined,
        );
        if (dry.format === "mt940") {
          await importFile(f, undefined, cashBox);
          return;
        }
      } catch (e) {
        // Dosyada hesap yok: hesap seçimi istenir (#780).
        if (isApiError(e) && e.code === "BANK_ACCOUNT_REQUIRED" && !cashBox) {
          setNeedBox(f);
          return;
        }
        // Bilinmeyen düzen: kolonlar önizlemede seçilir.
        if (!isApiError(e) || e.code !== "STATEMENT_LAYOUT_UNKNOWN") throw e;
      }
      setAnalysis(await bankPostingService.analyzeStatement(slug, f));
    } catch (e) {
      toast.error(uploadError(e));
    } finally {
      setBusy(false);
    }
  };

  const confirm = async (d: PreviewDecision) => {
    if (!analysis || !file) return;
    setBusy(true);
    try {
      if (d.saveTemplate) {
        await bankPostingService.saveTemplate(slug, {
          format: analysis.format,
          bank_id: analysis.bank_id || undefined,
          name: d.templateName || undefined,
          columns: d.overrides.columns,
        });
      }
      await importFile(file, d.overrides);
    } catch (e) {
      toast.error(uploadError(e));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <input
        ref={input}
        type="file"
        className="sr-only"
        accept=".xlsx,.xls,.csv,.txt,.pdf,.sta,.940,.mt940"
        aria-label={t("office.bank.upload.button")}
        onChange={(e) => {
          const f = e.target.files?.[0];
          e.target.value = "";
          if (!f) return;
          setFile(f);
          void open(f);
        }}
      />
      <Button
        variant={variant}
        disabled={busy}
        onClick={() => input.current?.click()}
      >
        <Upload className="size-4" aria-hidden />
        {t("office.bank.upload.button")}
      </Button>

      {analysis && file ? (
        <BankStatementPreview
          key={`${file.name}-${file.lastModified}`}
          fileName={file.name}
          analysis={analysis}
          busy={busy}
          tryRead={(o) =>
            bankPostingService.importStatement(
              slug,
              engagement,
              file,
              o,
              true,
              box || undefined,
            )
          }
          onCancel={() => setAnalysis(null)}
          onConfirm={(d) => void confirm(d)}
        />
      ) : null}

      <Dialog
        open={needBox !== null}
        onOpenChange={(o) => {
          if (!o) setNeedBox(null);
        }}
      >
        <DialogContent data-testid="bank-statement-account">
          <DialogHeader>
            <DialogTitle>{t("office.bank.upload.pick_title")}</DialogTitle>
            <DialogDescription>
              {activeBoxes.length > 0 || boxes.isLoading
                ? t("office.bank.upload.pick_description")
                : t("office.bank.upload.pick_none")}
            </DialogDescription>
          </DialogHeader>
          {activeBoxes.length > 0 ? (
            <Select value={box} onValueChange={setBox}>
              <SelectTrigger aria-label={t("office.bank.upload.pick_label")}>
                <SelectValue placeholder={t("office.bank.upload.pick_label")} />
              </SelectTrigger>
              <SelectContent>
                {activeBoxes.map((b) => (
                  <SelectItem key={b.uuid} value={b.uuid}>
                    {b.name}
                    {b.bank_id
                      ? ` · ${t(`office.bank.banks.${b.bank_id}`)}`
                      : ""}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setNeedBox(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              disabled={!box || busy}
              onClick={() => {
                const f = needBox;
                setNeedBox(null);
                if (f) void open(f, box);
              }}
            >
              {t("office.bank.upload.pick_continue")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
