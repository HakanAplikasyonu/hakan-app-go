"use client";

import { Plus } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { FieldLegend, FieldSet } from "@/components/ui/field";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useDebounce } from "@/hooks/use-debounce";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

import {
  useAccountMemoryActions,
  useRulePreview,
} from "../hooks/use-account-memory";
import { ACCOUNT_CODE_RE } from "../hooks/use-review";
import type { AccountRuleInput } from "../services/account-memory.service";
import { Figure } from "./office-ui";

type Draft = {
  name: string;
  taxId: string;
  nameContains: string;
  itemContains: string;
  direction: "" | "incoming" | "outgoing";
  taxRate: string;
  code: string;
};

const EMPTY: Draft = {
  name: "",
  taxId: "",
  nameContains: "",
  itemContains: "",
  direction: "",
  taxRate: "",
  code: "",
};

/** Formdan API girdisi; kod geçersizse null (önizleme ve kayıt beklemede). */
function toInput(d: Draft): AccountRuleInput | null {
  const code = d.code.trim();
  if (!ACCOUNT_CODE_RE.test(code)) return null;
  const items = d.itemContains
    .split(",")
    .map((x) => x.trim())
    .filter(Boolean);
  const conditions: NonNullable<AccountRuleInput["conditions"]> = {};
  if (d.taxId.trim()) conditions.counterparty_tax_id = d.taxId.trim();
  if (d.nameContains.trim())
    conditions.counterparty_name_contains = d.nameContains.trim();
  if (items.length) conditions.item_name_contains = items;
  if (d.direction) conditions.direction = d.direction;
  if (d.taxRate.trim()) conditions.tax_rate = d.taxRate.trim();
  return {
    role: "line",
    priority: 100,
    account_code: code,
    name: d.name.trim() || undefined,
    conditions,
  };
}

function Field({
  id,
  label,
  hint,
  children,
}: {
  id: string;
  label: string;
  hint?: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-1.5">
      <Label htmlFor={id}>{label}</Label>
      {children}
      {hint ? <p className="text-text-2 text-xs">{hint}</p> : null}
    </div>
  );
}

/**
 * Kural editörü (OfisMizan): koşullar, hesap kodu ve "kaydetmeden önce"
 * önizlemesi — kural son 6 ayda kaç belgeye uyardı, kaçı zaten bu kodla,
 * kaçı başka kodla onaylanmış (#333 preview ucu).
 */
export function RuleEditor({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [open, setOpen] = useState(false);
  const [draft, setDraft] = useState<Draft>(EMPTY);
  const input = toInput(draft);
  const key = useDebounce(JSON.stringify(input), 300);
  const debounced = JSON.parse(key) as AccountRuleInput | null;
  const preview = useRulePreview(slug, engagement, debounced);
  const { createRule } = useAccountMemoryActions(slug, engagement);
  const set =
    (k: keyof Draft) =>
    (e: React.ChangeEvent<HTMLInputElement | HTMLSelectElement>) =>
      setDraft((d) => ({ ...d, [k]: e.target.value }));
  const p = preview.data;
  const codeInvalid = draft.code.trim() !== "" && !input;

  return (
    <Sheet
      open={open}
      onOpenChange={(v) => {
        setOpen(v);
        if (!v) setDraft(EMPTY);
      }}
    >
      <Button
        className="bg-brand text-on-brand hover:bg-brand/90 pointer-coarse:h-11"
        size="sm"
        onClick={() => setOpen(true)}
      >
        <Plus className="size-4" />
        {t("office.rules.new")}
      </Button>
      <SheetContent className="flex w-full flex-col gap-0 overflow-y-auto p-0 sm:max-w-[460px]">
        <SheetHeader className="border-b p-5">
          <SheetTitle>{t("office.rules.new_title")}</SheetTitle>
          <SheetDescription>
            {t("office.rules.new_description")}
          </SheetDescription>
        </SheetHeader>
        <form
          className="flex flex-1 flex-col gap-5 p-5"
          onSubmit={(e) => {
            e.preventDefault();
            if (!input) return;
            createRule.mutate(input, {
              onSuccess: () => {
                toast.success(
                  t("office.rules.saved", { code: input.account_code }),
                );
                setOpen(false);
                setDraft(EMPTY);
              },
              onError: (err) =>
                toast.error(
                  isApiError(err) ? err.message : t("office.rules.save_failed"),
                ),
            });
          }}
        >
          <FieldSet className="gap-4">
            <FieldLegend className="text-muted-foreground mb-2 font-mono text-[11px] font-semibold tracking-[0.08em] uppercase">
              {t("office.rules.if")}
            </FieldLegend>
            <Field id="rule-taxid" label={t("office.rules.cond_tax_id")}>
              <Input
                id="rule-taxid"
                inputMode="numeric"
                className="font-mono"
                value={draft.taxId}
                onChange={set("taxId")}
              />
            </Field>
            <Field id="rule-cpname" label={t("office.rules.cond_name")}>
              <Input
                id="rule-cpname"
                value={draft.nameContains}
                onChange={set("nameContains")}
              />
            </Field>
            <Field
              id="rule-items"
              label={t("office.rules.cond_items")}
              hint={t("office.rules.cond_items_hint")}
            >
              <Input
                id="rule-items"
                value={draft.itemContains}
                onChange={set("itemContains")}
              />
            </Field>
            <div className="grid grid-cols-2 gap-3">
              <Field id="rule-dir" label={t("office.rules.cond_direction")}>
                <NativeSelect
                  id="rule-dir"
                  wrapperClassName="w-full"
                  className="h-10"
                  value={draft.direction}
                  onChange={set("direction")}
                >
                  <NativeSelectOption value="">
                    {t("office.filter_all")}
                  </NativeSelectOption>
                  <NativeSelectOption value="incoming">
                    {t("office.review_api.direction_incoming")}
                  </NativeSelectOption>
                  <NativeSelectOption value="outgoing">
                    {t("office.review_api.direction_outgoing")}
                  </NativeSelectOption>
                </NativeSelect>
              </Field>
              <Field id="rule-tax" label={t("office.rules.cond_tax_rate")}>
                <Input
                  id="rule-tax"
                  inputMode="decimal"
                  className="font-mono"
                  value={draft.taxRate}
                  onChange={set("taxRate")}
                />
              </Field>
            </div>
          </FieldSet>
          <FieldSet className="gap-4">
            <FieldLegend className="text-muted-foreground mb-2 font-mono text-[11px] font-semibold tracking-[0.08em] uppercase">
              {t("office.rules.then")}
            </FieldLegend>
            <Field
              id="rule-code"
              label={t("office.rules.code")}
              hint={codeInvalid ? t("office.rules.code_invalid") : undefined}
            >
              <Input
                id="rule-code"
                required
                className="font-mono"
                placeholder="770.01"
                aria-invalid={codeInvalid || undefined}
                value={draft.code}
                onChange={set("code")}
              />
            </Field>
            <Field id="rule-name" label={t("office.rules.name")}>
              <Input id="rule-name" value={draft.name} onChange={set("name")} />
            </Field>
          </FieldSet>

          <section
            aria-labelledby="rule-preview-title"
            aria-live="polite"
            className="bg-surface-2 rounded-lg border p-4 text-sm"
          >
            <h3 id="rule-preview-title" className="mb-2 font-semibold">
              {t("office.rules.preview_title")}
            </h3>
            {!input ? (
              <p className="text-text-2">
                {t("office.rules.preview_need_code")}
              </p>
            ) : preview.isError ? (
              <p className="text-text-2">{t("office.rules.preview_error")}</p>
            ) : !p ? (
              <Skeleton className="h-12 w-full" />
            ) : (
              <div className="flex flex-col gap-2">
                <p>
                  <Figure className="font-semibold">{p.documents}</Figure>{" "}
                  {t("office.rules.preview_documents", { months: p.months })}
                </p>
                <p>
                  <Figure className="font-semibold">
                    {p.approved - p.approved_different}
                  </Figure>{" "}
                  {t("office.rules.preview_same", { code: input.account_code })}
                </p>
                <p className={p.approved_different ? "text-danger" : ""}>
                  <Figure className="font-semibold">
                    {p.approved_different}
                  </Figure>{" "}
                  {t("office.rules.preview_different")}
                </p>
                {p.truncated ? (
                  <p className="text-text-2 text-xs">
                    {t("office.rules.preview_truncated")}
                  </p>
                ) : null}
                {p.samples.length ? (
                  <ul className="mt-1 flex flex-col gap-1 border-t pt-2 text-xs">
                    {p.samples.slice(0, 5).map((x) => (
                      <li
                        key={`${x.invoice_uuid}-${x.item_name ?? ""}`}
                        className="flex gap-2"
                      >
                        <Link
                          href={routes.tenant.invoice(
                            slug,
                            x.invoice_uuid,
                            engagement,
                          )}
                          className="text-brand-text font-mono hover:underline"
                        >
                          {x.invoice_number}
                        </Link>
                        <span className="text-text-2 min-w-0 flex-1 truncate">
                          {x.item_name}
                        </span>
                        <Figure
                          className={
                            x.approved_code !== input.account_code
                              ? "text-danger font-semibold"
                              : ""
                          }
                        >
                          {x.approved_code}
                        </Figure>
                      </li>
                    ))}
                  </ul>
                ) : null}
              </div>
            )}
          </section>

          <div className="mt-auto flex justify-end gap-2 border-t pt-4">
            <Button
              type="button"
              variant="ghost"
              className="h-11"
              onClick={() => setOpen(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button
              type="submit"
              className="bg-brand text-on-brand hover:bg-brand/90 h-11"
              disabled={!input || createRule.isPending}
            >
              {t("office.rules.save")}
            </Button>
          </div>
        </form>
      </SheetContent>
    </Sheet>
  );
}
