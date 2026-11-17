import {
  createTable,
  getCoreRowModel,
  getSortedRowModel,
  type ColumnDef,
} from "@tanstack/react-table";
import { describe, expect, it } from "vitest";

import {
  createActionsColumn,
  createDateColumn,
  createMoneyColumn,
  createStatusColumn,
} from "./column-helpers";

type Inv = { id: string; total: string; date: string; status: string };

const rows: Inv[] = [
  { id: "a", total: "100.00", date: "2026-02-01", status: "active" },
  { id: "b", total: "9.50", date: "2026-03-01", status: "draft" },
  { id: "c", total: "1000", date: "2025-12-01", status: "cancelled" },
];

function sortedIds(column: ColumnDef<Inv, unknown>, desc = false) {
  const table = createTable<Inv>({
    data: rows,
    columns: [column],
    getRowId: (r) => r.id,
    getCoreRowModel: getCoreRowModel(),
    getSortedRowModel: getSortedRowModel(),
    state: {
      sorting: [{ id: column.id!, desc }],
      columnPinning: {},
      columnVisibility: {},
      columnOrder: [],
    },
    onStateChange: () => {},
    renderFallbackValue: null,
  });
  return table.getSortedRowModel().rows.map((r) => r.id);
}

describe("createMoneyColumn", () => {
  const col = createMoneyColumn<Inv>({
    id: "total",
    labelKey: "office.col_payable",
    accessor: (r) => r.total,
  });

  it("sağa hizalı, i18n başlıklı", () => {
    expect(col.meta?.align).toBe("end");
    expect(col.meta?.labelKey).toBe("office.col_payable");
  });

  it("sayısal sıralar (metin değil)", () => {
    expect(sortedIds(col)).toEqual(["b", "a", "c"]);
    expect(sortedIds(col, true)).toEqual(["c", "a", "b"]);
  });

  it("align meta ile değiştirilebilir", () => {
    const c = createMoneyColumn<Inv>({
      id: "t",
      labelKey: "x",
      accessor: (r) => r.total,
      meta: { align: "start" },
    });
    expect(c.meta?.align).toBe("start");
  });
});

describe("createDateColumn", () => {
  it("tarihe göre sıralar", () => {
    const col = createDateColumn<Inv>({
      id: "date",
      labelKey: "office.col_date",
      accessor: (r) => r.date,
    });
    expect(sortedIds(col)).toEqual(["c", "a", "b"]);
  });
});

describe("createStatusColumn", () => {
  const variants = {
    active: {
      labelKey: "office.api.invoice_state_active",
      tone: "ok" as const,
    },
    draft: { labelKey: "office.api.invoice_state_draft" },
  };

  it("filterable iken faceted süzgeç seçenekleri değişkenlerden", () => {
    const col = createStatusColumn<Inv>({
      id: "status",
      labelKey: "office.col_status",
      accessor: (r) => r.status,
      variants,
      filterable: true,
    });
    expect(col.meta?.filterVariant).toBe("faceted");
    expect(col.meta?.filterOptions?.map((o) => o.value)).toEqual([
      "active",
      "draft",
    ]);
    expect(col.enableColumnFilter).toBe(true);
  });

  it("filterable değilse süzgeç yok", () => {
    const col = createStatusColumn<Inv>({
      id: "status",
      labelKey: "office.col_status",
      accessor: (r) => r.status,
      variants,
    });
    expect(col.enableColumnFilter).toBe(false);
  });
});

describe("createActionsColumn", () => {
  it("sıralanmaz, gizlenmez, başlık yalnızca ekran okuyucuya", () => {
    const col = createActionsColumn<Inv>({ cell: () => null });
    expect(col.id).toBe("actions");
    expect(col.enableSorting).toBe(false);
    expect(col.enableHiding).toBe(false);
    expect(col.meta?.srOnlyHeader).toBe(true);
  });
});
