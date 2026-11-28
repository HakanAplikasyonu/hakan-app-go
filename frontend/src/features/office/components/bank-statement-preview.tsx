"use client";

import { useEffect, useMemo, useState } from "react";

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
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import type {
  BankStatementAnalysis,
  BankStatementField,
  BankStatementOverrides,
  BankStatementReport,
} from "../services/bank-posting.service";
import { safeAnalysis } from "../lib/bank-statement";

export const STATEMENT_FIELDS: BankStatementField[] = [
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

/** Önizlemede müşavirin kararları ve şablon adı. */
export type PreviewDecision = {
  overrides: BankStatementOverrides;
  /** Eşleme önerilenden farklı ya da ofis şablonu yoksa kaydedilir. */
  saveTemplate: boolean;
  templateName: string;
};

/**
 * Ekstre önizlemesi (Excel benzeri): dosya hangi biçimde gelirse gelsin
 * (Excel, CSV, metin PDF) aynı tabloya çevrilmiş halidir. Kolonların
 * anlamı başlıktan seçilir, gereksiz satırlar (devreden, toplam …) işareti
 * kaldırılarak dışarıda bırakılır, okunamayan hücreler yerinde düzeltilir.
 * Kolon eşlemesi değişince sunucu deneme okuması yapar; sayılar ve
 * okunamayan satırlar tazelenir. Seçilen eşleme ofis şablonu olur.
 */
export function BankStatementPreview({
  fileName,
  analysis: raw,
  busy,
  tryRead,
  onCancel,
  onConfirm,
}: {
  fileName: string;
  analysis: BankStatementAnalysis;
  busy: boolean;
  /** Deneme okuması (yazmadan): eşleme ve düzeltmelerle. */
  tryRead: (o: BankStatementOverrides) => Promise<BankStatementReport>;
  onCancel: () => void;
  onConfirm: (d: PreviewDecision) => void;
}) {
  const { t } = useLocale();
  const analysis = useMemo(() => safeAnalysis(raw), [raw]);
  const initial = useMemo(
    () => analysis.columns.map((c) => c.field),
    [analysis],
  );
  const [fields, setFields] = useState<BankStatementField[]>(initial);
  const [skip, setSkip] = useState<Set<number>>(
    () =>
      new Set(
        // Toplam satırı ve sonrası baştan dışarıda; okunamayan satırlar
        // işaretli kalır (düzeltilmezse okunmaz, sayıda görünür).
        analysis.rows.filter((r) => r.kind === "skip").map((r) => r.row),
      ),
  );
  const [edits, setEdits] = useState<Map<string, string>>(new Map());
  const [name, setName] = useState("");
  const [report, setReport] = useState<BankStatementReport | null>(null);
  const [readError, setReadError] = useState(false);

  const used = (f: BankStatementField) => fields.filter((x) => x === f).length;
  const valid =
    used("date") === 1 &&
    (used("amount") === 1 || (used("debit") === 1 && used("credit") === 1)) &&
    STATEMENT_FIELDS.every((f) => used(f) <= 1);
  const changed = fields.some((f, i) => f !== initial[i]);
  const officeTemplate = analysis.matched?.source === "office";

  const overrides = useMemo<BankStatementOverrides>(
    () => ({
      columns: analysis.columns.map((c, i) => ({
        header: c.header,
        field: fields[i] ?? "",
      })),
      skip_rows: [...skip].sort((a, b) => a - b),
      edits: [...edits].map(([key, value]) => {
        const [row, column] = key.split(":").map(Number);
        return { row, column, value };
      }),
    }),
    [analysis, fields, skip, edits],
  );

  // Deneme okuması: eşleme, dışarıda bırakılan satırlar ya da düzeltmeler
  // değişince (kısa gecikmeyle) sayılar ve okunamayan satırlar tazelenir.
  const key = JSON.stringify(overrides);
  useEffect(() => {
    if (!valid) return;
    let live = true;
    const timer = window.setTimeout(() => {
      tryRead(overrides)
        .then((r) => {
          if (!live) return;
          setReport(r);
          setReadError(false);
        })
        .catch(() => {
          if (!live) return;
          setReport(null);
          setReadError(true);
        });
    }, 400);
    return () => {
      live = false;
      window.clearTimeout(timer);
    };
    // overrides her çizimde yeni nesne; içerik anahtarı yeter.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [key, valid]);

  const errors = new Map<number, string>();
  const accounts = Array.isArray(report?.accounts) ? report.accounts : [];
  for (const a of accounts)
    for (const e of Array.isArray(a?.errors) ? a.errors : [])
      if (e && e.Row !== undefined) errors.set(e.Row, String(e.Reason ?? ""));
  const sum = (k: "inserted" | "merged" | "skipped" | "possible_duplicates") =>
    accounts.reduce(
      (n, a) => n + (Number.isFinite(Number(a?.[k])) ? Number(a?.[k]) : 0),
      0,
    );
  const already = sum("merged") + sum("skipped");

  const kindOf = (row: number, kind: string) => {
    if (skip.has(row)) return "left_out";
    if (report) return errors.has(row) ? "error" : "movement";
    return kind || "movement";
  };

  return (
    <Dialog open onOpenChange={(o) => (o ? null : onCancel())}>
      <DialogContent className="max-w-5xl" data-testid="bank-statement-preview">
        <DialogHeader>
          <DialogTitle>
            {analysis.matched
              ? t("office.bank.preview.title")
              : t("office.bank.upload.template_title")}
          </DialogTitle>
          <DialogDescription>
            {t("office.bank.preview.description", {
              file: fileName,
              bank: analysis.bank_id
                ? t(`office.bank.banks.${analysis.bank_id}`)
                : "—",
            })}
          </DialogDescription>
        </DialogHeader>
        <div className="space-y-3">
          {analysis.warnings.map((w, i) => (
            <p key={`${i}-${w}`} className="text-warn text-sm">
              {w}
            </p>
          ))}
          <div
            className="max-h-[55vh] overflow-auto rounded-lg border"
            data-testid="bank-statement-grid"
          >
            <Table containerClassName="overflow-visible" className="text-xs">
              <TableHeader className="bg-surface-2 sticky top-0 z-10">
                <TableRow>
                  <TableHead className="w-10 px-2 py-1.5 text-start">
                    {t("office.bank.preview.col_row")}
                  </TableHead>
                  {analysis.columns.map((c, i) => (
                    <TableHead
                      key={`${i}-${c.index}-${c.header}`}
                      className="min-w-36 px-2 py-1.5 text-start align-top"
                    >
                      <div className="mb-1 font-semibold">{c.header}</div>
                      <Select
                        value={fields[i] || NONE}
                        onValueChange={(v) => {
                          const next = [...fields];
                          next[i] = (v === NONE ? "" : v) as BankStatementField;
                          setFields(next);
                        }}
                      >
                        <SelectTrigger
                          aria-label={t("office.bank.preview.column_field", {
                            header: c.header,
                          })}
                          className="h-7 text-xs"
                        >
                          <SelectValue />
                        </SelectTrigger>
                        <SelectContent>
                          <SelectItem value={NONE}>
                            {t("office.bank.upload.field_none")}
                          </SelectItem>
                          {STATEMENT_FIELDS.map((f) => (
                            <SelectItem key={f} value={f}>
                              {t(`office.bank.upload.field_${f}`)}
                            </SelectItem>
                          ))}
                        </SelectContent>
                      </Select>
                    </TableHead>
                  ))}
                </TableRow>
              </TableHeader>
              <TableBody>
                {analysis.rows.map((r) => {
                  const kind = kindOf(r.row, r.kind);
                  const reason = errors.get(r.row) ?? r.reason ?? "";
                  return (
                    <TableRow
                      key={r.row}
                      data-testid="bank-statement-row"
                      data-kind={kind}
                      className={cn(
                        "border-line border-t",
                        kind === "left_out" && "text-text-3 line-through",
                        kind === "error" && "bg-danger-surface",
                      )}
                      title={kind === "error" ? reason : undefined}
                    >
                      <TableCell className="px-2 py-1">
                        <span className="inline-flex items-center gap-1.5">
                          <Checkbox
                            checked={!skip.has(r.row)}
                            aria-label={t("office.bank.preview.include_row", {
                              row: r.row,
                            })}
                            onCheckedChange={(v) =>
                              setSkip((s) => {
                                const n = new Set(s);
                                if (v) n.delete(r.row);
                                else n.add(r.row);
                                return n;
                              })
                            }
                          />
                          <span className="tabular-nums">{r.row}</span>
                        </span>
                      </TableCell>
                      {analysis.columns.map((c, k) => {
                        const id = `${r.row}:${c.index}`;
                        const value = edits.get(id) ?? r.cells[k] ?? "";
                        return (
                          <TableCell key={`${k}-${id}`} className="px-1 py-0.5">
                            {kind === "error" || edits.has(id) ? (
                              <Input
                                value={value}
                                aria-label={t("office.bank.preview.cell", {
                                  row: r.row,
                                  header: c.header,
                                })}
                                onChange={(e) =>
                                  setEdits((m) =>
                                    new Map(m).set(id, e.target.value),
                                  )
                                }
                                className="h-7 text-xs"
                              />
                            ) : (
                              <span className="block truncate px-1">
                                {value}
                              </span>
                            )}
                          </TableCell>
                        );
                      })}
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          </div>
          <div
            className="text-sm"
            aria-live="polite"
            data-testid="bank-statement-counts"
          >
            {!valid ? (
              <p className="text-warn">{t("office.bank.upload.need_fields")}</p>
            ) : readError ? (
              <p className="text-warn">
                {t("office.bank.preview.read_failed")}
              </p>
            ) : report ? (
              <p>
                {t("office.bank.preview.counts", {
                  inserted: sum("inserted"),
                  errors: errors.size,
                })}
                {already > 0
                  ? ` ${t("office.bank.preview.already", { count: already })}`
                  : ""}
                {sum("possible_duplicates") > 0
                  ? ` ${t("office.bank.preview.possible_duplicates", {
                      count: sum("possible_duplicates"),
                    })}`
                  : ""}
              </p>
            ) : (
              <p className="text-text-2">{t("office.bank.preview.reading")}</p>
            )}
          </div>
          {changed || !officeTemplate ? (
            <div className="max-w-sm space-y-1.5">
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
              <p className="text-text-3 text-xs">
                {t("office.bank.preview.template_hint")}
              </p>
            </div>
          ) : null}
        </div>
        <DialogFooter>
          <Button variant="outline" onClick={onCancel}>
            {t("common.cancel")}
          </Button>
          <Button
            disabled={!valid || busy || !report || sum("inserted") === 0}
            onClick={() =>
              onConfirm({
                overrides,
                saveTemplate: changed || !officeTemplate,
                templateName: name.trim(),
              })
            }
          >
            {t("office.bank.preview.import", { count: sum("inserted") })}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
