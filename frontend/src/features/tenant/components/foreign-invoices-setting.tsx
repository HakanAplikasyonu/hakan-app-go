"use client";

import { useState } from "react";
import { toast } from "sonner";

import { EntitySectionCard } from "@/components/entity";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  useForeignServiceKinds,
  useSaveForeignServiceKinds,
  type ForeignInvoiceAccounts,
  type ForeignInvoiceTreatment,
  type ForeignServiceKindSetting,
} from "@/features/office/hooks/use-foreign-invoices";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

/** Fiş hesap kodları (#780 KDV2 B): alan ve etiket anahtarı. */
const ACCOUNT_FIELDS = [
  ["kdv2_account", "tenant.foreign_account_kdv2"],
  ["stopaj_account", "tenant.foreign_account_stopaj"],
  ["deductible_vat_account", "tenant.foreign_account_deductible"],
  ["counter_account", "tenant.foreign_account_counter"],
  ["rounding_income_account", "tenant.foreign_account_rounding_income"],
  ["rounding_expense_account", "tenant.foreign_account_rounding_expense"],
] as const satisfies readonly (readonly [
  keyof ForeignInvoiceAccounts,
  string,
])[];

/**
 * Ayarlar → Yabancı hizmet faturaları (#780 A1): mükellef sayfasındaki
 * "Yabancı faturalar / KDV2" sekmesi (varsayılan açık) ve işlem türlerinin
 * varsayılanları (yalnız KDV2 ya da KDV2 + stopaj, oran, brütleştirme).
 * Fatura eklerken işlem türü seçilince bu değerler dolar.
 */
export function ForeignInvoicesSetting({ canWrite }: { canWrite: boolean }) {
  const { t } = useLocale();
  const query = useForeignServiceKinds();
  const save = useSaveForeignServiceKinds();
  const [draft, setDraft] = useState<ForeignServiceKindSetting[] | null>(null);
  const [accDraft, setAccDraft] = useState<ForeignInvoiceAccounts | null>(null);
  const data = query.data;
  const accounts = accDraft ?? data?.accounts;
  const on = data?.enabled !== false;
  const rows = draft ?? data?.items ?? [];

  const edit = (kind: string, patch: Partial<ForeignServiceKindSetting>) =>
    setDraft(
      rows.map((r) => (r.service_kind === kind ? { ...r, ...patch } : r)),
    );

  const fail = (err: unknown) =>
    toast.error(isApiError(err) ? err.message : t("tenant.foreign_failed"));

  return (
    <EntitySectionCard title={t("tenant.foreign_title")}>
      <div className="space-y-4">
        <div className="flex items-start justify-between gap-4">
          <div className="space-y-1">
            <Label htmlFor="foreign-invoices-enabled">
              {t("tenant.foreign_label")}
            </Label>
            <p className="text-text-2 text-sm">
              {on ? t("tenant.foreign_on") : t("tenant.foreign_off")}
            </p>
          </div>
          <Switch
            id="foreign-invoices-enabled"
            checked={on}
            disabled={!canWrite || !data || save.isPending}
            onCheckedChange={(v) =>
              save.mutate({ enabled: v }, { onError: fail })
            }
          />
        </div>
        {on && rows.length > 0 ? (
          <div className="space-y-3">
            <p className="text-text-2 text-sm">
              {t("tenant.foreign_kinds_hint")}
            </p>
            <div className="overflow-x-auto">
              <Table data-testid="foreign-kinds">
                <TableCaption className="sr-only">
                  {t("tenant.foreign_kinds_title")}
                </TableCaption>
                <TableHeader>
                  <TableRow>
                    <TableHead scope="col">
                      {t("tenant.foreign_col_kind")}
                    </TableHead>
                    <TableHead scope="col">
                      {t("tenant.foreign_col_treatment")}
                    </TableHead>
                    <TableHead scope="col">
                      {t("tenant.foreign_col_stopaj")}
                    </TableHead>
                    <TableHead scope="col">
                      {t("tenant.foreign_col_gross_up")}
                    </TableHead>
                    <TableHead scope="col">
                      {t("tenant.foreign_col_vat")}
                    </TableHead>
                    <TableHead scope="col">
                      {t("tenant.foreign_col_expense")}
                    </TableHead>
                    <TableHead scope="col">
                      {t("tenant.foreign_col_enabled")}
                    </TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {rows.map((r) => {
                    const stopaj = r.treatment === "kdv2_stopaj";
                    return (
                      <TableRow key={r.service_kind}>
                        <TableCell className="font-medium">{r.label}</TableCell>
                        <TableCell>
                          <NativeSelect
                            aria-label={t("tenant.foreign_treatment_for", {
                              kind: r.label,
                            })}
                            size="sm"
                            disabled={!canWrite}
                            value={r.treatment}
                            onChange={(e) =>
                              edit(r.service_kind, {
                                treatment: e.target
                                  .value as ForeignInvoiceTreatment,
                              })
                            }
                          >
                            <NativeSelectOption value="kdv2">
                              {t("tenant.foreign_treatment_kdv2")}
                            </NativeSelectOption>
                            <NativeSelectOption value="kdv2_stopaj">
                              {t("tenant.foreign_treatment_kdv2_stopaj")}
                            </NativeSelectOption>
                          </NativeSelect>
                        </TableCell>
                        <TableCell>
                          <Input
                            aria-label={t("tenant.foreign_stopaj_for", {
                              kind: r.label,
                            })}
                            inputMode="decimal"
                            className="h-8 w-20"
                            disabled={!canWrite || !stopaj}
                            value={stopaj ? r.stopaj_rate : "0"}
                            onChange={(e) =>
                              edit(r.service_kind, {
                                stopaj_rate: e.target.value,
                              })
                            }
                          />
                        </TableCell>
                        <TableCell>
                          <Checkbox
                            aria-label={t("tenant.foreign_gross_up_for", {
                              kind: r.label,
                            })}
                            disabled={!canWrite || !stopaj}
                            checked={stopaj && r.gross_up}
                            onCheckedChange={(v) =>
                              edit(r.service_kind, { gross_up: v === true })
                            }
                          />
                        </TableCell>
                        <TableCell>
                          <Input
                            aria-label={t("tenant.foreign_vat_for", {
                              kind: r.label,
                            })}
                            inputMode="decimal"
                            className="h-8 w-20"
                            disabled={!canWrite}
                            value={r.vat_rate}
                            onChange={(e) =>
                              edit(r.service_kind, { vat_rate: e.target.value })
                            }
                          />
                        </TableCell>
                        <TableCell>
                          <Input
                            aria-label={t("tenant.foreign_expense_for", {
                              kind: r.label,
                            })}
                            className="h-8 w-24 font-mono"
                            disabled={!canWrite}
                            value={r.expense_account}
                            onChange={(e) =>
                              edit(r.service_kind, {
                                expense_account: e.target.value,
                              })
                            }
                          />
                        </TableCell>
                        <TableCell>
                          <Checkbox
                            aria-label={t("tenant.foreign_enabled_for", {
                              kind: r.label,
                            })}
                            disabled={!canWrite}
                            checked={r.enabled}
                            onCheckedChange={(v) =>
                              edit(r.service_kind, { enabled: v === true })
                            }
                          />
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            </div>
            {canWrite ? (
              <div className="flex justify-end">
                <Button
                  size="sm"
                  disabled={!draft || save.isPending}
                  onClick={() =>
                    save.mutate(
                      {
                        enabled: on,
                        items: rows.map((r) => {
                          const stopaj = r.treatment === "kdv2_stopaj";
                          return {
                            service_kind: r.service_kind,
                            treatment: r.treatment,
                            stopaj_rate: stopaj ? r.stopaj_rate : "0",
                            gross_up: stopaj && r.gross_up,
                            vat_rate: r.vat_rate,
                            enabled: r.enabled,
                            expense_account: r.expense_account.trim(),
                          };
                        }),
                      },
                      {
                        onSuccess: () => {
                          setDraft(null);
                          toast.success(t("tenant.foreign_saved"));
                        },
                        onError: fail,
                      },
                    )
                  }
                >
                  {t("tenant.foreign_save")}
                </Button>
              </div>
            ) : null}
          </div>
        ) : null}
        {on && accounts ? (
          <div className="space-y-3" data-testid="foreign-accounts">
            <div className="space-y-1">
              <h3 className="text-sm font-semibold">
                {t("tenant.foreign_accounts_title")}
              </h3>
              <p className="text-text-2 text-sm">
                {t("tenant.foreign_accounts_hint")}
              </p>
            </div>
            <div className="grid gap-3 sm:grid-cols-2">
              {ACCOUNT_FIELDS.map(([key, label]) => (
                <div key={key} className="space-y-1">
                  <Label htmlFor={`foreign-account-${key}`}>{t(label)}</Label>
                  <Input
                    id={`foreign-account-${key}`}
                    className="h-8 font-mono"
                    disabled={!canWrite}
                    value={accounts[key]}
                    onChange={(e) =>
                      setAccDraft({ ...accounts, [key]: e.target.value })
                    }
                  />
                </div>
              ))}
            </div>
            {canWrite ? (
              <div className="flex justify-end">
                <Button
                  size="sm"
                  disabled={!accDraft || save.isPending}
                  onClick={() =>
                    save.mutate(
                      { enabled: on, accounts },
                      {
                        onSuccess: () => {
                          setAccDraft(null);
                          toast.success(t("tenant.foreign_accounts_saved"));
                        },
                        onError: fail,
                      },
                    )
                  }
                >
                  {t("tenant.foreign_accounts_save")}
                </Button>
              </div>
            ) : null}
          </div>
        ) : null}
      </div>
    </EntitySectionCard>
  );
}
