"use client";

import type { ColumnDef } from "@tanstack/react-table";
import { Star } from "lucide-react";
import { useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { DataTable, createColumn, createDateColumn } from "@/components/tables";
import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import { StorageAccessBadge } from "@/features/storage/components/storage-access-badge";
import {
  StorageContextMenu,
  type StorageAction,
} from "@/features/storage/components/storage-context-menu";
import { StorageFileThumbnail } from "@/features/storage/components/storage-file-thumbnail";
import { formatBytes } from "@/features/storage/lib/format";
import type { StorageObject } from "@/features/storage/types";
import { useLocale } from "@/providers/locale-provider";

/** Satırı ad hücresindeki işaretle bulur (çekirdek satıra öznitelik koymuyor). */
const ROW_KEY_ATTR = "data-storage-key";

export function StorageFileList({
  items,
  loading,
  selected,
  canWrite,
  trash,
  onToggle,
  onOpen,
  onAction,
}: {
  items: StorageObject[];
  loading?: boolean;
  selected: Set<string>;
  canWrite: boolean;
  trash?: boolean;
  onToggle: (key: string, checked: boolean) => void;
  onOpen: (item: StorageObject) => void;
  onAction: (action: StorageAction, item: StorageObject) => void;
}) {
  const { t } = useLocale();
  // Sağ tıklanan satır; bağlam menüsü tek, içeriği bu satıra göre çizilir.
  const [menuTarget, setMenuTarget] = useState<StorageObject | null>(null);

  const columns = useMemo<ColumnDef<StorageObject, unknown>[]>(
    () => [
      createColumn<StorageObject>({
        id: "select",
        labelKey: "table.select",
        enableSorting: false,
        enableHiding: false,
        meta: { srOnlyHeader: true, cellClassName: "w-10" },
        cell: ({ row }) => (
          <Checkbox
            checked={selected.has(row.original.key)}
            onCheckedChange={(value) =>
              onToggle(row.original.key, value === true)
            }
            aria-label={row.original.name}
          />
        ),
      }),
      createColumn<StorageObject>({
        id: "name",
        labelKey: "storage.col_name",
        accessorKey: "name",
        gridPrimary: true,
        cell: ({ row }) => {
          const item = row.original;
          return (
            <Button
              variant="ghost"
              className="h-auto max-w-md justify-start gap-2 p-0 text-start font-normal hover:bg-transparent has-[>svg]:px-0"
              onClick={() => onOpen(item)}
              data-storage-key={item.key}
            >
              <StorageFileThumbnail
                object={item}
                className="size-9 shrink-0 rounded-md"
                iconClassName="size-4"
              />
              <span className="truncate font-medium">{item.name}</span>
              {item.is_starred ? (
                <Star className="size-3.5 fill-amber-400 text-amber-400" />
              ) : null}
            </Button>
          );
        },
      }),
      createColumn<StorageObject>({
        id: "file_kind",
        labelKey: "storage.col_type",
        accessorKey: "file_kind",
        meta: { cellClassName: "text-muted-foreground" },
        cell: ({ row }) =>
          t(`storage.kind_${row.original.file_kind}` as "storage.kind_unknown"),
      }),
      createColumn<StorageObject>({
        id: "size",
        labelKey: "storage.col_size",
        accessorKey: "size",
        meta: { cellClassName: "text-muted-foreground" },
        cell: ({ row }) =>
          row.original.kind === "folder" ? "—" : formatBytes(row.original.size),
      }),
      createDateColumn<StorageObject>({
        id: "updated_at",
        labelKey: "storage.col_modified",
        accessor: (item) => item.updated_at,
        style: "datetime",
        className: "text-muted-foreground",
      }),
      createColumn<StorageObject>({
        id: "access",
        labelKey: "storage.col_access",
        accessorKey: "access",
        cell: ({ row }) => <StorageAccessBadge access={row.original.access} />,
      }),
    ],
    [onOpen, onToggle, selected, t],
  );

  if (loading) {
    return (
      <div className="space-y-2">
        {Array.from({ length: 6 }).map((_, i) => (
          <Skeleton key={i} className="h-11 w-full" />
        ))}
      </div>
    );
  }

  const table = (
    <DataTable<StorageObject>
      columns={columns}
      data={items}
      getRowId={(item) => item.key}
      getRowClassName={(item) =>
        selected.has(item.key) ? "bg-accent/40" : undefined
      }
      // Gezgin kendi araç çubuğu, seçimi, ızgara görünümü ve sıralamasıyla
      // çalışır; tablo yalnızca liste görünümünü çizer. Mobilde de tablo
      // kalır (ızgara görünümü gezginde ayrı bir mod).
      features={{
        toolbar: false,
        sorting: false,
        pagination: false,
        rowSelection: false,
        columnFilters: false,
        globalFilter: false,
        facetedFilters: false,
        viewMode: false,
        mobileAutoCards: false,
        columnVisibility: false,
        columnResizing: false,
        columnPinning: false,
        columnOrdering: false,
        density: false,
      }}
    />
  );

  if (!items.length) return table;

  const rowItem = (target: EventTarget) => {
    if (!(target instanceof Element)) return undefined;
    const key = target
      .closest("tbody tr")
      ?.querySelector(`[${ROW_KEY_ATTR}]`)
      ?.getAttribute(ROW_KEY_ATTR);
    return key == null ? undefined : items.find((item) => item.key === key);
  };

  // Çekirdek satırı sarmaya izin vermediği için bağlam menüsü ve çift tıklama
  // tablo kabında yakalanıp tıklanan satıra yönlendirilir. Satır dışında
  // (başlık) menü açılmaz; tarayıcının kendi menüsü kalır.
  return (
    <StorageContextMenu
      object={menuTarget ?? items[0]!}
      canWrite={canWrite}
      trash={trash}
      onAction={onAction}
    >
      <div
        onContextMenuCapture={(event) => {
          const item = rowItem(event.target);
          if (item) setMenuTarget(item);
          else event.stopPropagation();
        }}
        onPointerDownCapture={(event) => {
          // Dokunmatik uzun basış da menüyü açar.
          if (event.pointerType === "mouse") return;
          const item = rowItem(event.target);
          if (item) setMenuTarget(item);
          else event.stopPropagation();
        }}
        onDoubleClick={(event) => {
          const item = rowItem(event.target);
          if (item) onOpen(item);
        }}
      >
        {table}
      </div>
    </StorageContextMenu>
  );
}
