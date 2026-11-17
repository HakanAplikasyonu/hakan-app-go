"use client";

import type { Cell, Table } from "@tanstack/react-table";
import {
  useEffect,
  useRef,
  useState,
  type KeyboardEvent as ReactKeyboardEvent,
  type ReactNode,
} from "react";

import type {
  ColumnEditVariant,
  DataTableColumnMeta,
} from "@/components/tables/types";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { useLocale } from "@/providers/locale-provider";
import { cn } from "@/lib/utils";

export type CellEditPayload<TData> = {
  rowId: string;
  columnId: string;
  value: unknown;
  row: TData;
};

type EditableCellProps<TData> = {
  cell: Cell<TData, unknown>;
  table?: Table<TData>;
  children: ReactNode;
  enabled: boolean;
  editing: boolean;
  onStartEdit: () => void;
  onCancelEdit: () => void;
  onCommit: (value: unknown) => void;
};

function resolveEditVariant(
  meta: DataTableColumnMeta | undefined,
): ColumnEditVariant | undefined {
  if (!meta?.editVariant) return undefined;
  return meta.editVariant;
}

export function EditableCell<TData>({
  cell,
  children,
  enabled,
  editing,
  onStartEdit,
  onCancelEdit,
  onCommit,
}: EditableCellProps<TData>) {
  const meta = cell.column.columnDef.meta as DataTableColumnMeta | undefined;
  const variant = resolveEditVariant(meta);
  const editable = enabled && Boolean(variant);

  if (!editable) {
    return <>{children}</>;
  }

  if (!editing) {
    return (
      <div
        role="button"
        tabIndex={0}
        className="focus-visible:ring-ring min-w-0 cursor-text rounded-sm outline-none focus-visible:ring-2"
        onDoubleClick={(e) => {
          e.stopPropagation();
          onStartEdit();
        }}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            onStartEdit();
          }
        }}
        title="Double-click to edit"
      >
        {children}
      </div>
    );
  }

  return (
    <CellEditor
      variant={variant!}
      options={meta?.editOptions}
      initialValue={cell.getValue()}
      onCancel={onCancelEdit}
      onCommit={onCommit}
    />
  );
}

function CellEditor({
  variant,
  options,
  initialValue,
  onCancel,
  onCommit,
}: {
  variant: ColumnEditVariant;
  options?: DataTableColumnMeta["editOptions"];
  initialValue: unknown;
  onCancel: () => void;
  onCommit: (value: unknown) => void;
}) {
  const { t } = useLocale();
  const [value, setValue] = useState(initialValue);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  if (variant === "boolean") {
    return (
      <div className="flex items-center gap-2 py-0.5">
        <Checkbox
          checked={Boolean(value)}
          onCheckedChange={(checked) => {
            const next = Boolean(checked);
            setValue(next);
            onCommit(next);
          }}
          onKeyDown={(e) => {
            if (e.key === "Escape") onCancel();
          }}
        />
        <span className="text-muted-foreground text-xs">
          {Boolean(value) ? t("table.true") : t("table.false")}
        </span>
      </div>
    );
  }

  if (variant === "select") {
    return (
      <Select
        value={String(value ?? "")}
        onValueChange={(next) => {
          setValue(next);
          onCommit(next);
        }}
        open
        onOpenChange={(open) => {
          if (!open) onCancel();
        }}
      >
        <SelectTrigger className="h-8 w-full">
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {(options ?? []).map((option) => (
            <SelectItem key={option.value} value={option.value}>
              {option.label}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    );
  }

  return (
    <Input
      ref={inputRef}
      type={variant === "number" ? "number" : "text"}
      className={cn("h-8")}
      value={value == null ? "" : String(value)}
      onChange={(e) => {
        if (variant === "number") {
          const n = e.target.value === "" ? "" : Number(e.target.value);
          setValue(n);
        } else {
          setValue(e.target.value);
        }
      }}
      onBlur={() => {
        const next =
          variant === "number"
            ? value === "" || value == null
              ? 0
              : Number(value)
            : value;
        onCommit(next);
      }}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          (e.target as HTMLInputElement).blur();
        }
        if (e.key === "Escape") {
          e.preventDefault();
          onCancel();
        }
      }}
    />
  );
}

// ---------------------------------------------------------------------------
// Düzenlenebilir ızgara (#394) hücre düzenleyicisi

/** Izgara düzenleyici türleri. `money` ondalık metin (kuruş hassasiyeti). */
export type GridEditorKind = "text" | "number" | "money" | "select";

export type GridCommitNav = {
  key: "Enter" | "Tab";
  shift: boolean;
};

type GridCellEditorProps = {
  kind: GridEditorKind;
  /** Düzenleyicinin açılış metni (yazmaya başlanan karakter ya da değer). */
  initialText: string;
  /** Yazmaya başlanarak açıldıysa imleç sona, aksi halde metin seçili. */
  seeded?: boolean;
  options?: { label: string; value: string }[];
  /** Metin düzenleyicide öneri listesi (datalist). */
  suggestions?: string[];
  listId?: string;
  label: string;
  placeholder?: string;
  invalid?: boolean;
  onCommit: (text: string, nav?: GridCommitNav) => void;
  onCancel: () => void;
};

/**
 * Izgara hücresi düzenleyicisi. Enter/Tab kaydedip gezinmeyi ızgaraya
 * bırakır (Shift ile ters yön), Escape vazgeçer, odak kaybı kaydeder.
 */
export function GridCellEditor({
  kind,
  initialText,
  seeded,
  options,
  suggestions,
  listId,
  label,
  placeholder,
  invalid,
  onCommit,
  onCancel,
}: GridCellEditorProps) {
  const [text, setText] = useState(initialText);
  const ref = useRef<HTMLInputElement & HTMLSelectElement>(null);
  const done = useRef(false);

  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    el.focus();
    if (el instanceof HTMLInputElement) {
      if (seeded) {
        const end = el.value.length;
        el.setSelectionRange(end, end);
      } else {
        el.select();
      }
    }
  }, [seeded]);

  const finish = (nav?: GridCommitNav) => {
    if (done.current) return;
    done.current = true;
    onCommit(text, nav);
  };

  const onKeyDown = (e: ReactKeyboardEvent) => {
    if (e.key === "Enter" || e.key === "Tab") {
      e.preventDefault();
      e.stopPropagation();
      finish({ key: e.key, shift: e.shiftKey });
    } else if (e.key === "Escape") {
      e.preventDefault();
      e.stopPropagation();
      done.current = true;
      onCancel();
    } else {
      // Ok tuşları vb. düzenleyicinin içinde kalsın, ızgara gezinmesin.
      e.stopPropagation();
    }
  };

  const common = {
    "aria-label": label,
    "aria-invalid": invalid || undefined,
    className: cn(
      "border-primary bg-background h-8 w-full min-w-0 rounded-sm border px-1.5 text-sm outline-none",
      (kind === "number" || kind === "money") && "text-end tabular-nums",
    ),
    onKeyDown,
    onBlur: () => finish(),
  };

  if (kind === "select") {
    return (
      <select
        ref={ref}
        {...common}
        value={text}
        onChange={(e) => setText(e.target.value)}
      >
        <option value="">—</option>
        {(options ?? []).map((o) => (
          <option key={o.value} value={o.value}>
            {o.label}
          </option>
        ))}
      </select>
    );
  }

  return (
    <>
      <input
        ref={ref}
        {...common}
        type="text"
        inputMode={kind === "text" ? undefined : "decimal"}
        autoComplete="off"
        spellCheck={false}
        placeholder={placeholder}
        list={suggestions?.length ? listId : undefined}
        value={text}
        onChange={(e) => setText(e.target.value)}
      />
      {suggestions?.length && listId ? (
        <datalist id={listId}>
          {suggestions.map((s) => (
            <option key={s} value={s} />
          ))}
        </datalist>
      ) : null}
    </>
  );
}
