"use client";

import { useMutation } from "@tanstack/react-query";
import { Pencil, Trash2 } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  type ExpenseCategory,
  receiptsService,
} from "../services/receipts.service";

type Scope = "taxpayer" | "office";

type Form = {
  code: string;
  name: string;
  account_code: string;
  vat_account_code: string;
  deductible: "yes" | "no";
  keywords: string;
  scope: Scope;
};

const empty: Form = {
  code: "",
  name: "",
  account_code: "770",
  vat_account_code: "191",
  deductible: "yes",
  keywords: "",
  scope: "taxpayer",
};

const scopeClass: Record<string, string> = {
  platform: "bg-surface-3 text-text-2",
  office: "bg-brand-tint text-brand-text",
  taxpayer: "bg-ok-tint text-ok",
};

/**
 * Masraf kategorileri (#384): platform varsayılanı → ofis şablonu →
 * mükellefe özel. Aynı kodla yazılan kategori alttakini ezer; silinince
 * alttaki geri gelir.
 */
export function ExpenseCategoriesDialog({
  open,
  onOpenChange,
  engagement,
  categories,
  canEdit,
  onChanged,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  engagement: string;
  categories: ExpenseCategory[];
  canEdit: boolean;
  onChanged: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [form, setForm] = useState<Form>(empty);
  const fail = (e: unknown) =>
    toast.error(isApiError(e) ? e.message : t("office.receipts.save_failed"));

  const save = useMutation({
    mutationFn: (f: Form) =>
      receiptsService.saveCategory(
        slug,
        f.scope === "taxpayer" ? engagement : null,
        {
          code: f.code.trim(),
          name: f.name.trim(),
          account_code: f.account_code.trim(),
          vat_account_code: f.vat_account_code.trim() || "191",
          deductible: f.deductible,
          is_active: true,
          keywords: f.keywords
            .split(",")
            .map((k) => k.trim())
            .filter(Boolean),
        },
      ),
    onSuccess: () => {
      toast.success(t("office.expenses.category_saved"));
      setForm(empty);
      onChanged();
    },
    onError: fail,
  });
  const remove = useMutation({
    mutationFn: (c: ExpenseCategory) =>
      receiptsService.deleteCategory(
        slug,
        c.scope === "taxpayer" ? engagement : null,
        c.uuid ?? "",
      ),
    onSuccess: () => {
      toast.success(t("office.expenses.category_deleted"));
      onChanged();
    },
    onError: fail,
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-3xl">
        <DialogHeader>
          <DialogTitle>{t("office.expenses.categories")}</DialogTitle>
          <DialogDescription>
            {t("office.expenses.categories_hint")}
          </DialogDescription>
        </DialogHeader>
        <ul className="divide-line divide-y rounded-lg border">
          {categories.map((c) => (
            <li
              key={c.code}
              className={cn(
                "flex flex-wrap items-center gap-2 px-3 py-2 text-sm",
                !c.is_active && "opacity-60",
              )}
            >
              <span className="min-w-0 flex-1">
                <span className="font-medium">{c.name}</span>
                <span className="text-text-3 ms-2 font-mono text-xs">
                  {c.code}
                </span>
                <span className="text-text-2 block text-xs">
                  {c.account_code}
                  {c.deductible === "no"
                    ? ` · ${t("office.expenses.vat_not_deductible")}`
                    : ` · KDV ${c.vat_account_code}`}
                  {c.keywords.length ? ` · ${c.keywords.join(", ")}` : ""}
                </span>
              </span>
              <span
                className={cn(
                  "rounded px-1.5 py-0.5 text-[11px]",
                  scopeClass[c.scope],
                )}
              >
                {t(`office.expenses.scope_${c.scope}`)}
              </span>
              {canEdit ? (
                <>
                  <Button
                    size="sm"
                    variant="ghost"
                    className="max-md:min-h-11"
                    aria-label={t("office.expenses.category_edit", {
                      name: c.name,
                    })}
                    onClick={() =>
                      setForm({
                        code: c.code,
                        name: c.name,
                        account_code: c.account_code,
                        vat_account_code: c.vat_account_code,
                        deductible: c.deductible,
                        keywords: c.keywords.join(", "),
                        scope: c.scope === "office" ? "office" : "taxpayer",
                      })
                    }
                  >
                    <Pencil className="size-4" aria-hidden />
                  </Button>
                  {c.scope !== "platform" && c.uuid ? (
                    <Button
                      size="sm"
                      variant="ghost"
                      className="max-md:min-h-11"
                      aria-label={t("office.expenses.category_delete", {
                        name: c.name,
                      })}
                      disabled={remove.isPending}
                      onClick={() => remove.mutate(c)}
                    >
                      <Trash2 className="size-4" aria-hidden />
                    </Button>
                  ) : null}
                </>
              ) : null}
            </li>
          ))}
        </ul>
        {canEdit ? (
          <form
            className="grid gap-3 sm:grid-cols-2"
            onSubmit={(e) => {
              e.preventDefault();
              save.mutate(form);
            }}
          >
            <Label className="flex flex-col gap-1 text-xs leading-normal font-normal">
              <span className="text-text-2">
                {t("office.expenses.category_code")}
              </span>
              <Input
                required
                pattern="[a-z0-9_]{1,48}"
                value={form.code}
                onChange={(e) =>
                  setForm({ ...form, code: e.target.value.toLowerCase() })
                }
              />
            </Label>
            <Label className="flex flex-col gap-1 text-xs leading-normal font-normal">
              <span className="text-text-2">
                {t("office.expenses.category_name")}
              </span>
              <Input
                required
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
              />
            </Label>
            <Label className="flex flex-col gap-1 text-xs leading-normal font-normal">
              <span className="text-text-2">
                {t("office.expenses.col_account")}
              </span>
              <Input
                required
                className="font-mono"
                value={form.account_code}
                onChange={(e) =>
                  setForm({ ...form, account_code: e.target.value })
                }
              />
            </Label>
            <Label className="flex flex-col gap-1 text-xs leading-normal font-normal">
              <span className="text-text-2">
                {t("office.expenses.category_keywords")}
              </span>
              <Input
                value={form.keywords}
                onChange={(e) => setForm({ ...form, keywords: e.target.value })}
              />
            </Label>
            <Label className="flex items-center gap-2 text-sm leading-normal font-normal">
              <Checkbox
                checked={form.deductible === "no"}
                onCheckedChange={(v) =>
                  setForm({
                    ...form,
                    deductible: v === true ? "no" : "yes",
                  })
                }
              />
              {t("office.expenses.vat_not_deductible")}
            </Label>
            <div className="flex items-center gap-2 text-xs">
              <Label
                htmlFor="expense-category-scope"
                className="text-text-2 text-xs leading-normal font-normal"
              >
                {t("office.expenses.category_scope")}
              </Label>
              <NativeSelect
                id="expense-category-scope"
                className="pl-2 shadow-none"
                value={form.scope}
                onChange={(e) =>
                  setForm({ ...form, scope: e.target.value as Scope })
                }
              >
                <NativeSelectOption value="taxpayer">
                  {t("office.expenses.scope_taxpayer")}
                </NativeSelectOption>
                <NativeSelectOption value="office">
                  {t("office.expenses.scope_office")}
                </NativeSelectOption>
              </NativeSelect>
            </div>
            <div className="flex justify-end gap-2 sm:col-span-2">
              <Button
                type="button"
                variant="ghost"
                onClick={() => setForm(empty)}
              >
                {t("office.expenses.category_clear")}
              </Button>
              <Button type="submit" disabled={save.isPending}>
                {t("office.expenses.category_save")}
              </Button>
            </div>
          </form>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
