"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import type { ColumnDef } from "@tanstack/react-table";
import { Plus } from "lucide-react";
import { useMemo, useState } from "react";
import { toast } from "sonner";

import { EntityTable } from "@/components/entity";
import {
  createColumn,
  createStatusColumn,
  type StatusVariant,
} from "@/components/tables";
import { Button } from "@/components/ui/button";
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
import { usePermission } from "@/providers/permission-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  bankPostingService,
  type BankAlias,
  type BankAliasInput,
} from "../services/bank-posting.service";

const statusVariants: Record<string, StatusVariant> = {
  learned: { labelKey: "office.bank.alias.status_learned", tone: "info" },
  rule: { labelKey: "office.bank.alias.status_rule", tone: "ok" },
};

function errText(err: unknown, fallback: string) {
  return isApiError(err) ? err.message : fallback;
}

type MatchType = BankAliasInput["match_type"];
type Dir = "both" | "A" | "B";

/**
 * Ödeyen / alıcı takma adları (#383): bankadaki karşı taraf (IBAN, VKN/TCKN
 * ya da ad) → mizandaki cari. Kararlarla öğrenilenler, kural adayları
 * ("kural yapılsın mı?") ve elle tanımlama.
 */
export function BankAliasPanel({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can("tenant.bank.write");
  const qc = useQueryClient();
  const key = ["tenant", slug.toLowerCase(), "bank-aliases", engagement];
  const aliases = useQuery({
    queryKey: [...key, "list"],
    queryFn: () => bankPostingService.aliases(slug, engagement),
  });
  const candidates = useQuery({
    queryKey: [...key, "candidates"],
    queryFn: () => bankPostingService.aliasCandidates(slug, engagement),
  });
  const refresh = () => void qc.invalidateQueries({ queryKey: key });
  const promote = useMutation({
    mutationFn: (id: string) =>
      bankPostingService.promoteAlias(slug, engagement, id),
    onSuccess: () => {
      toast.success(t("office.bank.alias.promoted"));
      refresh();
    },
    onError: (e) => toast.error(errText(e, t("office.bank.posting.failed"))),
  });
  const remove = useMutation({
    mutationFn: (id: string) =>
      bankPostingService.deleteAlias(slug, engagement, id),
    onSuccess: () => {
      toast.success(t("office.bank.alias.deleted"));
      refresh();
    },
    onError: (e) => toast.error(errText(e, t("office.bank.posting.failed"))),
  });

  const columns = useMemo<ColumnDef<BankAlias, unknown>[]>(
    () => [
      createColumn<BankAlias>({
        id: "counterparty",
        labelKey: "office.bank.alias.col_counterparty",
        accessorFn: (x) => x.display_name || x.match_value,
        gridPrimary: true,
        meta: { cellClassName: "min-w-56" },
        cell: ({ row }) => {
          const x = row.original;
          return (
            <>
              <div className="max-w-sm truncate">
                {x.display_name || x.match_value}
              </div>
              <div className="text-text-3 max-w-sm truncate text-xs">
                {t(`office.bank.alias.type_${x.match_type}`)}
                {x.match_type !== "name" ? (
                  <span className="figure"> · {x.match_value}</span>
                ) : null}
                {x.direction
                  ? ` · ${t(x.direction === "A" ? "office.bank.direction_in" : "office.bank.direction_out")}`
                  : null}
              </div>
            </>
          );
        },
      }),
      createColumn<BankAlias>({
        id: "account",
        labelKey: "office.bank.posting.col_account",
        accessorKey: "account_code",
        cell: ({ row }) => (
          <span className="flex items-baseline gap-2">
            <span className="figure font-semibold">
              {row.original.account_code}
            </span>
            <span className="text-text-2 truncate text-sm">
              {row.original.account_name}
            </span>
          </span>
        ),
      }),
      createColumn<BankAlias>({
        id: "decisions",
        labelKey: "office.bank.alias.col_decisions",
        accessorFn: (x) => x.accepts + x.corrections,
        meta: { align: "end" },
        cell: ({ row }) => (
          <span className="figure text-sm">
            {row.original.accepts + row.original.corrections}
          </span>
        ),
      }),
      createStatusColumn<BankAlias>({
        id: "status",
        labelKey: "office.bank.posting.col_status",
        accessor: (x) => x.status,
        variants: statusVariants,
      }),
    ],
    [t],
  );

  const cands = candidates.data?.items ?? [];

  return (
    <div className="space-y-4">
      {cands.length > 0 ? (
        <section
          aria-label={t("office.bank.alias.candidates_title")}
          className="bg-brand-tint border-line space-y-2 rounded-xl border p-4"
        >
          <h2 className="font-display text-base font-semibold">
            {t("office.bank.alias.candidates_title")}
          </h2>
          <ul className="space-y-2">
            {cands.map((c) => (
              <li
                key={c.uuid}
                className="flex flex-wrap items-center justify-between gap-2"
              >
                <span className="text-sm">{c.question}</span>
                {canWrite ? (
                  <Button
                    size="sm"
                    disabled={promote.isPending}
                    onClick={() => promote.mutate(c.uuid)}
                  >
                    {t("office.bank.alias.promote")}
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        </section>
      ) : null}

      {canWrite ? (
        <AliasForm engagement={engagement} onSaved={refresh} />
      ) : null}

      <EntityTable<BankAlias>
        columns={columns}
        data={aliases.data?.items ?? []}
        getRowId={(x) => x.uuid}
        isLoading={aliases.isLoading}
        isError={aliases.isError}
        errorTitle={t("office.api.load_error")}
        errorDescription={aliases.error?.message}
        onRetry={() => void aliases.refetch()}
        emptyTitle={t("office.bank.alias.empty_title")}
        emptyDescription={t("office.bank.alias.empty")}
        rowActions={
          canWrite
            ? (x) => (
                <div className="flex justify-end gap-1.5">
                  {x.status === "learned" ? (
                    <Button
                      size="sm"
                      variant="outline"
                      onClick={() => promote.mutate(x.uuid)}
                    >
                      {t("office.bank.alias.promote")}
                    </Button>
                  ) : null}
                  <Button
                    size="sm"
                    variant="outline"
                    onClick={() => remove.mutate(x.uuid)}
                  >
                    {t("common.delete")}
                  </Button>
                </div>
              )
            : undefined
        }
        manual={{ pagination: false, sorting: false, filtering: false }}
        features={{
          sorting: false,
          globalFilter: false,
          columnFilters: false,
          rowSelection: false,
          facetedFilters: false,
          viewMode: false,
          pagination: false,
          persistKey: "office.bank.alias.table",
        }}
      />
    </div>
  );
}

function AliasForm({
  engagement,
  onSaved,
}: {
  engagement: string;
  onSaved: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [open, setOpen] = useState(false);
  const [type, setType] = useState<MatchType>("name");
  const [dir, setDir] = useState<Dir>("both");
  const [value, setValue] = useState("");
  const [code, setCode] = useState("");
  const [name, setName] = useState("");
  const save = useMutation({
    mutationFn: () =>
      bankPostingService.createAlias(slug, engagement, {
        match_type: type,
        match_value: value.trim(),
        direction: dir === "both" ? null : dir,
        account_code: code.trim(),
        account_name: name.trim(),
      }),
    onSuccess: () => {
      toast.success(t("office.bank.alias.saved"));
      setValue("");
      setCode("");
      setName("");
      setOpen(false);
      onSaved();
    },
    onError: (e) => toast.error(errText(e, t("office.bank.posting.failed"))),
  });
  if (!open)
    return (
      <Button variant="outline" onClick={() => setOpen(true)}>
        <Plus className="size-4" aria-hidden />
        {t("office.bank.alias.add")}
      </Button>
    );
  return (
    <form
      className="bg-surface border-line grid gap-3 rounded-xl border p-4 sm:grid-cols-2 lg:grid-cols-3"
      onSubmit={(e) => {
        e.preventDefault();
        save.mutate();
      }}
    >
      <div className="space-y-1.5">
        <Label>{t("office.bank.alias.match_type")}</Label>
        <Select value={type} onValueChange={(v) => setType(v as MatchType)}>
          <SelectTrigger aria-label={t("office.bank.alias.match_type")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {(["name", "iban", "tax_id"] as const).map((k) => (
              <SelectItem key={k} value={k}>
                {t(`office.bank.alias.type_${k}`)}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5 lg:col-span-2">
        <Label htmlFor="alias-value">
          {t("office.bank.alias.match_value")}
        </Label>
        <Input
          id="alias-value"
          value={value}
          placeholder={t(`office.bank.alias.placeholder_${type}`)}
          onChange={(e) => setValue(e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label>{t("office.bank.alias.direction")}</Label>
        <Select value={dir} onValueChange={(v) => setDir(v as Dir)}>
          <SelectTrigger aria-label={t("office.bank.alias.direction")}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="both">{t("office.bank.alias.both")}</SelectItem>
            <SelectItem value="A">{t("office.bank.direction_in")}</SelectItem>
            <SelectItem value="B">{t("office.bank.direction_out")}</SelectItem>
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="alias-code">
          {t("office.bank.posting.account_code")}
        </Label>
        <Input
          id="alias-code"
          className="figure"
          value={code}
          placeholder="120.01.015"
          onChange={(e) => setCode(e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor="alias-name">
          {t("office.bank.posting.account_name")}
        </Label>
        <Input
          id="alias-name"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
      </div>
      <div className="flex justify-end gap-2 sm:col-span-2 lg:col-span-3">
        <Button type="button" variant="outline" onClick={() => setOpen(false)}>
          {t("common.cancel")}
        </Button>
        <Button
          type="submit"
          disabled={
            !value.trim() ||
            !/^\d{3}(\.[0-9A-Za-z]+)*$/.test(code.trim()) ||
            save.isPending
          }
        >
          {t("common.save")}
        </Button>
      </div>
    </form>
  );
}
