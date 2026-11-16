"use client";

import type { ColumnDef, RowData } from "@tanstack/react-table";

import { Checkbox } from "@/components/ui/checkbox";
import { useLocale } from "@/providers/locale-provider";

export function createSelectColumnDef<TData extends RowData>(): ColumnDef<
  TData,
  unknown
> {
  return {
    id: "__select",
    size: 40,
    enableSorting: false,
    enableHiding: false,
    enableColumnFilter: false,
    enableResizing: false,
    enablePinning: true,
    meta: {
      labelKey: "table.select",
      enableHiding: false,
    },
    header: ({ table }) => (
      <LabeledCheckbox
        labelKey="common.select_all"
        checked={
          table.getIsAllPageRowsSelected() ||
          (table.getIsSomePageRowsSelected() && "indeterminate")
        }
        onCheckedChange={(value) =>
          table.toggleAllPageRowsSelected(Boolean(value))
        }
      />
    ),
    cell: ({ row }) => (
      <LabeledCheckbox
        labelKey="common.select_row"
        checked={row.getIsSelected()}
        disabled={!row.getCanSelect()}
        onCheckedChange={(value) => row.toggleSelected(Boolean(value))}
      />
    ),
  };
}

/** Tablo seçim kutusu, Türkçe erişilebilir adla (#194). */
function LabeledCheckbox({
  labelKey,
  ...props
}: React.ComponentProps<typeof Checkbox> & { labelKey: string }) {
  const { t } = useLocale();
  return <Checkbox aria-label={t(labelKey)} {...props} />;
}
