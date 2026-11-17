"use client";

import { useQueryClient } from "@tanstack/react-query";
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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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

import {
  bankPostingService,
  type BankStatementAnalysis,
  type BankStatementField,
} from "../services/bank-posting.service";

const FIELDS: BankStatementField[] = [
  "date",
  "time",
  "value_date",
  "description",
  "amount",
  "debit",
  "credit",
  "balance",
  "receipt",
  "reference",
  "channel",
  "counterparty",
  "counterparty_iban",
  "counterparty_tax_id",
];
const NONE = "__none";

/**
 * Ekstre yükleme (#383). Bilinen banka biçimi (başlangıç ya da ofis
 * şablonu) doğrudan okunur; bilinmeyen biçimde dosya analiz edilir, kolon
 * önerileri (AI ya da başlık adlarından) müşavire gösterilir, onaylanınca
 * şablon kaydedilir ve dosya yeniden yüklenir.
 */
/** Backend hata kodları: Türkçe ileti (ham İngilizce ileti gösterilmez). */
const UPLOAD_ERRORS = [
  "BANK_ACCOUNT_REQUIRED",
  "UNKNOWN_BANK_ACCOUNT",
  "UNSUPPORTED_FILE",
  "SCANNED_PDF",
  "STATEMENT_BALANCE_MISMATCH",
] as const;

export function BankStatementUpload({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [file, setFile] = useState<File | null>(null);
  const [analysis, setAnalysis] = useState<BankStatementAnalysis | null>(null);
  const [fields, setFields] = useState<BankStatementField[]>([]);
  const [name, setName] = useState("");

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

  const send = async (f: File) => {
    setBusy(true);
    try {
      const rep = await bankPostingService.importStatement(slug, engagement, f);
      const inserted = rep.accounts.reduce((n, a) => n + a.inserted, 0);
      toast.success(
        t("office.bank.upload.done", {
          count: inserted,
          bank: rep.bank_id ? t(`office.bank.banks.${rep.bank_id}`) : "—",
        }),
      );
      setAnalysis(null);
      refresh();
    } catch (e) {
      if (isApiError(e) && e.code === "STATEMENT_LAYOUT_UNKNOWN") {
        try {
          const a = await bankPostingService.analyzeStatement(slug, f);
          setAnalysis(a);
          setFields(a.columns.map((c) => c.field));
          setName("");
        } catch (e2) {
          toast.error(uploadError(e2));
        }
      } else {
        toast.error(uploadError(e));
      }
    } finally {
      setBusy(false);
    }
  };

  const used = (f: BankStatementField) => fields.filter((x) => x === f).length;
  const valid =
    used("date") === 1 &&
    (used("amount") === 1 || (used("debit") === 1 && used("credit") === 1)) &&
    FIELDS.every((f) => used(f) <= 1);

  const saveTemplate = async () => {
    if (!analysis || !file) return;
    setBusy(true);
    try {
      await bankPostingService.saveTemplate(slug, {
        format: analysis.format,
        bank_id: analysis.bank_id || undefined,
        name: name.trim() || undefined,
        columns: analysis.columns.map((c, i) => ({
          header: c.header,
          field: fields[i] ?? "",
        })),
      });
      toast.success(t("office.bank.upload.template_saved"));
    } catch (e) {
      toast.error(uploadError(e));
      setBusy(false);
      return;
    }
    await send(file);
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
          void send(f);
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

      <Dialog
        open={analysis !== null}
        onOpenChange={(o) => (o ? null : setAnalysis(null))}
      >
        <DialogContent className="max-w-3xl">
          <DialogHeader>
            <DialogTitle>{t("office.bank.upload.template_title")}</DialogTitle>
            <DialogDescription>
              {t("office.bank.upload.template_description")}
            </DialogDescription>
          </DialogHeader>
          {analysis ? (
            <div className="space-y-3">
              {analysis.warnings.map((w) => (
                <p key={w} className="text-warn text-sm">
                  {w}
                </p>
              ))}
              <div className="max-h-80 overflow-auto rounded-lg border">
                <table className="w-full text-sm">
                  <thead className="bg-surface-2">
                    <tr>
                      <th className="px-2 py-1.5 text-start">
                        {t("office.bank.upload.col_header")}
                      </th>
                      <th className="px-2 py-1.5 text-start">
                        {t("office.bank.upload.col_field")}
                      </th>
                      <th className="px-2 py-1.5 text-start">
                        {t("office.bank.upload.col_sample")}
                      </th>
                    </tr>
                  </thead>
                  <tbody>
                    {analysis.columns.map((c, i) => (
                      <tr key={`${c.index}-${c.header}`} className="border-t">
                        <td className="px-2 py-1.5 font-medium">{c.header}</td>
                        <td className="px-2 py-1.5">
                          <Select
                            value={fields[i] || NONE}
                            onValueChange={(v) => {
                              const next = [...fields];
                              next[i] = (
                                v === NONE ? "" : v
                              ) as BankStatementField;
                              setFields(next);
                            }}
                          >
                            <SelectTrigger
                              aria-label={c.header}
                              className="min-w-44"
                            >
                              <SelectValue />
                            </SelectTrigger>
                            <SelectContent>
                              <SelectItem value={NONE}>
                                {t("office.bank.upload.field_none")}
                              </SelectItem>
                              {FIELDS.map((f) => (
                                <SelectItem key={f} value={f}>
                                  {t(`office.bank.upload.field_${f}`)}
                                </SelectItem>
                              ))}
                            </SelectContent>
                          </Select>
                        </td>
                        <td className="text-text-2 max-w-48 truncate px-2 py-1.5">
                          {analysis.sample[0]?.[i] ?? ""}
                        </td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="template-name">
                  {t("office.bank.upload.template_name")}
                </Label>
                <Input
                  id="template-name"
                  value={name}
                  placeholder={
                    analysis.bank_id
                      ? t(`office.bank.banks.${analysis.bank_id}`)
                      : undefined
                  }
                  onChange={(e) => setName(e.target.value)}
                />
              </div>
              {!valid ? (
                <p className="text-warn text-sm">
                  {t("office.bank.upload.need_fields")}
                </p>
              ) : null}
            </div>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setAnalysis(null)}>
              {t("common.cancel")}
            </Button>
            <Button
              disabled={!valid || busy}
              onClick={() => void saveTemplate()}
            >
              {t("office.bank.upload.save_and_import")}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  );
}
