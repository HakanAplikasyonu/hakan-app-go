"use client";

import { Loader2, Plus, Scale, Trash2 } from "lucide-react";
import {
  createContext,
  useCallback,
  useContext,
  useDeferredValue,
  useEffect,
  useMemo,
  useRef,
  useState,
} from "react";
import type { ColumnDef } from "@tanstack/react-table";

import {
  createActionsColumn,
  createColumn,
  DataTable,
  type DataTableFeatures,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { useJournalActions } from "../hooks/use-journals";
import { ACCOUNT_CODE_RE, useAccountPlanAccounts } from "../hooks/use-review";
import {
  formatAmount,
  formatMoney,
  fromMinor,
  parseAmountInput,
  sub,
  sum,
  toMinor,
} from "../lib/decimal";
import { journalErrorText } from "../lib/journal-error";
import { Figure, Money, Panel } from "./office-ui";

type LineRow = {
  account: string;
  description: string;
  debit: string;
  credit: string;
};
const emptyLine = (): LineRow => ({
  account: "",
  description: "",
  debit: "",
  credit: "",
});

/** Satırdaki alanlar, Enter ile bu sırada ilerlenir. */
const FIELDS = ["account", "description", "debit", "credit"] as const;
type Field = (typeof FIELDS)[number];

const ACCOUNTS_LIST_ID = "journal-new-accounts";

// Fiş kalemi formu: araç çubuğu, sayfalama, seçim, sıralama yok (kalem
// sırası fişin sırasıdır). Mobilde kartlar açık; kart içindeki girdiler çalışır.
const lineEditorTable: DataTableFeatures = {
  toolbar: false,
  pagination: false,
  rowSelection: false,
  columnFilters: false,
  facetedFilters: false,
  globalFilter: false,
  viewMode: false,
  columnVisibility: false,
  columnResizing: false,
  columnPinning: false,
  columnOrdering: false,
  density: false,
  sorting: false,
};

/** Planda bulunan hesap; null = planda arandı, yok. */
type AccountInfo = { name: string; leaf: boolean } | null;

type LineEditor = {
  accounts: Record<string, AccountInfo>;
  /** Odaktaki hesap kutusunun satırı (durumu odak çıkınca gösterilir). */
  activeAccount: number | null;
  setActiveAccount: (row: number | null) => void;
  searchAccount: (q: string) => void;
  set: (row: number, field: Field, value: string) => void;
  onKeyDown: (
    e: React.KeyboardEvent<HTMLInputElement>,
    row: number,
    field: Field,
  ) => void;
};
const LineEditorContext = createContext<LineEditor | null>(null);
const useLineEditor = () => {
  const ctx = useContext(LineEditorContext);
  if (!ctx) throw new Error("LineEditorContext missing");
  return ctx;
};

const cellId = (row: number, field: Field) => `journal-new-${field}-${row}`;

/** Hesap kodu: plandan öneri (kod ya da ad), altında hesabın adı/durumu. */
function AccountCell({ row, value }: { row: number; value: string }) {
  const { t } = useLocale();
  const ed = useLineEditor();
  const code = value.trim();
  const info = code ? ed.accounts[code] : undefined;
  const showStatus = ed.activeAccount !== row && code !== "";
  const malformed = code !== "" && !ACCOUNT_CODE_RE.test(code);
  return (
    <div className="w-44 space-y-0.5">
      <Input
        id={cellId(row, "account")}
        className="h-8 font-mono"
        placeholder="770.01"
        list={ACCOUNTS_LIST_ID}
        autoComplete="off"
        aria-label={`${t("office.col_account")} ${row + 1}`}
        aria-invalid={
          showStatus && (malformed || info === null) ? true : undefined
        }
        value={value}
        onFocus={() => {
          ed.setActiveAccount(row);
          ed.searchAccount(value);
        }}
        onBlur={() => ed.setActiveAccount(null)}
        onChange={(e) => {
          ed.set(row, "account", e.target.value);
          ed.searchAccount(e.target.value);
        }}
        onKeyDown={(e) => ed.onKeyDown(e, row, "account")}
      />
      {showStatus ? (
        <p
          className={cn(
            "truncate text-xs",
            info ? "text-muted-foreground" : "text-status-warning",
            info && !info.leaf && "text-status-warning",
          )}
        >
          {malformed
            ? t("office.journals.entry.account_malformed")
            : info === null
              ? t("office.journals.entry.account_not_in_plan")
              : info
                ? info.leaf
                  ? info.name
                  : t("office.journals.entry.account_parent", {
                      name: info.name,
                    })
                : null}
        </p>
      ) : null}
    </div>
  );
}

/** Borç/alacak: Türkçe yazım ("1.250,50"), odak çıkınca biçimlenir. */
function AmountCell({
  row,
  field,
  value,
}: {
  row: number;
  field: "debit" | "credit";
  value: string;
}) {
  const { t } = useLocale();
  const ed = useLineEditor();
  const invalid = parseAmountInput(value) === null;
  return (
    <Input
      id={cellId(row, field)}
      className="ms-auto h-8 w-32 text-right font-mono"
      inputMode="decimal"
      autoComplete="off"
      aria-label={`${t(`office.journals.${field}`)} ${row + 1}`}
      aria-invalid={invalid || undefined}
      value={value}
      onChange={(e) => ed.set(row, field, e.target.value)}
      onBlur={() => {
        const parsed = parseAmountInput(value);
        if (parsed && toMinor(parsed) > BigInt(0)) {
          ed.set(row, field, formatAmount(parsed));
        }
      }}
      onKeyDown={(e) => ed.onKeyDown(e, row, field)}
    />
  );
}

/** Satırın tutarı API biçiminde ("" = boş, null = okunamadı). */
function lineAmounts(l: LineRow) {
  return {
    debit: parseAmountInput(l.debit),
    credit: parseAmountInput(l.credit),
  };
}

const positive = (v: string | null) => !!v && toMinor(v) > BigInt(0);

/**
 * Yeni mahsup fişi (#573). Hesap kodu plandan aranır (kod ya da ad) ve
 * altında hesabın adı görünür; planda olmayan ya da ana hesap uyarı verir.
 * Tutarlar Türkçe yazılır, borç yazılınca aynı satırın alacağı silinir.
 * Denge canlı; "Farkı kapat" farkı son satıra yazar. Enter sonraki alana,
 * son satırın sonunda yeni satıra geçer; Ctrl+Enter kaydeder.
 */
export function NewDraft({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { create } = useJournalActions(slug, engagement);
  const [date, setDate] = useState(() => new Date().toISOString().slice(0, 10));
  const [description, setDescription] = useState("");
  const [lines, setLines] = useState<LineRow[]>([emptyLine(), emptyLine()]);
  // Yeni satır çizildikten sonra odaklanacak alan.
  const focusNext = useRef<string | null>(null);

  // Plan araması: odaktaki hesap kutusunun metni. Bulunan hesaplar birikir;
  // tam kod arandığında bulunamazsa "planda yok" diye işaretlenir.
  const [accountQ, setAccountQ] = useState("");
  const search = useDeferredValue(accountQ.trim());
  const plan = useAccountPlanAccounts(engagement, search);
  const [accounts, setAccounts] = useState<Record<string, AccountInfo>>({});
  const [seen, setSeen] = useState<unknown>(null);
  if (plan.data && plan.data !== seen) {
    setSeen(plan.data);
    const found: Record<string, AccountInfo> = {};
    for (const a of plan.data.items) {
      found[a.code] = { name: a.name, leaf: a.is_leaf };
    }
    // Plan hiç yüklenmemişse (boş) "planda yok" denmez.
    const planKnown =
      plan.data.items.length > 0 || Object.values(accounts).some(Boolean);
    if (planKnown && ACCOUNT_CODE_RE.test(search) && !(search in found)) {
      found[search] = null;
    }
    setAccounts((prev) => ({ ...prev, ...found }));
  }
  const [activeAccount, setActiveAccount] = useState<number | null>(null);

  const parsed = lines.map(lineAmounts);
  const debit = sum(parsed.map((p) => p.debit || "0"));
  const credit = sum(parsed.map((p) => p.credit || "0"));
  const diff = sub(debit, credit);
  const balanced = toMinor(diff) === BigInt(0) && toMinor(debit) > BigInt(0);

  // Kaydetmeyi durduran ilk satır sorunu (sunucu da aynısını reddeder).
  const problem = (() => {
    for (const [i, l] of lines.entries()) {
      const { debit: d, credit: c } = parsed[i];
      if (d === null || c === null) {
        return t("office.journals.entry.amount_invalid", { line: i + 1 });
      }
      const has = l.account.trim() !== "";
      if (!has && !positive(d) && !positive(c)) continue;
      if (!has || positive(d) === positive(c)) {
        return t("office.journals.entry.line_incomplete", { line: i + 1 });
      }
    }
    return lines.some((l) => l.account.trim())
      ? null
      : t("office.journals.entry.no_lines");
  })();

  const set = useCallback((i: number, k: Field, v: string) => {
    setLines((ls) =>
      ls.map((l, j) => {
        if (j !== i) return l;
        const next = { ...l, [k]: v };
        // Bir satır ya borç ya alacaktır: biri yazılınca öbürü silinir.
        if (k === "debit" && v.trim()) next.credit = "";
        if (k === "credit" && v.trim()) next.debit = "";
        return next;
      }),
    );
  }, []);

  const linesRef = useRef(lines);
  useEffect(() => {
    linesRef.current = lines;
  }, [lines]);
  const submitRef = useRef<() => void>(() => {});

  const onKeyDown = useCallback(
    (e: React.KeyboardEvent<HTMLInputElement>, row: number, field: Field) => {
      if (e.key !== "Enter") return;
      e.preventDefault();
      if (e.ctrlKey || e.metaKey) {
        submitRef.current();
        return;
      }
      const ls = linesRef.current;
      // Borç yazıldıysa alacak atlanır; satır sonu bir sonraki satırın hesabı.
      let idx = FIELDS.indexOf(field) + 1;
      if (field === "debit" && ls[row]?.debit.trim()) idx = FIELDS.length;
      if (idx < FIELDS.length) {
        document.getElementById(cellId(row, FIELDS[idx]))?.focus();
        return;
      }
      if (row + 1 < ls.length) {
        document.getElementById(cellId(row + 1, "account"))?.focus();
        return;
      }
      setLines((cur) => [...cur, emptyLine()]);
      focusNext.current = cellId(row + 1, "account");
    },
    [],
  );
  useEffect(() => {
    const id = focusNext.current;
    if (!id) return;
    focusNext.current = null;
    document.getElementById(id)?.focus();
  }, [lines]);

  const editor = useMemo<LineEditor>(
    () => ({
      accounts,
      activeAccount,
      setActiveAccount,
      searchAccount: setAccountQ,
      set,
      onKeyDown,
    }),
    [accounts, activeAccount, set, onKeyDown],
  );

  // Kolonlar yalnızca t ve set'e bağlı: her tuşta yeniden oluşursa hücreler
  // yeniden bağlanır ve girdi odağı kaybolur. Değişen veri bağlamdan okunur.
  const lineColumns = useMemo<ColumnDef<LineRow, unknown>[]>(
    () => [
      createColumn<LineRow>({
        id: "account",
        labelKey: "office.col_account",
        gridPrimary: true,
        cell: ({ row }) => (
          <AccountCell row={row.index} value={row.original.account} />
        ),
      }),
      createColumn<LineRow>({
        id: "description",
        labelKey: "office.journals.line_description",
        cell: ({ row }) => (
          <DescriptionCell row={row.index} value={row.original.description} />
        ),
      }),
      createColumn<LineRow>({
        id: "debit",
        labelKey: "office.journals.debit",
        meta: { align: "end" },
        cell: ({ row }) => (
          <AmountCell
            row={row.index}
            field="debit"
            value={row.original.debit}
          />
        ),
      }),
      createColumn<LineRow>({
        id: "credit",
        labelKey: "office.journals.credit",
        meta: { align: "end" },
        cell: ({ row }) => (
          <AmountCell
            row={row.index}
            field="credit"
            value={row.original.credit}
          />
        ),
      }),
      // Silme düğmesi satır sırasına ve kalem sayısına bakar; bunları hücre
      // bağlamından okumak için yardımcının cell'i geçersiz kılınır.
      {
        ...createActionsColumn<LineRow>({
          labelKey: "office.journals.remove_line",
          cell: () => null,
        }),
        cell: ({ row, table }) => (
          <Button
            variant="ghost"
            size="sm"
            className="max-md:min-w-11"
            aria-label={t("office.journals.remove_line")}
            disabled={table.options.data.length <= 2}
            onClick={() =>
              setLines((ls) => ls.filter((_, j) => j !== row.index))
            }
          >
            <Trash2 className="size-4" />
          </Button>
        ),
      },
    ],
    [t],
  );

  /** Farkı son boş satıra (yoksa yeni satıra) eksik tarafa yazar. */
  const closeDifference = () => {
    const d = toMinor(diff);
    if (d === BigInt(0)) return;
    const side: "debit" | "credit" = d > BigInt(0) ? "credit" : "debit";
    const amount = formatAmount(fromMinor(d > BigInt(0) ? d : -d));
    const at = lines.findLastIndex((l) => !l.debit.trim() && !l.credit.trim());
    const i = at >= 0 ? at : lines.length;
    const base = at >= 0 ? lines : [...lines, emptyLine()];
    setLines(
      base.map((l, j) =>
        j === i ? { ...l, debit: "", credit: "", [side]: amount } : l,
      ),
    );
    focusNext.current = cellId(i, base[i].account.trim() ? side : "account");
  };

  const submit = () => {
    if (problem || create.isPending) return;
    create.mutate(
      {
        entry_type: "mahsup",
        entry_date: `${date}T00:00:00Z`,
        description,
        lines: lines
          .map((l, i) => ({ l, p: parsed[i] }))
          .filter(({ l }) => l.account.trim())
          .map(({ l, p }) => ({
            account: l.account.trim(),
            description: l.description,
            debit: p.debit || "0",
            credit: p.credit || "0",
          })),
      },
      {
        onSuccess: () => {
          setDescription("");
          setLines([emptyLine(), emptyLine()]);
        },
      },
    );
  };
  useEffect(() => {
    submitRef.current = submit;
  });

  const options = plan.data?.items ?? [];

  return (
    <Panel title={t("office.journals.new_title")}>
      <div className="space-y-3 p-4">
        <div className="flex flex-wrap gap-3">
          <div className="flex items-center gap-2 text-xs">
            <Label
              htmlFor="journal-new-date"
              className="text-muted-foreground text-xs font-normal"
            >
              {t("office.col_date")}
            </Label>
            <DatePicker
              id="journal-new-date"
              className="h-8 w-40"
              value={date}
              onChange={(v) => setDate(v)}
            />
          </div>
          <Input
            className="h-8 max-w-md flex-1"
            placeholder={t("office.journals.description")}
            aria-label={t("office.journals.description")}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>
        <datalist id={ACCOUNTS_LIST_ID}>
          {options.map((a) => (
            <option key={a.code} value={a.code}>
              {a.name}
            </option>
          ))}
        </datalist>
        <LineEditorContext.Provider value={editor}>
          <DataTable<LineRow>
            columns={lineColumns}
            data={lines}
            getRowId={(_, i) => String(i)}
            features={lineEditorTable}
          />
        </LineEditorContext.Provider>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Button
            variant="outline"
            size="sm"
            onClick={() => {
              setLines((ls) => [...ls, emptyLine()]);
              focusNext.current = cellId(lines.length, "account");
            }}
          >
            <Plus className="size-4" />
            {t("office.journals.add_line")}
          </Button>
          <div className="flex flex-wrap items-center gap-3 text-xs">
            <span
              role="status"
              className={cn(
                balanced ? "text-status-approved" : "text-status-warning",
              )}
            >
              {balanced
                ? t("office.journals.balanced")
                : t("office.journals.difference", {
                    amount: formatMoney(diff, "TRY"),
                  })}
            </span>
            {!balanced && toMinor(diff) !== BigInt(0) ? (
              <Button variant="outline" size="sm" onClick={closeDifference}>
                <Scale className="size-4" />
                {t("office.journals.entry.close_difference")}
              </Button>
            ) : null}
            <Figure>
              <Money value={debit} /> / <Money value={credit} />
            </Figure>
            <Button
              size="sm"
              onClick={submit}
              disabled={create.isPending || !!problem}
            >
              {create.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Plus className="size-4" />
              )}
              {t("office.journals.create")}
            </Button>
          </div>
        </div>
        {problem &&
        lines.some((l) => l.account.trim() || l.debit || l.credit) ? (
          <p className="text-status-warning text-xs">{problem}</p>
        ) : null}
        {create.isError ? (
          <p className="text-status-blocking text-sm">
            {journalErrorText(t, create.error, t("office.api.load_error"))}
          </p>
        ) : null}
        <p className="text-muted-foreground text-xs">
          {t("office.journals.new_hint")}{" "}
          {t("office.journals.entry.keyboard_hint")}
        </p>
      </div>
    </Panel>
  );
}

function DescriptionCell({ row, value }: { row: number; value: string }) {
  const { t } = useLocale();
  const ed = useLineEditor();
  return (
    <Input
      id={cellId(row, "description")}
      className="h-8"
      aria-label={`${t("office.journals.line_description")} ${row + 1}`}
      value={value}
      onChange={(e) => ed.set(row, "description", e.target.value)}
      onKeyDown={(e) => ed.onKeyDown(e, row, "description")}
    />
  );
}
