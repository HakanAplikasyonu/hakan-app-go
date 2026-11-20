"use client";

import { useQueryClient } from "@tanstack/react-query";
import { Upload } from "lucide-react";
import { useRef, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

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
  "STATEMENT_BALANCE_MISMATCH",
  "INVALID_OVERRIDES",
] as const;

/**
 * Ekstre yükleme (#383, #669 ekstre). MT940 doğrudan okunur. Excel, CSV ve
 * metin PDF önce deneme okumasından geçer, sonra Excel benzeri önizlemede
 * gösterilir: kolonların anlamı, dışarıda bırakılacak satırlar, okunamayan
 * hücreler. Biçim tanınmadıysa kolonlar başlık adlarından (ve AI'dan)
 * önerilir. Onaylanan eşleme ofis şablonu olarak kaydedilir; aynı düzendeki
 * bir sonraki ekstrede kendiliğinden gelir.
 */
export function BankStatementUpload({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [analysis, setAnalysis] = useState<BankStatementAnalysis | null>(null);

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

  const done = (rep: BankStatementReport) => {
    const inserted = rep.accounts.reduce((n, a) => n + a.inserted, 0);
    toast.success(
      t("office.bank.upload.done", {
        count: inserted,
        bank: rep.bank_id ? t(`office.bank.banks.${rep.bank_id}`) : "—",
      }),
    );
    setAnalysis(null);
    refresh();
  };

  const importFile = async (f: File, o?: BankStatementOverrides) => {
    done(await bankPostingService.importStatement(slug, engagement, f, o));
  };

  const open = async (f: File) => {
    setBusy(true);
    try {
      // Deneme okuması: hesap ve biçim hataları hemen söylenir.
      try {
        const dry = await bankPostingService.importStatement(
          slug,
          engagement,
          f,
          undefined,
          true,
        );
        if (dry.format === "mt940") {
          await importFile(f);
          return;
        }
      } catch (e) {
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
        accept=".xlsx,.csv,.txt,.pdf,.sta,.940,.mt940"
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
        variant="outline"
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
            bankPostingService.importStatement(slug, engagement, file, o, true)
          }
          onCancel={() => setAnalysis(null)}
          onConfirm={(d) => void confirm(d)}
        />
      ) : null}
    </>
  );
}
