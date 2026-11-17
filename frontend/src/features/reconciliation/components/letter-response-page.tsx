"use client";

import { useMutation, useQuery } from "@tanstack/react-query";
import { CheckCircle2, XCircle } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { isApiError } from "@/lib/api";
import { formatMoney } from "@/lib/decimal";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

import { type PublicLetter, publicLetterService } from "../service";

function Shell({ children }: { children: React.ReactNode }) {
  return (
    <main className="bg-background flex min-h-svh items-center justify-center p-4 sm:p-6">
      <Card className="w-full max-w-lg">{children}</Card>
    </main>
  );
}

function balanceText(
  t: (k: string, v?: Record<string, string | number>) => string,
  l: PublicLetter,
) {
  const n = Number(l.balance);
  if (n === 0)
    return t("reconciliation.balance_zero", { date: date(l.as_of_date) });
  return t(
    n > 0 ? "reconciliation.balance_debit" : "reconciliation.balance_credit",
    {
      date: date(l.as_of_date),
      amount: formatMoney(String(Math.abs(n)), l.currency),
    },
  );
}

/**
 * `/mutabakat/{token}` (#389): mutabakat mektubunun yanıt sayfası. Oturum
 * gerekmez; karşı firma bakiyeyi görür, "mutabıkız" ya da kendi bakiyesiyle
 * "mutabık değiliz" der. Bir kez yanıtlanır.
 */
export function LetterResponsePage({ token }: { token: string }) {
  const { t } = useLocale();
  const letter = useQuery({
    queryKey: ["public-reconciliation-letter", token],
    queryFn: () => publicLetterService.preview(token),
    retry: false,
  });
  const [mode, setMode] = useState<"agreed" | "disputed" | null>(null);
  const [balance, setBalance] = useState("");
  const [note, setNote] = useState("");
  const [name, setName] = useState("");
  const respond = useMutation({
    mutationFn: () =>
      publicLetterService.respond(token, {
        response: mode!,
        balance: balance.trim()
          ? balance.replace(/\./g, "").replace(",", ".")
          : null,
        note: note.trim() || null,
        responder_name: name.trim() || null,
      }),
    onSuccess: () => void letter.refetch(),
  });

  if (letter.isLoading) {
    return (
      <Shell>
        <CardContent className="space-y-3 p-6">
          <Skeleton className="h-6 w-2/3" />
          <Skeleton className="h-20 w-full" />
        </CardContent>
      </Shell>
    );
  }
  if (letter.isError || !letter.data) {
    const expired = isApiError(letter.error) && letter.error.status === 410;
    return (
      <Shell>
        <CardHeader>
          <CardTitle role="heading" aria-level={1}>
            {expired
              ? t("reconciliation.expired_title")
              : t("reconciliation.not_found_title")}
          </CardTitle>
          <CardDescription>
            {expired
              ? t("reconciliation.expired")
              : t("reconciliation.not_found")}
          </CardDescription>
        </CardHeader>
      </Shell>
    );
  }
  const l = letter.data;
  return (
    <Shell>
      <CardHeader>
        <CardTitle role="heading" aria-level={1}>{t("reconciliation.title")}</CardTitle>
        <CardDescription>
          {t("reconciliation.from", {
            taxpayer: l.taxpayer_title,
            counterparty: l.counterparty_name,
          })}
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        <p className="bg-surface-2 rounded-lg p-3 text-sm leading-relaxed">
          {balanceText(t, l)}
        </p>
        {l.note ? <p className="text-text-2 text-sm">{l.note}</p> : null}
        {l.response ? (
          <div className="flex items-start gap-2 text-sm" role="status">
            {l.response === "agreed" ? (
              <CheckCircle2 className="text-ok size-5 shrink-0" aria-hidden />
            ) : (
              <XCircle className="text-danger size-5 shrink-0" aria-hidden />
            )}
            <span>
              {l.response === "agreed"
                ? t("reconciliation.answered_agreed")
                : t("reconciliation.answered_disputed")}
              {l.responded_at ? ` (${date(l.responded_at)})` : ""}
            </span>
          </div>
        ) : (
          <form
            className="space-y-3"
            onSubmit={(e) => {
              e.preventDefault();
              if (mode) respond.mutate();
            }}
          >
            <fieldset className="grid gap-2 sm:grid-cols-2">
              <legend className="mb-1 text-sm font-medium">
                {t("reconciliation.question")}
              </legend>
              {(["agreed", "disputed"] as const).map((m) => (
                <label
                  key={m}
                  className="border-line has-[:checked]:border-brand has-[:checked]:bg-brand-tint flex min-h-11 cursor-pointer items-center gap-2 rounded-lg border px-3 text-sm"
                >
                  <input
                    type="radio"
                    name="response"
                    checked={mode === m}
                    onChange={() => setMode(m)}
                  />
                  {t(`reconciliation.choice_${m}`)}
                </label>
              ))}
            </fieldset>
            {mode === "disputed" ? (
              <label className="flex flex-col gap-1.5 text-sm">
                <span className="font-medium">
                  {t("reconciliation.your_balance")}
                </span>
                <Input
                  inputMode="decimal"
                  value={balance}
                  onChange={(e) => setBalance(e.target.value)}
                />
                <span className="text-text-3 text-xs">
                  {t("reconciliation.your_balance_hint")}
                </span>
              </label>
            ) : null}
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="font-medium">{t("reconciliation.name")}</span>
              <Input
                autoComplete="name"
                value={name}
                onChange={(e) => setName(e.target.value)}
              />
            </label>
            <label className="flex flex-col gap-1.5 text-sm">
              <span className="font-medium">{t("reconciliation.note")}</span>
              <Textarea
                rows={3}
                maxLength={2000}
                value={note}
                onChange={(e) => setNote(e.target.value)}
              />
            </label>
            {respond.isError ? (
              <p className="text-danger text-sm" role="alert">
                {isApiError(respond.error)
                  ? respond.error.message
                  : t("reconciliation.failed")}
              </p>
            ) : null}
            <Button
              type="submit"
              className="w-full max-md:min-h-11"
              disabled={!mode || respond.isPending}
            >
              {t("reconciliation.submit")}
            </Button>
          </form>
        )}
        <p className="text-text-3 text-xs">
          {t("reconciliation.footer", { office: l.office || "—" })}
        </p>
      </CardContent>
    </Shell>
  );
}
