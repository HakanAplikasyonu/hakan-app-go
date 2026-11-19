"use client";

import {
  AlertCircle,
  Check,
  Circle,
  Keyboard,
  Loader2,
  Redo2,
  Save,
  Undo2,
} from "lucide-react";
import {
  useCallback,
  useEffect,
  useId,
  useMemo,
  useRef,
  useState,
  type ClipboardEvent as ReactClipboardEvent,
  type FocusEvent as ReactFocusEvent,
  type KeyboardEvent as ReactKeyboardEvent,
  type MouseEvent as ReactMouseEvent,
  type ReactNode,
} from "react";

import {
  GridCellEditor,
  type GridCommitNav,
} from "@/components/tables/editable-cell";
import {
  displayText,
  defaultParse,
  defaultToText,
  isColumnEditable,
  validateCell,
  type EditableGridColumn,
  type GridMessages,
} from "@/components/tables/grid-columns";
import {
  applyChanges,
  clampCoord,
  clearChanges,
  copyMatrix,
  EMPTY_HISTORY,
  fillChanges,
  fillDownChanges,
  fillRightChanges,
  fillTarget,
  forgetRow,
  isNavKey,
  isPrintableKey,
  moveCoord,
  normalizeRange,
  parseTsv,
  pasteChanges,
  pushHistory,
  rangeContains,
  rangeSize,
  redoHistory,
  sameValue,
  singleRange,
  toTsv,
  undoHistory,
  type CellChange,
  type CellCoord,
  type CellRange,
  type GridColumnModel,
  type GridDrafts,
  type GridHistory,
  type GridModelInput,
  type NormalizedRange,
} from "@/components/tables/grid-model";
import { Hint } from "@/components/ui/hint";
import type { ColumnAlign } from "@/components/tables/types";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
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

export type GridRowSaveInput<TData> = {
  rowId: string;
  row: TData;
  /** Yalnızca değişen kolonlar → yeni değer. */
  changes: Record<string, unknown>;
  /** Satırın kolondaki güncel (taslak dahil) değeri. */
  value: (columnId: string) => unknown;
};

export type EditableGridProps<TData> = {
  columns: EditableGridColumn<TData>[];
  /**
   * Satırlar. Kimliği değişen dizi, kaydedilmiş-ama-yenilenmemiş değerleri
   * sıfırlar; üst bileşen diziyi `useMemo` ile sabitlemeli.
   */
  rows: TData[];
  getRowId: (row: TData) => string;
  /** Izgaranın erişilebilir adı. */
  label: string;
  /**
   * Satır kaydı. Hata fırlatırsa satır taslağı korunur ve hata satırda
   * gösterilir. Verilmezse ızgara yalnızca taslak tutar.
   */
  onSaveRow?: (input: GridRowSaveInput<TData>) => Promise<void> | void;
  /**
   * `row-leave`: etkin satırdan çıkınca (başka satıra geçiş ya da ızgaradan
   * odak çıkışı) satır geçerliyse kaydedilir. `manual`: yalnızca Ctrl+S ve
   * kaydet düğmeleri. Default: "manual"
   */
  saveMode?: "manual" | "row-leave";
  /** Kaydedilmemiş satır sayısı değişince. */
  onDirtyChange?: (count: number) => void;
  /** Satır başlığında (durum ikonunun yanında) gösterilecek etiket. */
  rowLabel?: (row: TData, index: number) => ReactNode;
  isLoading?: boolean;
  emptyTitle?: string;
  emptyDescription?: string;
  /** Araç çubuğunun solu (süzgeçler vb.). */
  toolbar?: ReactNode;
  maxHeight?: number | string;
  className?: string;
};

type RowStatus =
  | { state: "saving" }
  | { state: "saved" }
  | { state: "error"; message: string };

type Editing = { row: number; col: number; text: string; seeded: boolean };

const alignClass: Record<ColumnAlign, string> = {
  start: "text-start",
  center: "text-center",
  end: "text-end",
};

const PAGE_SIZE = 10;

const EMPTY_DRAFTS: GridDrafts = {};

/**
 * Excel benzeri düzenlenebilir ızgara (#394): hücre içi düzenleme, klavye
 * gezintisi (ok / Tab / Enter / Home / End / PageUp / PageDown), fare ve
 * Shift ile çoklu seçim, Excel'den TSV yapıştırma ve kopyalama, doldurma
 * (Ctrl+D, Ctrl+R, doldurma tutamacı), geri al / yinele, doğrulama ve
 * satır bazında kaydetme. WAI-ARIA grid deseni: gezilen hücrede tek
 * sekme durağı (roving tabindex).
 */
export function EditableGrid<TData>({
  columns,
  rows,
  getRowId,
  label,
  onSaveRow,
  saveMode = "manual",
  onDirtyChange,
  rowLabel,
  isLoading,
  emptyTitle,
  emptyDescription,
  toolbar,
  maxHeight,
  className,
}: EditableGridProps<TData>) {
  const { t, locale } = useLocale();
  const gridId = useId();

  const [selection, setSelection] = useState<CellRange>(
    singleRange({ row: 0, col: 0 }),
  );
  const [editing, setEditing] = useState<Editing | null>(null);
  const [drafts, setDrafts] = useState<GridDrafts>({});
  /**
   * Kaydedilmiş, üst bileşen yeni `rows` verene kadar gösterilen değerler.
   * Hangi `rows` dizisi için tutulduğu saklanır; dizi değişince düşer.
   */
  const [committedState, setCommittedState] = useState<{
    rows: TData[];
    values: GridDrafts;
  }>({ rows, values: {} });
  const committed =
    committedState.rows === rows ? committedState.values : EMPTY_DRAFTS;
  const [history, setHistory] = useState<GridHistory>(EMPTY_HISTORY);
  const [status, setStatus] = useState<Record<string, RowStatus>>({});
  const [announcement, setAnnouncement] = useState("");
  /** Doldurma tutamacı sürüklemesi: kaynak seçim ve üzerinde olunan hücre. */
  const [fill, setFill] = useState<{
    source: NormalizedRange;
    over: CellCoord | null;
  } | null>(null);

  const tableRef = useRef<HTMLTableElement>(null);
  const cellRefs = useRef(new Map<string, HTMLTableCellElement>());
  const focusPending = useRef(false);
  const selecting = useRef(false);
  const saving = useRef(new Set<string>());

  const messages: GridMessages = useMemo(
    () => ({
      required: t("table.grid_required"),
      invalidNumber: t("table.grid_invalid_number"),
      invalidOption: t("table.grid_invalid_option"),
    }),
    [t],
  );

  const rowIds = useMemo(() => rows.map(getRowId), [rows, getRowId]);
  const rowById = useMemo(() => {
    const m = new Map<string, TData>();
    rows.forEach((r, i) => m.set(rowIds[i]!, r));
    return m;
  }, [rows, rowIds]);
  const colById = useMemo(
    () => new Map(columns.map((c) => [c.id, c])),
    [columns],
  );

  const bounds = { rows: rows.length, cols: columns.length };

  const baseValue = useCallback(
    (rowId: string, columnId: string): unknown => {
      const c = committed[rowId];
      if (c && columnId in c) return c[columnId];
      const row = rowById.get(rowId);
      const col = colById.get(columnId);
      return row && col ? col.value(row) : undefined;
    },
    [committed, rowById, colById],
  );

  const valueAt = useCallback(
    (rowId: string, columnId: string): unknown => {
      const d = drafts[rowId];
      if (d && columnId in d) return d[columnId];
      return baseValue(rowId, columnId);
    },
    [drafts, baseValue],
  );

  const model: GridModelInput = useMemo(() => {
    const cols: GridColumnModel[] = columns.map((col) => ({
      id: col.id,
      editable: (rowId) => {
        const row = rowById.get(rowId);
        return !!row && isColumnEditable(col, row);
      },
      parse: (text, rowId) => {
        const row = rowById.get(rowId);
        return col.parse && row !== undefined
          ? col.parse(text, row)
          : defaultParse(col, text);
      },
    }));
    return { rowIds, columns: cols, valueAt };
  }, [columns, rowIds, rowById, valueAt]);

  /** Kaydedilmemiş satırların hücre hataları: rowId → colId → mesaj. */
  const errors = useMemo(() => {
    const out: Record<string, Record<string, string>> = {};
    for (const rowId of Object.keys(drafts)) {
      const row = rowById.get(rowId);
      if (row === undefined) continue;
      for (const col of columns) {
        if (!isColumnEditable(col, row)) continue;
        const msg = validateCell(col, valueAt(rowId, col.id), row, messages);
        if (msg) (out[rowId] ??= {})[col.id] = msg;
      }
    }
    return out;
  }, [drafts, rowById, columns, valueAt, messages]);

  const dirtyIds = useMemo(
    () => rowIds.filter((id) => drafts[id] !== undefined),
    [rowIds, drafts],
  );

  useEffect(() => {
    onDirtyChange?.(dirtyIds.length);
  }, [dirtyIds.length, onDirtyChange]);

  // Kaydedilmemiş değişiklik varken sekmeyi kapatmadan önce uyar.
  useEffect(() => {
    if (dirtyIds.length === 0) return;
    const onBeforeUnload = (e: BeforeUnloadEvent) => {
      e.preventDefault();
    };
    window.addEventListener("beforeunload", onBeforeUnload);
    return () => window.removeEventListener("beforeunload", onBeforeUnload);
  }, [dirtyIds.length]);

  // Satır sayısı küçülürse seçimi sınırlara çek.
  const sel: CellRange =
    bounds.rows === 0 || bounds.cols === 0
      ? selection
      : {
          anchor: clampCoord(selection.anchor, bounds),
          focus: clampCoord(selection.focus, bounds),
        };
  const range = normalizeRange(sel);
  const active = sel.focus;

  // Odak yönetimi: kullanıcı etkileşiminden sonra etkin hücreye odaklan.
  useEffect(() => {
    if (!focusPending.current || editing) return;
    focusPending.current = false;
    const el = cellRefs.current.get(`${active.row}:${active.col}`);
    if (el) {
      el.focus({ preventScroll: true });
      el.scrollIntoView?.({ block: "nearest", inline: "nearest" });
    }
  }, [active.row, active.col, editing]);

  const announce = useCallback((msg: string) => {
    // Aynı metin art arda okunabilsin diye önce boşalt.
    setAnnouncement("");
    requestAnimationFrame(() => setAnnouncement(msg));
  }, []);

  const apply = useCallback(
    (batch: CellChange[], message?: string) => {
      if (batch.length === 0) return;
      setDrafts((d) => applyChanges(d, batch, baseValue));
      setHistory((h) => pushHistory(h, batch));
      setStatus((s) => {
        const next = { ...s };
        for (const ch of batch) delete next[ch.rowId];
        return next;
      });
      if (message) announce(message);
    },
    [baseValue, announce],
  );

  // ---------------------------------------------------------------------
  // Kaydetme

  // Eşzamansız kayıt en güncel taslağı okusun diye.
  const latest = useRef({ drafts, errors, valueAt, rowById });
  useEffect(() => {
    latest.current = { drafts, errors, valueAt, rowById };
  }, [drafts, errors, valueAt, rowById]);

  const saveRow = useCallback(
    async (rowId: string): Promise<boolean> => {
      if (!onSaveRow || saving.current.has(rowId)) return false;
      const { drafts: d, errors: e, valueAt: v, rowById: rb } = latest.current;
      const changes = d[rowId];
      const row = rb.get(rowId);
      if (!changes || row === undefined) return false;
      const rowErrors = e[rowId];
      if (rowErrors && Object.keys(rowErrors).length > 0) {
        const message = Object.values(rowErrors)[0]!;
        setStatus((s) => ({ ...s, [rowId]: { state: "error", message } }));
        announce(t("table.grid_invalid_row", { message }));
        return false;
      }
      saving.current.add(rowId);
      setStatus((s) => ({ ...s, [rowId]: { state: "saving" } }));
      const sent = { ...changes };
      try {
        await onSaveRow({
          rowId,
          row,
          changes: sent,
          value: (columnId) => v(rowId, columnId),
        });
        setCommittedState((c) => {
          const values = c.rows === rows ? c.values : EMPTY_DRAFTS;
          return {
            rows,
            values: { ...values, [rowId]: { ...values[rowId], ...sent } },
          };
        });
        // Kayıt sürerken yeniden düzenlenen hücreler taslakta kalır.
        setDrafts((cur) => {
          const rowDraft = cur[rowId];
          if (!rowDraft) return cur;
          const rest: Record<string, unknown> = {};
          for (const [k, val] of Object.entries(rowDraft)) {
            if (!(k in sent) || !sameValue(sent[k], val)) rest[k] = val;
          }
          const next = { ...cur };
          if (Object.keys(rest).length) next[rowId] = rest;
          else delete next[rowId];
          return next;
        });
        setHistory((h) => forgetRow(h, rowId));
        setStatus((s) => ({ ...s, [rowId]: { state: "saved" } }));
        return true;
      } catch (err) {
        const message =
          err instanceof Error && err.message
            ? err.message
            : t("table.grid_save_failed");
        setStatus((s) => ({ ...s, [rowId]: { state: "error", message } }));
        announce(t("table.grid_error", { message }));
        return false;
      } finally {
        saving.current.delete(rowId);
      }
    },
    [onSaveRow, announce, t, rows],
  );

  const saveAll = useCallback(async () => {
    let ok = 0;
    for (const id of Object.keys(latest.current.drafts)) {
      if (await saveRow(id)) ok += 1;
    }
    if (ok) announce(t("table.grid_saved_rows", { count: ok }));
  }, [saveRow, announce, t]);

  // ---------------------------------------------------------------------
  // Seçim ve gezinme

  const select = (next: CellRange, focus = true) => {
    const prevRow = rowIds[sel.focus.row];
    const nextRow = rowIds[next.focus.row];
    if (
      saveMode === "row-leave" &&
      prevRow !== undefined &&
      prevRow !== nextRow &&
      latest.current.drafts[prevRow]
    ) {
      void saveRow(prevRow);
    }
    if (focus) focusPending.current = true;
    setSelection(next);
  };

  const cellEditable = (c: CellCoord) => {
    const rowId = rowIds[c.row];
    const col = model.columns[c.col];
    return rowId !== undefined && !!col && col.editable(rowId);
  };

  const cellText = (rowId: string, columnId: string, value: unknown) => {
    const col = colById.get(columnId);
    const row = rowById.get(rowId);
    if (!col || row === undefined) return "";
    return col.toText
      ? col.toText(value, row)
      : defaultToText(col, value, locale);
  };

  const startEdit = (c: CellCoord, seed?: string) => {
    if (!cellEditable(c)) return;
    const rowId = rowIds[c.row]!;
    const colId = columns[c.col]!.id;
    setSelection(singleRange(c));
    setEditing({
      row: c.row,
      col: c.col,
      text: seed ?? cellText(rowId, colId, valueAt(rowId, colId)),
      seeded: seed !== undefined,
    });
  };

  const commitEdit = (text: string, nav?: GridCommitNav) => {
    if (!editing) return;
    const rowId = rowIds[editing.row];
    const col = model.columns[editing.col];
    if (rowId !== undefined && col) {
      const prev = valueAt(rowId, col.id);
      const next = col.parse(text, rowId);
      if (!sameValue(prev, next)) {
        apply([{ rowId, columnId: col.id, prev, next }]);
      }
    }
    const at = { row: editing.row, col: editing.col };
    setEditing(null);
    if (nav) {
      select(singleRange(moveCoord(at, nav.key, bounds, { shift: nav.shift })));
    } else if (tableRef.current?.contains(document.activeElement) ?? true) {
      focusPending.current = true;
    }
  };

  const cancelEdit = () => {
    setEditing(null);
    focusPending.current = true;
  };

  const undo = useCallback(() => {
    const r = undoHistory(history);
    if (!r) return;
    setHistory(r.history);
    setDrafts((d) => applyChanges(d, r.batch, baseValue, "backward"));
    announce(t("table.grid_undone"));
  }, [history, baseValue, announce, t]);

  const redo = useCallback(() => {
    const r = redoHistory(history);
    if (!r) return;
    setHistory(r.history);
    setDrafts((d) => applyChanges(d, r.batch, baseValue, "forward"));
    announce(t("table.grid_redone"));
  }, [history, baseValue, announce, t]);

  const onKeyDown = (e: ReactKeyboardEvent<HTMLTableElement>) => {
    if (editing || bounds.rows === 0) return;
    const ctrl = e.ctrlKey || e.metaKey;
    const key = e.key.length === 1 ? e.key.toLowerCase() : e.key;

    if (ctrl && !e.altKey) {
      if (key === "z") {
        e.preventDefault();
        if (e.shiftKey) redo();
        else undo();
        return;
      }
      if (key === "y") {
        e.preventDefault();
        redo();
        return;
      }
      if (key === "a") {
        e.preventDefault();
        select({
          anchor: { row: 0, col: 0 },
          focus: { row: bounds.rows - 1, col: bounds.cols - 1 },
        });
        return;
      }
      if (key === "d" || key === "r") {
        e.preventDefault();
        const batch =
          key === "d"
            ? fillDownChanges(model, range)
            : fillRightChanges(model, range);
        apply(batch, t("table.grid_filled", { count: batch.length }));
        return;
      }
      if (key === "s") {
        e.preventDefault();
        const rowId = rowIds[active.row];
        if (rowId !== undefined) void saveRow(rowId);
        return;
      }
    }

    if (e.key === "F2" || (e.key === "Enter" && !e.shiftKey && !ctrl)) {
      if (cellEditable(active)) {
        e.preventDefault();
        startEdit(active);
        return;
      }
    }

    if (e.key === "Escape") {
      if (range.top !== range.bottom || range.left !== range.right) {
        e.preventDefault();
        select(singleRange(active));
      }
      return;
    }

    if (e.key === "Delete" || e.key === "Backspace") {
      e.preventDefault();
      const batch = clearChanges(model, range);
      apply(batch, t("table.grid_cleared", { count: batch.length }));
      return;
    }

    if (isNavKey(e.key)) {
      const rtl = typeof document !== "undefined" && document.dir === "rtl";
      const extend = e.shiftKey && e.key !== "Tab" && e.key !== "Enter";
      const next = moveCoord(active, e.key, bounds, {
        shift: e.shiftKey,
        ctrl,
        rtl,
        pageSize: PAGE_SIZE,
      });
      // Tab ilk/son hücrede ızgaradan çıkar (odak tuzağı olmasın).
      if (
        e.key === "Tab" &&
        next.row === active.row &&
        next.col === active.col
      ) {
        return;
      }
      e.preventDefault();
      select(extend ? { anchor: sel.anchor, focus: next } : singleRange(next));
      return;
    }

    if (isPrintableKey(e) && cellEditable(active)) {
      e.preventDefault();
      startEdit(active, e.key);
    }
  };

  const onCopy = (e: ReactClipboardEvent<HTMLTableElement>) => {
    if (editing || bounds.rows === 0) return;
    const matrix = copyMatrix(model, range, cellText);
    e.clipboardData.setData("text/plain", toTsv(matrix));
    e.preventDefault();
    const size = rangeSize(range);
    announce(t("table.grid_copied", { count: size.rows * size.cols }));
  };

  const onCut = (e: ReactClipboardEvent<HTMLTableElement>) => {
    if (editing || bounds.rows === 0) return;
    onCopy(e);
    apply(clearChanges(model, range));
  };

  const onPaste = (e: ReactClipboardEvent<HTMLTableElement>) => {
    if (editing || bounds.rows === 0) return;
    const text = e.clipboardData.getData("text/plain");
    if (!text) return;
    e.preventDefault();
    const matrix = parseTsv(text);
    const batch = pasteChanges(model, range, matrix);
    // Yapıştırılan alanı seç (tek hücre seçiliyse matris boyu kadar).
    const size = rangeSize(range);
    const single = size.rows === 1 && size.cols === 1;
    const mCols = Math.max(0, ...matrix.map((r) => r.length));
    const focus = single
      ? clampCoord(
          {
            row: range.top + matrix.length - 1,
            col: range.left + mCols - 1,
          },
          bounds,
        )
      : sel.focus;
    apply(batch, t("table.grid_pasted", { count: batch.length }));
    select({ anchor: { row: range.top, col: range.left }, focus });
  };

  const onBlur = (e: ReactFocusEvent<HTMLTableElement>) => {
    if (saveMode !== "row-leave") return;
    const next = e.relatedTarget as Node | null;
    if (next && tableRef.current?.contains(next)) return;
    const rowId = rowIds[active.row];
    if (rowId !== undefined && latest.current.drafts[rowId]) {
      void saveRow(rowId);
    }
  };

  // Fare: tıkla, Shift+tıkla, sürükleyerek seç; doldurma tutamacı.
  useEffect(() => {
    const onUp = () => {
      selecting.current = false;
      if (!fill) return;
      setFill(null);
      if (!fill.over) return;
      const target = fillTarget(fill.source, fill.over);
      const batch = fillChanges(model, fill.source, target);
      apply(batch, t("table.grid_filled", { count: batch.length }));
      // Odak kaynak seçimin köşesinde kalır; doldurulan alan seçili olur.
      focusPending.current = true;
      setSelection({
        anchor: { row: target.top, col: target.left },
        focus: { row: target.bottom, col: target.right },
      });
    };
    window.addEventListener("mouseup", onUp);
    return () => window.removeEventListener("mouseup", onUp);
  }, [fill, model, apply, t]);

  const onCellMouseDown = (
    e: ReactMouseEvent<HTMLTableCellElement>,
    c: CellCoord,
  ) => {
    if (e.button !== 0) return;
    const target = e.target as HTMLElement;
    if (target.closest("input,select,textarea")) return;
    // Metin seçimini engelle; odak efektle etkin hücreye taşınır.
    e.preventDefault();
    if (e.shiftKey) {
      select({ anchor: sel.anchor, focus: c });
    } else {
      select(singleRange(c));
    }
    selecting.current = true;
  };

  const onCellMouseEnter = (c: CellCoord) => {
    if (fill) {
      setFill({ source: fill.source, over: c });
    } else if (selecting.current) {
      setSelection((s) => ({ anchor: s.anchor, focus: c }));
    }
  };

  const fillPreview = fill?.over ? fillTarget(fill.source, fill.over) : null;

  // ---------------------------------------------------------------------
  // Çizim

  if (isLoading) {
    return (
      <div className={cn("space-y-2", className)} aria-busy="true">
        <Skeleton className="h-9 w-full max-w-sm" />
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-8 w-full" />
        ))}
      </div>
    );
  }

  const canUndo = history.past.length > 0;
  const canRedo = history.future.length > 0;
  const activeRowId = rowIds[active.row];
  const activeDirty = activeRowId !== undefined && !!drafts[activeRowId];

  return (
    <div className={cn("space-y-2", className)}>
      <div className="flex flex-wrap items-center gap-2">
        <div className="flex min-w-0 flex-1 flex-wrap items-center gap-2">
          {toolbar}
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          <span
            className="text-muted-foreground hidden items-center gap-1 text-xs md:inline-flex"
            title={t("table.grid_help")}
          >
            <Keyboard className="size-3.5" aria-hidden />
            <span className="sr-only">{t("table.grid_help")}</span>
          </span>
          {dirtyIds.length > 0 ? (
            <span className="text-muted-foreground text-xs tabular-nums">
              {t("table.grid_unsaved", { count: dirtyIds.length })}
            </span>
          ) : null}
          <Hint label={`${t("table.grid_undo")} (Ctrl+Z)`} wrap={!canUndo}>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={undo}
              disabled={!canUndo}
              aria-label={t("table.grid_undo")}
            >
              <Undo2 aria-hidden />
            </Button>
          </Hint>
          <Hint label={`${t("table.grid_redo")} (Ctrl+Y)`} wrap={!canRedo}>
            <Button
              type="button"
              variant="ghost"
              size="icon-sm"
              onClick={redo}
              disabled={!canRedo}
              aria-label={t("table.grid_redo")}
            >
              <Redo2 aria-hidden />
            </Button>
          </Hint>
          {onSaveRow ? (
            <>
              <Hint label={"Ctrl+S"} wrap={!activeDirty}>
                <Button
                  type="button"
                  variant="outline"
                  size="sm"
                  disabled={!activeDirty}
                  onClick={() => activeRowId && void saveRow(activeRowId)}
                >
                  {t("table.grid_save_row")}
                </Button>
              </Hint>
              <Button
                type="button"
                size="sm"
                disabled={dirtyIds.length === 0}
                onClick={() => void saveAll()}
              >
                <Save aria-hidden />
                {t("table.grid_save_all", { count: dirtyIds.length })}
              </Button>
            </>
          ) : null}
        </div>
      </div>

      <div
        className="border-border relative w-full overflow-auto rounded-lg border"
        style={maxHeight != null ? { maxHeight } : undefined}
      >
        <Table
          ref={tableRef}
          containerClassName="overflow-visible"
          role="grid"
          aria-label={label}
          aria-rowcount={rows.length + 1}
          aria-colcount={columns.length + 1}
          aria-multiselectable="true"
          aria-describedby={`${gridId}-help`}
          className="w-full border-collapse text-sm select-none"
          onKeyDown={onKeyDown}
          onCopy={onCopy}
          onCut={onCut}
          onPaste={onPaste}
          onBlur={onBlur}
        >
          <TableHeader className="bg-muted sticky top-0 z-20 [&_tr]:border-b-0">
            <TableRow
              role="row"
              aria-rowindex={1}
              className="border-0 transition-none hover:bg-transparent"
            >
              <TableHead
                role="columnheader"
                aria-colindex={1}
                scope="col"
                className="border-border text-muted-foreground h-auto w-14 border-e border-b px-2 py-1.5 text-start text-xs font-medium"
              >
                <span className="sr-only">{t("table.grid_status")}</span>#
              </TableHead>
              {columns.map((col, ci) => (
                <TableHead
                  key={col.id}
                  role="columnheader"
                  aria-colindex={ci + 2}
                  scope="col"
                  style={col.width != null ? { width: col.width } : undefined}
                  className={cn(
                    "border-border text-muted-foreground h-auto border-e border-b px-2 py-1.5 text-start text-xs font-medium whitespace-nowrap last:border-e-0",
                    col.align && alignClass[col.align],
                    !col.editor && "bg-muted/60",
                  )}
                >
                  {col.header}
                  {col.required ? (
                    <span aria-hidden className="text-destructive ms-0.5">
                      *
                    </span>
                  ) : null}
                </TableHead>
              ))}
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.length === 0 ? (
              <TableRow
                role="row"
                aria-rowindex={2}
                className="border-0 transition-none hover:bg-transparent"
              >
                <TableCell
                  role="gridcell"
                  aria-colindex={1}
                  colSpan={columns.length + 1}
                  className="px-3 py-10 text-center"
                >
                  <p className="text-foreground text-sm font-medium">
                    {emptyTitle ?? t("table.grid_empty")}
                  </p>
                  {emptyDescription ? (
                    <p className="text-muted-foreground mt-1 text-sm">
                      {emptyDescription}
                    </p>
                  ) : null}
                </TableCell>
              </TableRow>
            ) : (
              rows.map((row, ri) => {
                const rowId = rowIds[ri]!;
                const dirty = !!drafts[rowId];
                const st = status[rowId];
                const rowErr = errors[rowId];
                const rowSelected = ri >= range.top && ri <= range.bottom;
                return (
                  <TableRow
                    key={rowId}
                    role="row"
                    aria-rowindex={ri + 2}
                    data-dirty={dirty || undefined}
                    className={cn(
                      "border-border border-b transition-none last:border-b-0 hover:bg-transparent",
                      dirty &&
                        "bg-amber-50/60 hover:bg-amber-50/60 dark:bg-amber-950/20 dark:hover:bg-amber-950/20",
                    )}
                  >
                    <TableHead
                      role="rowheader"
                      scope="row"
                      aria-colindex={1}
                      className={cn(
                        "border-border text-muted-foreground h-auto border-e px-2 py-1 text-start text-xs font-normal whitespace-normal tabular-nums",
                        rowSelected && "bg-muted",
                      )}
                      title={st?.state === "error" ? st.message : undefined}
                    >
                      <span className="inline-flex items-center gap-1">
                        <RowStatusIcon
                          dirty={dirty}
                          status={st}
                          invalid={!!rowErr}
                        />
                        {rowLabel ? rowLabel(row, ri) : ri + 1}
                      </span>
                      <span className="sr-only">
                        {st?.state === "saving"
                          ? t("table.grid_saving")
                          : st?.state === "error"
                            ? t("table.grid_error", { message: st.message })
                            : st?.state === "saved" && !dirty
                              ? t("table.grid_saved")
                              : dirty
                                ? t("table.grid_dirty")
                                : null}
                      </span>
                    </TableHead>
                    {columns.map((col, ci) => {
                      const c = { row: ri, col: ci };
                      const isActive = ri === active.row && ci === active.col;
                      const inRange = rangeContains(range, c);
                      const isEditing =
                        editing?.row === ri && editing?.col === ci;
                      const editable = isColumnEditable(col, row);
                      const value = valueAt(rowId, col.id);
                      const changed = dirty && col.id in (drafts[rowId] ?? {});
                      const err = rowErr?.[col.id];
                      const inFill =
                        fillPreview &&
                        rangeContains(fillPreview, c) &&
                        !inRange;
                      const showHandle =
                        !editing &&
                        ri === range.bottom &&
                        ci === range.right &&
                        editable;
                      const errId = err ? `${gridId}-e-${ri}-${ci}` : undefined;
                      return (
                        <TableCell
                          key={col.id}
                          ref={(el) => {
                            const k = `${ri}:${ci}`;
                            if (el) cellRefs.current.set(k, el);
                            else cellRefs.current.delete(k);
                          }}
                          role="gridcell"
                          aria-colindex={ci + 2}
                          aria-selected={inRange}
                          aria-readonly={!editable || undefined}
                          aria-invalid={err ? true : undefined}
                          aria-describedby={errId}
                          tabIndex={isActive && !isEditing ? 0 : -1}
                          onMouseDown={(e) => onCellMouseDown(e, c)}
                          onMouseEnter={() => onCellMouseEnter(c)}
                          onDoubleClick={() => startEdit(c)}
                          className={cn(
                            "border-border relative h-9 border-e px-2 py-1 align-middle outline-none last:border-e-0",
                            col.align && alignClass[col.align],
                            !editable && "bg-muted/30 text-muted-foreground",
                            inRange && "bg-primary/10",
                            changed && "font-medium",
                            err && "bg-destructive/10",
                            isActive && "ring-primary z-10 ring-2 ring-inset",
                            inFill &&
                              "outline-primary outline-1 -outline-offset-2 outline-dashed",
                          )}
                          style={
                            col.width != null ? { width: col.width } : undefined
                          }
                        >
                          {isEditing ? (
                            <GridCellEditor
                              kind={col.editor ?? "text"}
                              initialText={editing.text}
                              seeded={editing.seeded}
                              options={col.options}
                              suggestions={col.suggestions}
                              listId={`${gridId}-list-${ci}`}
                              label={`${col.header}, ${t("table.grid_row", { n: ri + 1 })}`}
                              placeholder={col.placeholder}
                              invalid={!!err}
                              onCommit={commitEdit}
                              onCancel={cancelEdit}
                            />
                          ) : (
                            <div className="min-w-0 truncate">
                              {col.render
                                ? col.render(value, row)
                                : displayText(col, value, row, locale) || (
                                    <span className="text-muted-foreground">
                                      {editable && col.placeholder
                                        ? col.placeholder
                                        : ""}
                                    </span>
                                  )}
                            </div>
                          )}
                          {err ? (
                            <span id={errId} className="sr-only">
                              {err}
                            </span>
                          ) : null}
                          {err && !isEditing ? (
                            <AlertCircle
                              aria-hidden
                              className="text-destructive absolute end-0.5 top-0.5 size-3"
                            >
                              <title>{err}</title>
                            </AlertCircle>
                          ) : null}
                          {showHandle ? (
                            <span
                              aria-hidden
                              title={t("table.grid_fill_handle")}
                              onMouseDown={(e) => {
                                if (e.button !== 0) return;
                                e.preventDefault();
                                e.stopPropagation();
                                setFill({ source: range, over: null });
                              }}
                              className="bg-primary border-background absolute -end-1 -bottom-1 z-20 size-2 cursor-crosshair border"
                            />
                          ) : null}
                        </TableCell>
                      );
                    })}
                  </TableRow>
                );
              })
            )}
          </TableBody>
        </Table>
      </div>
      <p id={`${gridId}-help`} className="sr-only">
        {t("table.grid_help")}
      </p>
      <div aria-live="polite" role="status" className="sr-only">
        {announcement}
      </div>
    </div>
  );
}

function RowStatusIcon({
  dirty,
  status,
  invalid,
}: {
  dirty: boolean;
  status?: RowStatus;
  invalid: boolean;
}) {
  if (status?.state === "saving") {
    return <Loader2 aria-hidden className="size-3 animate-spin" />;
  }
  if (status?.state === "error" || (dirty && invalid)) {
    return <AlertCircle aria-hidden className="text-destructive size-3" />;
  }
  if (dirty) {
    return (
      <Circle aria-hidden className="size-2.5 fill-amber-500 text-amber-500" />
    );
  }
  if (status?.state === "saved") {
    return <Check aria-hidden className="size-3 text-emerald-600" />;
  }
  return <span aria-hidden className="inline-block size-3" />;
}
