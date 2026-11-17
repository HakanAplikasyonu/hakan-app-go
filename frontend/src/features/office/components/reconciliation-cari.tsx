"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import {
  AlertTriangle,
  FileText,
  Mail,
  MoreHorizontal,
  Plus,
  Send,
  Trash2,
  Upload,
  Users,
} from "lucide-react";
import { useMemo, useRef, useState } from "react";
import { toast } from "sonner";

import { EntityTable } from "@/components/entity";
import {
  createActionsColumn,
  createColumn,
  createDateColumn,
  createMoneyColumn,
} from "@/components/tables";
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
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  type BabsDeclaration,
  type BabsImportResult,
  type DecisionStatus,
  type ReconciliationCounterparty,
  type ReconciliationLetter,
  reconciliationKeys,
  reconciliationService,
} from "../services/reconciliation.service";
import { Money } from "./office-ui";
import { SummaryCards } from "./receipts-ui";
import {
  babsTone,
  decisionTone,
  errText,
  letterTone,
  NoteDialog,
  Pill,
  resultTone,
} from "./reconciliation-ui";

const selectClass =
  "bg-background focus-visible:ring-ring h-9 rounded-md border px-2 text-sm focus-visible:ring-2 focus-visible:outline-none max-md:h-11";

function BabsCell({
  form,
  amount,
  count,
  declared,
  declaredCount,
  result,
}: {
  form: string;
  amount: string;
  count: number;
  declared: string | null;
  declaredCount: number | null;
  result: string;
}) {
  const { t } = useLocale();
  if (result === "not_required" && declared === null) {
    return <span className="text-text-3 text-xs">{form} —</span>;
  }
  return (
    <div className="flex flex-col items-start gap-0.5">
      <span className="text-xs">
        {form}: <Money value={amount} /> ({count})
      </span>
      {declared !== null ? (
        <span className="text-text-2 text-xs">
          {t("office.reconciliation.declared")}: <Money value={declared} /> (
          {declaredCount ?? 0})
        </span>
      ) : null}
      <Pill group="babs" value={result} tones={babsTone} />
    </div>
  );
}

export function CounterpartiesView({
  engagement,
  period,
  canWrite,
}: {
  engagement: string;
  period: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const [onlyIssues, setOnlyIssues] = useState(true);
  const [letterFor, setLetterFor] = useState<ReconciliationCounterparty | null>(
    null,
  );
  const [noteFor, setNoteFor] = useState<{
    row: ReconciliationCounterparty;
    mode: "explain" | "review";
  } | null>(null);
  const key = reconciliationKeys.counterparties(slug, engagement, period);
  const list = useQuery({
    queryKey: key,
    queryFn: () =>
      reconciliationService.counterparties(slug, engagement, period),
  });
  const refresh = () => void qc.invalidateQueries({ queryKey: key });
  const decide = useMutation({
    mutationFn: (v: { uuid: string; status: DecisionStatus; note?: string }) =>
      reconciliationService.decideCounterparty(
        slug,
        engagement,
        v.uuid,
        v.status,
        v.note,
      ),
    onSuccess: () => {
      refresh();
      setNoteFor(null);
      toast.success(t("office.reconciliation.saved"));
    },
    onError: (e) =>
      toast.error(errText(e, t("office.reconciliation.save_failed"))),
  });
  const review = useMutation({
    mutationFn: (v: { uuid: string; note?: string }) =>
      reconciliationService.reviewCounterparty(
        slug,
        engagement,
        v.uuid,
        v.note,
      ),
    onSuccess: () => {
      refresh();
      setNoteFor(null);
      toast.success(t("office.reconciliation.sent_to_review"));
    },
    onError: (e) =>
      toast.error(errText(e, t("office.reconciliation.save_failed"))),
  });

  const issue = (r: ReconciliationCounterparty) =>
    r.result !== "matched" ||
    [
      "missing_in_form",
      "extra_in_form",
      "amount_mismatch",
      "count_mismatch",
    ].some((x) => x === r.ba_result || x === r.bs_result);
  const rows = useMemo(() => {
    const items = list.data?.items ?? [];
    return onlyIssues
      ? items.filter(
          (r) => issue(r) && r.status !== "resolved" && r.status !== "ignored",
        )
      : items;
  }, [list.data, onlyIssues]);

  const columns = useMemo<ColumnDef<ReconciliationCounterparty, unknown>[]>(
    () => [
      createColumn<ReconciliationCounterparty>({
        id: "counterparty",
        labelKey: "office.reconciliation.col_counterparty",
        accessorFn: (x) => x.counterparty_name,
        gridPrimary: true,
        meta: { cellClassName: "min-w-48" },
        cell: ({ row }) => (
          <div>
            <div className="max-w-xs truncate">
              {row.original.counterparty_name || "—"}
            </div>
            <div className="text-text-3 text-xs">
              {row.original.counterparty_tax_id}
              {row.original.account_code
                ? ` · ${row.original.account_code}`
                : ""}
            </div>
          </div>
        ),
      }),
      createMoneyColumn<ReconciliationCounterparty>({
        id: "ledger_balance",
        labelKey: "office.reconciliation.col_ledger_balance",
        accessor: (x) => x.ledger_balance,
      }),
      createMoneyColumn<ReconciliationCounterparty>({
        id: "source_balance",
        labelKey: "office.reconciliation.col_source_balance",
        accessor: (x) => x.source_balance,
      }),
      createMoneyColumn<ReconciliationCounterparty>({
        id: "difference",
        labelKey: "office.reconciliation.col_difference",
        accessor: (x) => x.difference,
        className: "font-semibold",
      }),
      createColumn<ReconciliationCounterparty>({
        id: "result",
        labelKey: "office.reconciliation.col_result",
        accessorFn: (x) => x.result,
        cell: ({ row }) => (
          <div className="flex flex-col items-start gap-1">
            <Pill
              group="result"
              value={row.original.result}
              tones={resultTone}
            />
            <Pill
              group="status"
              value={row.original.status}
              tones={decisionTone}
            />
          </div>
        ),
      }),
      createColumn<ReconciliationCounterparty>({
        id: "babs",
        labelKey: "office.reconciliation.col_babs",
        accessorFn: (x) => `${x.ba_result} ${x.bs_result}`,
        cell: ({ row }) => {
          const x = row.original;
          return (
            <div className="flex flex-col gap-1.5">
              <BabsCell
                form="BA"
                amount={x.ba_amount}
                count={x.ba_count}
                declared={x.declared_ba_amount}
                declaredCount={x.declared_ba_count}
                result={x.ba_result}
              />
              <BabsCell
                form="BS"
                amount={x.bs_amount}
                count={x.bs_count}
                declared={x.declared_bs_amount}
                declaredCount={x.declared_bs_count}
                result={x.bs_result}
              />
            </div>
          );
        },
      }),
      createColumn<ReconciliationCounterparty>({
        id: "letter",
        labelKey: "office.reconciliation.col_letter",
        accessorFn: (x) => x.last_letter?.status ?? "",
        cell: ({ row }) => {
          const l = row.original.last_letter;
          if (!l) return <span className="text-text-3">—</span>;
          return l.response ? (
            <Pill
              group="response"
              value={l.response}
              tones={{ agreed: "ok", disputed: "danger" }}
            />
          ) : (
            <Pill group="letter_status" value={l.status} tones={letterTone} />
          );
        },
      }),
      createActionsColumn<ReconciliationCounterparty>({
        cell: (x) =>
          canWrite ? (
            <div className="flex justify-end gap-1.5">
              <Button
                size="sm"
                variant="outline"
                className="max-md:min-h-11"
                onClick={() => setLetterFor(x)}
              >
                <Mail className="size-4" aria-hidden />
                {t("office.reconciliation.letter")}
              </Button>
              <DropdownMenu>
                <DropdownMenuTrigger asChild>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="max-md:min-h-11"
                    aria-label={t("office.reconciliation.decide_label", {
                      code: x.counterparty_name || x.counterparty_tax_id,
                    })}
                    disabled={x.status === "in_review"}
                  >
                    <MoreHorizontal className="size-4" aria-hidden />
                  </Button>
                </DropdownMenuTrigger>
                <DropdownMenuContent align="end">
                  <DropdownMenuItem
                    onSelect={() => setNoteFor({ row: x, mode: "explain" })}
                  >
                    {t("office.reconciliation.explain")}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onSelect={() =>
                      decide.mutate({ uuid: x.uuid, status: "resolved" })
                    }
                  >
                    {t("office.reconciliation.resolve")}
                  </DropdownMenuItem>
                  <DropdownMenuItem
                    onSelect={() =>
                      decide.mutate({ uuid: x.uuid, status: "ignored" })
                    }
                  >
                    {t("office.reconciliation.ignore")}
                  </DropdownMenuItem>
                  {issue(x) ? (
                    <DropdownMenuItem
                      onSelect={() => setNoteFor({ row: x, mode: "review" })}
                    >
                      {t("office.reconciliation.send_review")}
                    </DropdownMenuItem>
                  ) : null}
                </DropdownMenuContent>
              </DropdownMenu>
            </div>
          ) : null,
      }),
    ],
    [t, canWrite, decide],
  );

  const sum = list.data?.run.summary;
  if (list.isLoading) return <Skeleton className="h-64 w-full" />;
  return (
    <div className="space-y-4">
      <SummaryCards
        cards={[
          {
            label: t("office.reconciliation.counterparties"),
            value: sum?.counterparties ?? "–",
            icon: Users,
            box: "bg-surface-3 text-text-2",
          },
          {
            label: t("office.reconciliation.counterparty_differences"),
            value: sum?.counterparty_differences ?? "–",
            icon: AlertTriangle,
            box: "bg-warn-tint text-warn",
          },
          {
            label: t("office.reconciliation.babs_issues"),
            value: sum?.babs_issues ?? "–",
            icon: FileText,
            box: "bg-danger-tint text-danger",
          },
        ]}
      />
      <p className="text-text-2 text-sm">
        {t("office.reconciliation.counterparty_hint")}
      </p>
      <label className="flex w-fit items-center gap-2 text-sm">
        <input
          type="checkbox"
          className="size-4"
          checked={onlyIssues}
          onChange={(e) => setOnlyIssues(e.target.checked)}
        />
        {t("office.reconciliation.only_issues")}
      </label>
      <EntityTable<ReconciliationCounterparty>
        columns={columns}
        data={rows}
        getRowId={(x) => x.uuid}
        isError={list.isError}
        errorTitle={t("office.api.load_error")}
        errorDescription={list.error?.message}
        onRetry={() => void list.refetch()}
        emptyTitle={t("office.reconciliation.counterparties_empty_title")}
        emptyDescription={t("office.reconciliation.counterparties_empty")}
        features={{
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          facetedFilters: false,
          viewMode: false,
          pagination: false,
          persistKey: "office.reconciliation.counterparties",
        }}
      />
      <LetterDialog
        engagement={engagement}
        row={letterFor}
        onClose={() => setLetterFor(null)}
        onCreated={() => {
          refresh();
          void qc.invalidateQueries({
            queryKey: reconciliationKeys.letters(slug, engagement),
          });
        }}
      />
      <NoteDialog
        open={noteFor !== null}
        onOpenChange={(o) => !o && setNoteFor(null)}
        title={
          noteFor?.mode === "review"
            ? t("office.reconciliation.send_review_title", {
                code: noteFor.row.counterparty_name,
              })
            : t("office.reconciliation.explain_title", {
                code: noteFor?.row.counterparty_name ?? "",
              })
        }
        description={
          noteFor?.mode === "review"
            ? t("office.reconciliation.send_review_hint")
            : undefined
        }
        initial={noteFor?.row.note ?? ""}
        required={noteFor?.mode === "explain"}
        pending={decide.isPending || review.isPending}
        confirmLabel={
          noteFor?.mode === "review"
            ? t("office.reconciliation.send_review")
            : t("office.reconciliation.save")
        }
        onConfirm={(note) => {
          if (!noteFor) return;
          if (noteFor.mode === "review")
            review.mutate({ uuid: noteFor.row.uuid, note: note || undefined });
          else
            decide.mutate({
              uuid: noteFor.row.uuid,
              status: "explained",
              note,
            });
        }}
      />
    </div>
  );
}

function LetterDialog({
  engagement,
  row,
  onClose,
  onCreated,
}: {
  engagement: string;
  row: ReconciliationCounterparty | null;
  onClose: () => void;
  onCreated: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [email, setEmail] = useState("");
  const [note, setNote] = useState("");
  const [send, setSend] = useState(true);
  const [last, setLast] = useState<string | null>(null);
  if (row && last !== row.uuid) {
    setLast(row.uuid);
    setEmail(row.email ?? "");
    setNote("");
    setSend(Boolean(row.email));
  }
  const create = useMutation({
    mutationFn: () =>
      reconciliationService.createLetter(slug, engagement, {
        counterparty_uuid: row!.uuid,
        email: email.trim() || null,
        note: note.trim() || null,
        send,
      }),
    onSuccess: (l) => {
      onCreated();
      onClose();
      toast.success(
        l.status === "queued"
          ? t("office.reconciliation.letter_queued")
          : t("office.reconciliation.letter_created"),
      );
    },
    onError: (e) =>
      toast.error(errText(e, t("office.reconciliation.save_failed"))),
  });
  const balance = row?.ledger_balance ?? row?.source_balance ?? "0";
  return (
    <Dialog open={row !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>{t("office.reconciliation.letter_title")}</DialogTitle>
          <DialogDescription>
            {row?.counterparty_name} ·{" "}
            {t("office.reconciliation.letter_balance")}{" "}
            <Money value={balance} />
          </DialogDescription>
        </DialogHeader>
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="font-medium">
            {t("office.reconciliation.letter_email")}
          </span>
          <Input
            type="email"
            value={email}
            autoComplete="email"
            onChange={(e) => setEmail(e.target.value)}
          />
        </label>
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="font-medium">{t("office.reconciliation.note")}</span>
          <Textarea
            rows={3}
            maxLength={2000}
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </label>
        <label className="flex items-center gap-2 text-sm">
          <input
            type="checkbox"
            className="size-4"
            checked={send}
            onChange={(e) => setSend(e.target.checked)}
          />
          {t("office.reconciliation.letter_send_now")}
        </label>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t("office.reconciliation.cancel")}
          </Button>
          <Button
            disabled={create.isPending || (send && email.trim() === "")}
            onClick={() => create.mutate()}
          >
            <Send className="size-4" aria-hidden />
            {send
              ? t("office.reconciliation.letter_create_send")
              : t("office.reconciliation.letter_create")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

export function BabsView({
  engagement,
  period,
  canWrite,
}: {
  engagement: string;
  period: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const key = reconciliationKeys.babs(slug, engagement, period);
  const list = useQuery({
    queryKey: key,
    queryFn: () => reconciliationService.declarations(slug, engagement, period),
  });
  const refresh = () => void qc.invalidateQueries({ queryKey: key });
  const fileRef = useRef<HTMLInputElement>(null);
  const [form, setForm] = useState<"ba" | "bs">("ba");
  const [result, setResult] = useState<BabsImportResult | null>(null);
  const [adding, setAdding] = useState(false);
  const [draft, setDraft] = useState({
    tax: "",
    name: "",
    count: "1",
    amount: "",
  });

  const importer = useMutation({
    mutationFn: (file: File) =>
      reconciliationService.importDeclarations(
        slug,
        engagement,
        file,
        period,
        form,
      ),
    onSuccess: (r) => {
      setResult(r);
      refresh();
    },
    onError: (e) =>
      toast.error(errText(e, t("office.reconciliation.import_failed"))),
  });
  const save = useMutation({
    mutationFn: () =>
      reconciliationService.saveDeclaration(slug, engagement, {
        period,
        form_type: form,
        counterparty_tax_id: draft.tax.trim(),
        counterparty_name: draft.name.trim(),
        document_count: Number(draft.count) || 0,
        amount: draft.amount.replace(/\./g, "").replace(",", "."),
      }),
    onSuccess: () => {
      refresh();
      setDraft({ tax: "", name: "", count: "1", amount: "" });
      setAdding(false);
      toast.success(t("office.reconciliation.saved"));
    },
    onError: (e) =>
      toast.error(errText(e, t("office.reconciliation.save_failed"))),
  });
  const remove = useMutation({
    mutationFn: (uuid: string) =>
      reconciliationService.deleteDeclaration(slug, engagement, uuid),
    onSuccess: refresh,
  });

  const columns = useMemo<ColumnDef<BabsDeclaration, unknown>[]>(
    () => [
      createColumn<BabsDeclaration>({
        id: "form_type",
        labelKey: "office.reconciliation.col_form",
        accessorFn: (x) => x.form_type,
        cell: ({ row }) => (
          <span className="font-semibold uppercase">
            {row.original.form_type}
          </span>
        ),
      }),
      createColumn<BabsDeclaration>({
        id: "counterparty",
        labelKey: "office.reconciliation.col_counterparty",
        accessorFn: (x) => x.counterparty_name,
        gridPrimary: true,
        cell: ({ row }) => (
          <div>
            <div>{row.original.counterparty_name || "—"}</div>
            <div className="text-text-3 text-xs">
              {row.original.counterparty_tax_id}
            </div>
          </div>
        ),
      }),
      createColumn<BabsDeclaration>({
        id: "document_count",
        labelKey: "office.reconciliation.col_documents",
        accessorFn: (x) => x.document_count,
        meta: { align: "end" },
      }),
      createMoneyColumn<BabsDeclaration>({
        id: "amount",
        labelKey: "office.reconciliation.col_amount",
        accessor: (x) => x.amount,
      }),
      createColumn<BabsDeclaration>({
        id: "source",
        labelKey: "office.reconciliation.col_origin",
        accessorFn: (x) => x.source,
        cell: ({ row }) => (
          <span className="text-text-2 text-xs">
            {t(`office.reconciliation.origin_${row.original.source}`)}
          </span>
        ),
      }),
      createActionsColumn<BabsDeclaration>({
        cell: (x) =>
          canWrite ? (
            <Button
              size="sm"
              variant="ghost"
              className="max-md:min-h-11"
              aria-label={t("office.reconciliation.delete_line", {
                name: x.counterparty_name || x.counterparty_tax_id,
              })}
              disabled={remove.isPending}
              onClick={() => remove.mutate(x.uuid)}
            >
              <Trash2 className="size-4" aria-hidden />
            </Button>
          ) : null,
      }),
    ],
    [t, canWrite, remove],
  );

  return (
    <div className="space-y-4">
      <p className="text-text-2 text-sm">
        {t("office.reconciliation.babs_hint", { period })}
      </p>
      {canWrite ? (
        <div className="flex flex-wrap items-center gap-2">
          <label className="flex items-center gap-2 text-sm">
            <span className="text-text-2">
              {t("office.reconciliation.col_form")}
            </span>
            <select
              className={selectClass}
              value={form}
              onChange={(e) => setForm(e.target.value as "ba" | "bs")}
            >
              <option value="ba">{t("office.reconciliation.form_ba")}</option>
              <option value="bs">{t("office.reconciliation.form_bs")}</option>
            </select>
          </label>
          <Button
            size="sm"
            variant="outline"
            className="max-md:min-h-11"
            disabled={importer.isPending}
            onClick={() => fileRef.current?.click()}
          >
            <Upload className="size-4" aria-hidden />
            {t("office.reconciliation.babs_import")}
          </Button>
          <input
            ref={fileRef}
            type="file"
            accept=".xlsx,.csv,.txt"
            className="hidden"
            aria-hidden
            tabIndex={-1}
            onChange={(e) => {
              const f = e.target.files?.[0];
              if (f) importer.mutate(f);
              e.target.value = "";
            }}
          />
          <Button
            size="sm"
            variant="outline"
            className="max-md:min-h-11"
            onClick={() => setAdding((a) => !a)}
          >
            <Plus className="size-4" aria-hidden />
            {t("office.reconciliation.babs_add")}
          </Button>
        </div>
      ) : null}
      {result ? (
        <div className="border-line bg-surface rounded-xl border p-3 text-sm">
          {t("office.reconciliation.babs_imported", { count: result.imported })}
          {result.errors.length > 0 ? (
            <ul className="text-danger mt-1 list-disc ps-5">
              {result.errors.slice(0, 10).map((e) => (
                <li key={e.line}>
                  {t("office.reconciliation.line", { line: e.line })}:{" "}
                  {e.message}
                </li>
              ))}
            </ul>
          ) : null}
        </div>
      ) : null}
      {adding ? (
        <form
          className="border-line bg-surface grid gap-2 rounded-xl border p-3 sm:grid-cols-5"
          onSubmit={(e) => {
            e.preventDefault();
            save.mutate();
          }}
        >
          <Input
            aria-label={t("office.reconciliation.tax_id")}
            placeholder={t("office.reconciliation.tax_id")}
            inputMode="numeric"
            value={draft.tax}
            onChange={(e) => setDraft({ ...draft, tax: e.target.value })}
          />
          <Input
            aria-label={t("office.reconciliation.col_counterparty")}
            placeholder={t("office.reconciliation.col_counterparty")}
            value={draft.name}
            onChange={(e) => setDraft({ ...draft, name: e.target.value })}
          />
          <Input
            aria-label={t("office.reconciliation.col_documents")}
            inputMode="numeric"
            value={draft.count}
            onChange={(e) => setDraft({ ...draft, count: e.target.value })}
          />
          <Input
            aria-label={t("office.reconciliation.col_amount")}
            placeholder="12.500"
            inputMode="decimal"
            value={draft.amount}
            onChange={(e) => setDraft({ ...draft, amount: e.target.value })}
          />
          <Button type="submit" disabled={save.isPending}>
            {t("office.reconciliation.save")}
          </Button>
        </form>
      ) : null}
      <EntityTable<BabsDeclaration>
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(x) => x.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("office.api.load_error")}
        onRetry={() => void list.refetch()}
        emptyTitle={t("office.reconciliation.babs_empty_title")}
        emptyDescription={t("office.reconciliation.babs_empty")}
        features={{
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          facetedFilters: false,
          viewMode: false,
          pagination: false,
          persistKey: "office.reconciliation.babs",
        }}
      />
    </div>
  );
}

export function LettersView({
  engagement,
  canWrite,
}: {
  engagement: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const key = reconciliationKeys.letters(slug, engagement);
  const list = useQuery({
    queryKey: key,
    queryFn: () => reconciliationService.letters(slug, engagement),
  });
  const refresh = () =>
    void qc.invalidateQueries({
      queryKey: reconciliationKeys.all(slug, engagement),
    });
  const [answering, setAnswering] = useState<ReconciliationLetter | null>(null);
  const send = useMutation({
    mutationFn: (uuid: string) =>
      reconciliationService.sendLetter(slug, engagement, uuid),
    onSuccess: () => {
      refresh();
      toast.success(t("office.reconciliation.letter_queued"));
    },
    onError: (e) =>
      toast.error(errText(e, t("office.reconciliation.save_failed"))),
  });

  const columns = useMemo<ColumnDef<ReconciliationLetter, unknown>[]>(
    () => [
      createColumn<ReconciliationLetter>({
        id: "counterparty",
        labelKey: "office.reconciliation.col_counterparty",
        accessorFn: (x) => x.counterparty_name,
        gridPrimary: true,
        cell: ({ row }) => (
          <div>
            <div>{row.original.counterparty_name || "—"}</div>
            <div className="text-text-3 text-xs">
              {row.original.recipient_email ?? row.original.counterparty_tax_id}
            </div>
          </div>
        ),
      }),
      createDateColumn<ReconciliationLetter>({
        id: "as_of_date",
        labelKey: "office.reconciliation.col_as_of",
        accessor: (x) => x.as_of_date,
      }),
      createMoneyColumn<ReconciliationLetter>({
        id: "balance",
        labelKey: "office.reconciliation.col_letter_balance",
        accessor: (x) => x.balance,
      }),
      createColumn<ReconciliationLetter>({
        id: "status",
        labelKey: "office.reconciliation.col_status",
        accessorFn: (x) => x.status,
        cell: ({ row }) => {
          const l = row.original;
          return (
            <div className="flex flex-col items-start gap-1">
              <Pill group="letter_status" value={l.status} tones={letterTone} />
              {l.response ? (
                <Pill
                  group="response"
                  value={l.response}
                  tones={{ agreed: "ok", disputed: "danger" }}
                />
              ) : null}
              {l.last_error && l.status !== "sent" ? (
                <span className="text-danger max-w-xs truncate text-xs">
                  {l.last_error}
                </span>
              ) : null}
            </div>
          );
        },
      }),
      createColumn<ReconciliationLetter>({
        id: "response",
        labelKey: "office.reconciliation.col_response",
        accessorFn: (x) => x.response_note ?? "",
        meta: { cellClassName: "max-w-xs" },
        cell: ({ row }) => {
          const l = row.original;
          if (!l.response) return <span className="text-text-3">—</span>;
          return (
            <div className="text-sm">
              {l.response_balance ? (
                <div>
                  {t("office.reconciliation.their_balance")}{" "}
                  <Money value={l.response_balance} />
                </div>
              ) : null}
              {l.response_note ? (
                <div className="text-text-2">{l.response_note}</div>
              ) : null}
              <div className="text-text-3 text-xs">
                {[
                  l.responder_name,
                  l.response_channel &&
                    t(`office.reconciliation.channel_${l.response_channel}`),
                ]
                  .filter(Boolean)
                  .join(" · ")}
              </div>
            </div>
          );
        },
      }),
      createActionsColumn<ReconciliationLetter>({
        cell: (x) => (
          <div className="flex justify-end gap-1.5">
            <Button
              size="sm"
              variant="outline"
              className="max-md:min-h-11"
              onClick={() =>
                void reconciliationService
                  .openLetterPdf(slug, engagement, x.uuid)
                  .catch(() =>
                    toast.error(t("office.reconciliation.pdf_failed")),
                  )
              }
            >
              <FileText className="size-4" aria-hidden />
              PDF
            </Button>
            {canWrite && x.status !== "responded" ? (
              <>
                {x.recipient_email ? (
                  <Button
                    size="sm"
                    variant="ghost"
                    className="max-md:min-h-11"
                    disabled={send.isPending}
                    onClick={() => send.mutate(x.uuid)}
                  >
                    <Send className="size-4" aria-hidden />
                    {x.status === "draft"
                      ? t("office.reconciliation.send")
                      : t("office.reconciliation.resend")}
                  </Button>
                ) : null}
                <Button
                  size="sm"
                  variant="ghost"
                  className="max-md:min-h-11"
                  onClick={() => setAnswering(x)}
                >
                  {t("office.reconciliation.record_response")}
                </Button>
              </>
            ) : null}
          </div>
        ),
      }),
    ],
    [t, slug, engagement, canWrite, send],
  );

  return (
    <div className="space-y-4">
      <p className="text-text-2 text-sm">
        {t("office.reconciliation.letters_hint")}
      </p>
      <EntityTable<ReconciliationLetter>
        columns={columns}
        data={list.data?.items ?? []}
        getRowId={(x) => x.uuid}
        isLoading={list.isLoading}
        isError={list.isError}
        errorTitle={t("office.api.load_error")}
        onRetry={() => void list.refetch()}
        emptyTitle={t("office.reconciliation.letters_empty_title")}
        emptyDescription={t("office.reconciliation.letters_empty")}
        features={{
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          facetedFilters: false,
          viewMode: false,
          pagination: false,
          persistKey: "office.reconciliation.letters",
        }}
      />
      <ResponseDialog
        engagement={engagement}
        letter={answering}
        onClose={() => setAnswering(null)}
        onSaved={refresh}
      />
    </div>
  );
}

function ResponseDialog({
  engagement,
  letter,
  onClose,
  onSaved,
}: {
  engagement: string;
  letter: ReconciliationLetter | null;
  onClose: () => void;
  onSaved: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [response, setResponse] = useState<"agreed" | "disputed">("agreed");
  const [balance, setBalance] = useState("");
  const [note, setNote] = useState("");
  const [name, setName] = useState("");
  const save = useMutation({
    mutationFn: () =>
      reconciliationService.recordResponse(slug, engagement, letter!.uuid, {
        response,
        balance: balance.trim()
          ? balance.replace(/\./g, "").replace(",", ".")
          : null,
        note: note.trim() || null,
        responder_name: name.trim() || null,
      }),
    onSuccess: () => {
      onSaved();
      onClose();
      toast.success(t("office.reconciliation.saved"));
    },
    onError: (e) =>
      toast.error(errText(e, t("office.reconciliation.save_failed"))),
  });
  return (
    <Dialog open={letter !== null} onOpenChange={(o) => !o && onClose()}>
      <DialogContent className="sm:max-w-lg">
        <DialogHeader>
          <DialogTitle>
            {t("office.reconciliation.record_response")}
          </DialogTitle>
          <DialogDescription>{letter?.counterparty_name}</DialogDescription>
        </DialogHeader>
        <fieldset className="flex gap-4 text-sm">
          <legend className="sr-only">
            {t("office.reconciliation.col_response")}
          </legend>
          {(["agreed", "disputed"] as const).map((r) => (
            <label key={r} className="flex items-center gap-2">
              <input
                type="radio"
                name="response"
                checked={response === r}
                onChange={() => setResponse(r)}
              />
              {t(`office.reconciliation.response_${r}`)}
            </label>
          ))}
        </fieldset>
        {response === "disputed" ? (
          <label className="flex flex-col gap-1.5 text-sm">
            <span className="font-medium">
              {t("office.reconciliation.their_balance")}
            </span>
            <Input
              inputMode="decimal"
              value={balance}
              onChange={(e) => setBalance(e.target.value)}
            />
          </label>
        ) : null}
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="font-medium">
            {t("office.reconciliation.responder")}
          </span>
          <Input value={name} onChange={(e) => setName(e.target.value)} />
        </label>
        <label className="flex flex-col gap-1.5 text-sm">
          <span className="font-medium">{t("office.reconciliation.note")}</span>
          <Textarea
            rows={3}
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
        </label>
        <DialogFooter>
          <Button variant="ghost" onClick={onClose}>
            {t("office.reconciliation.cancel")}
          </Button>
          <Button disabled={save.isPending} onClick={() => save.mutate()}>
            {t("office.reconciliation.save")}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
