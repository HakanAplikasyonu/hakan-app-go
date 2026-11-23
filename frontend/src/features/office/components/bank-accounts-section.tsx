"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Landmark, Plus } from "lucide-react";
import { useState } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
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
import { Skeleton } from "@/components/ui/skeleton";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  bankPostingService,
  type CashBox,
} from "../services/bank-posting.service";
import {
  BANKS,
  bankFromIban,
  compactIban,
  formatIban,
  isValidTrIban,
} from "./iban";

const CURRENCIES = ["TRY", "USD", "EUR"] as const;

/** Hesap formunun değerleri (ekleme ve düzenleme). */
type Draft = {
  bank: string;
  branch: string;
  iban: string;
  name: string;
  currency: string;
};

const EMPTY: Draft = {
  bank: "",
  branch: "",
  iban: "",
  name: "",
  currency: "TRY",
};

function draftOf(b: CashBox): Draft {
  return {
    bank: b.bank_id ?? "",
    branch: b.bank_branch_code ?? "",
    iban: b.iban ? formatIban(b.iban) : "",
    name: b.name,
    currency: b.currency,
  };
}

/** Ekranda gösterilecek hata: sunucunun alanı ya da IBAN/banka denetimi. */
function validate(
  d: Draft,
  t: (k: string, v?: Record<string, string>) => string,
): string | null {
  if (!d.bank) return t("office.bank.accounts.error_bank");
  if (!isValidTrIban(d.iban)) return t("office.bank.accounts.error_iban");
  const fromIban = bankFromIban(d.iban);
  if (fromIban && fromIban !== d.bank)
    return t("office.bank.accounts.error_bank_mismatch", {
      bank: t(`office.bank.banks.${fromIban}`),
    });
  return null;
}

/**
 * Mükellefin banka hesapları (#780): müşavir mükellef sayfasından hesap
 * ekler (banka, şube, IBAN, hesap adı, para birimi), düzenler ve pasife
 * alır. Ekstre içe aktarma dosyadaki IBAN'la bu hesabı kendiliğinden seçer.
 * Hesap silinmez: hareketleri ve fişleri ona bağlıdır.
 */
export function BankAccountsSection({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canRead = can("tenant.bank.read");
  const canWrite = can("tenant.bank.write");
  const qc = useQueryClient();
  const key = ["tenant", slug.toLowerCase(), "cash-boxes", engagement];
  const boxes = useQuery({
    queryKey: key,
    queryFn: () => bankPostingService.cashBoxes(slug, engagement),
    enabled: canRead,
  });
  // null: form kapalı; "new": yeni hesap; uuid: düzenlenen hesap.
  const [editing, setEditing] = useState<string | null>(null);
  const refresh = () => void qc.invalidateQueries({ queryKey: key });

  const toggle = useMutation({
    mutationFn: (b: CashBox) =>
      bankPostingService.updateCashBox(slug, engagement, b.uuid, {
        active: !b.active,
      }),
    onSuccess: (b) => {
      toast.success(
        t(
          b.active
            ? "office.bank.accounts.reopened"
            : "office.bank.accounts.deactivated",
        ),
      );
      refresh();
    },
    onError: () => toast.error(t("office.bank.accounts.failed")),
  });

  const items = (boxes.data?.items ?? []).filter(
    (b) => b.type === "bank_account",
  );

  if (!canRead) return null;

  return (
    <section
      aria-labelledby="bank-accounts-title"
      className="bg-surface border-line rounded-xl border p-4 md:p-5"
      data-testid="bank-accounts"
    >
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <div>
          <h2
            id="bank-accounts-title"
            className="font-display text-base font-bold"
          >
            {t("office.bank.accounts.title")}
          </h2>
          <p className="text-text-2 text-sm">
            {t("office.bank.accounts.description")}
          </p>
        </div>
        {canWrite && editing === null ? (
          <Button variant="outline" onClick={() => setEditing("new")}>
            <Plus className="size-4" aria-hidden />
            {t("office.bank.accounts.add")}
          </Button>
        ) : null}
      </div>

      {editing === "new" ? (
        <AccountForm
          engagement={engagement}
          initial={EMPTY}
          onDone={() => {
            setEditing(null);
            refresh();
          }}
          onCancel={() => setEditing(null)}
        />
      ) : null}

      {boxes.isLoading ? (
        <Skeleton className="h-16 w-full" />
      ) : boxes.isError ? (
        <ErrorState
          title={t("office.api.load_error")}
          onRetry={() => void boxes.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : items.length === 0 ? (
        <p className="text-text-2 py-3 text-sm">
          {t("office.bank.accounts.empty")}
        </p>
      ) : (
        <ul className="divide-line divide-y">
          {items.map((b) =>
            editing === b.uuid ? (
              <li key={b.uuid} className="py-3">
                <AccountForm
                  engagement={engagement}
                  box={b}
                  initial={draftOf(b)}
                  onDone={() => {
                    setEditing(null);
                    refresh();
                  }}
                  onCancel={() => setEditing(null)}
                />
              </li>
            ) : (
              <li
                key={b.uuid}
                className="flex flex-wrap items-center gap-3 py-2.5"
                data-testid="bank-account-row"
              >
                <Landmark className="text-text-3 size-4" aria-hidden />
                <span className="min-w-0 flex-1 text-sm">
                  <span className="block truncate font-semibold">{b.name}</span>
                  <span className="text-text-2 block truncate text-xs">
                    {b.bank_id ? t(`office.bank.banks.${b.bank_id}`) : "—"}
                    {b.bank_branch_code
                      ? ` · ${t("office.bank.accounts.branch_short", { branch: b.bank_branch_code })}`
                      : null}
                    {" · "}
                    <span className="figure">
                      {b.iban
                        ? formatIban(b.iban)
                        : (b.bank_account_number ?? "")}
                    </span>
                    {` · ${b.currency}`}
                  </span>
                </span>
                <span
                  className={cn(
                    "rounded-md px-2 py-0.5 text-xs font-semibold whitespace-nowrap",
                    b.active
                      ? "bg-brand-tint text-brand-text"
                      : "bg-surface-2 text-text-3",
                  )}
                >
                  {t(
                    b.active
                      ? "office.bank.accounts.active"
                      : "office.bank.accounts.inactive",
                  )}
                </span>
                {b.web_service ? (
                  <span className="text-text-3 text-xs">
                    {t("office.bank.accounts.web_service")}
                  </span>
                ) : null}
                {canWrite ? (
                  <span className="flex gap-1">
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={editing !== null}
                      onClick={() => setEditing(b.uuid)}
                    >
                      {t("office.bank.accounts.edit")}
                    </Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      disabled={toggle.isPending}
                      onClick={() => toggle.mutate(b)}
                    >
                      {t(
                        b.active
                          ? "office.bank.accounts.deactivate"
                          : "office.bank.accounts.reopen",
                      )}
                    </Button>
                  </span>
                ) : null}
              </li>
            ),
          )}
        </ul>
      )}
    </section>
  );
}

function AccountForm({
  engagement,
  box,
  initial,
  onDone,
  onCancel,
}: {
  engagement: string;
  box?: CashBox;
  initial: Draft;
  onDone: () => void;
  onCancel: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [d, setD] = useState<Draft>(initial);
  const [touched, setTouched] = useState(false);
  const set = (k: keyof Draft, v: string) => setD((x) => ({ ...x, [k]: v }));
  const problem = validate(d, t);
  // Banka kodu web servis hesabında değişmez (sunucu da reddeder).
  const locked = !!box?.web_service;

  const save = useMutation({
    mutationFn: () => {
      const body = {
        bank_id: d.bank,
        iban: compactIban(d.iban),
        bank_branch_code: d.branch.trim(),
        currency: d.currency,
      };
      if (box)
        return bankPostingService.updateCashBox(slug, engagement, box.uuid, {
          ...body,
          ...(d.name.trim() ? { name: d.name.trim() } : {}),
        });
      return bankPostingService.createCashBox(slug, engagement, {
        ...body,
        type: "bank_account",
        name: d.name.trim() || undefined,
      });
    },
    onSuccess: () => {
      toast.success(
        t(box ? "office.bank.accounts.updated" : "office.bank.accounts.added"),
      );
      onDone();
    },
    onError: (e) => {
      const code = isApiError(e) ? e.code : "";
      toast.error(
        code === "CASH_BOX_EXISTS"
          ? t("office.bank.accounts.error_exists")
          : code === "INVALID_CASH_BOX"
            ? t("office.bank.accounts.error_invalid")
            : t("office.bank.accounts.failed"),
      );
    },
  });

  const id = box?.uuid ?? "new";
  return (
    <form
      className="bg-surface-2 border-line mb-3 grid gap-3 rounded-xl border p-4 sm:grid-cols-2 lg:grid-cols-3"
      aria-label={t(
        box ? "office.bank.accounts.edit_title" : "office.bank.accounts.add",
      )}
      onSubmit={(e) => {
        e.preventDefault();
        setTouched(true);
        if (!problem) save.mutate();
      }}
    >
      <div className="space-y-1.5">
        <Label htmlFor={`acc-bank-${id}`}>
          {t("office.bank.accounts.bank")}
        </Label>
        <Select
          value={d.bank}
          disabled={locked}
          onValueChange={(v) => set("bank", v)}
        >
          <SelectTrigger
            id={`acc-bank-${id}`}
            aria-label={t("office.bank.accounts.bank")}
          >
            <SelectValue placeholder={t("office.bank.accounts.bank_pick")} />
          </SelectTrigger>
          <SelectContent>
            {BANKS.map((b) => (
              <SelectItem key={b} value={b}>
                {t(`office.bank.banks.${b}`)}
              </SelectItem>
            ))}
            {/* Listede olmayan, web servisin açtığı bir bankanın hesabı. */}
            {d.bank && !(BANKS as readonly string[]).includes(d.bank) ? (
              <SelectItem value={d.bank}>
                {t(`office.bank.banks.${d.bank}`)}
              </SelectItem>
            ) : null}
          </SelectContent>
        </Select>
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`acc-branch-${id}`}>
          {t("office.bank.accounts.branch")}
        </Label>
        <Input
          id={`acc-branch-${id}`}
          value={d.branch}
          maxLength={32}
          placeholder={t("office.bank.accounts.optional")}
          onChange={(e) => set("branch", e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`acc-iban-${id}`}>
          {t("office.bank.accounts.iban")}
        </Label>
        <Input
          id={`acc-iban-${id}`}
          className="figure"
          value={d.iban}
          disabled={locked}
          placeholder="TR00 0000 0000 0000 0000 0000 00"
          aria-invalid={touched && !!problem}
          onChange={(e) => {
            const v = e.target.value;
            setD((x) => {
              // IBAN'dan banka kendiliğinden seçilir.
              const fromIban = bankFromIban(v);
              return { ...x, iban: v, bank: fromIban ?? x.bank };
            });
          }}
          onBlur={() => {
            setTouched(true);
            if (isValidTrIban(d.iban)) set("iban", formatIban(d.iban));
          }}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`acc-name-${id}`}>
          {t("office.bank.accounts.name")}
        </Label>
        <Input
          id={`acc-name-${id}`}
          value={d.name}
          maxLength={255}
          placeholder={t("office.bank.accounts.name_hint")}
          onChange={(e) => set("name", e.target.value)}
        />
      </div>
      <div className="space-y-1.5">
        <Label htmlFor={`acc-cur-${id}`}>
          {t("office.bank.accounts.currency")}
        </Label>
        <Select
          value={d.currency}
          disabled={locked}
          onValueChange={(v) => set("currency", v)}
        >
          <SelectTrigger
            id={`acc-cur-${id}`}
            aria-label={t("office.bank.accounts.currency")}
          >
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {[
              ...CURRENCIES,
              ...((CURRENCIES as readonly string[]).includes(d.currency)
                ? []
                : [d.currency]),
            ].map((c) => (
              <SelectItem key={c} value={c}>
                {(CURRENCIES as readonly string[]).includes(c)
                  ? t(`office.bank.accounts.currency_${c}`)
                  : c}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      {touched && problem ? (
        <p
          role="alert"
          className="text-danger text-sm sm:col-span-2 lg:col-span-3"
        >
          {problem}
        </p>
      ) : null}
      <div className="flex justify-end gap-2 sm:col-span-2 lg:col-span-3">
        <Button type="button" variant="outline" onClick={onCancel}>
          {t("common.cancel")}
        </Button>
        <Button type="submit" disabled={save.isPending}>
          {t("common.save")}
        </Button>
      </div>
    </form>
  );
}
