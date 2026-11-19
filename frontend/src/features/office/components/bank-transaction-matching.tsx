"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link2, Link2Off } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  bankMatchingKeys,
  bankMatchingService,
  type BankTransactionMatching,
} from "../services/bank-matching.service";
import { relatedKeys } from "../services/related.service";
import { warningKeys } from "../services/warnings.service";
import { errText } from "./engagement-expenses-page";
import { Figure, Money } from "./office-ui";

function trDate(iso: string) {
  const [y, m, d] = iso.split("-");
  return y && m && d ? `${d}.${m}.${y}` : iso;
}

/**
 * Seçili banka hareketinin ayrıntısı ve fatura eşleştirmesi (#528):
 * uyarıdan gelen kullanıcı hareketi görür, puanlı aday faturalardan
 * seçip eşleştirir ya da eşleşmeyi geri alır; dönemin banka uyarıları
 * hemen yenilenir.
 */
export function BankTransactionMatchingPanel({
  engagement,
  transaction,
  onChanged,
}: {
  engagement: string;
  transaction: string;
  /** Eşleşme değişince (beyan ekranı özetini yeniler). */
  onChanged?: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can("tenant.bank.write");
  const qc = useQueryClient();
  const key = bankMatchingKeys.one(slug, engagement, transaction);
  const [picked, setPicked] = useState<string[]>([]);

  const q = useQuery({
    queryKey: key,
    queryFn: () => bankMatchingService.get(slug, engagement, transaction),
  });

  const settle = (v: BankTransactionMatching, message: string) => {
    qc.setQueryData(key, v);
    setPicked([]);
    void qc.invalidateQueries({ queryKey: warningKeys.all(slug) });
    void qc.invalidateQueries({
      queryKey: relatedKeys.one(
        slug,
        engagement,
        "bank_transaction",
        transaction,
      ),
    });
    void qc.invalidateQueries({
      queryKey: ["tenant", slug.toLowerCase(), "bank-transactions"],
    });
    toast.success(message);
    onChanged?.();
  };
  const match = useMutation({
    mutationFn: (ids: string[]) =>
      bankMatchingService.match(slug, engagement, transaction, ids),
    onSuccess: (v) => settle(v, t("office.bank.matching.matched")),
    onError: (e) => toast.error(errText(e, t("office.bank.matching.failed"))),
  });
  const unmatch = useMutation({
    mutationFn: () =>
      bankMatchingService.unmatch(slug, engagement, transaction),
    onSuccess: (v) => settle(v, t("office.bank.matching.unmatched")),
    onError: (e) => toast.error(errText(e, t("office.bank.matching.failed"))),
  });
  const busy = match.isPending || unmatch.isPending;

  if (q.isPending) return <Skeleton className="h-32 w-full" />;
  if (q.isError || !q.data)
    return (
      <p className="text-muted-foreground text-sm">
        {t("office.bank.matching.load_failed")}
      </p>
    );
  const v = q.data;
  const invoiceHref = (id: string) =>
    routes.tenant.engagementInvoice(slug, engagement, id);
  const toggle = (id: string, on: boolean) =>
    setPicked((p) => (on ? [...p, id] : p.filter((x) => x !== id)));

  return (
    <div className="space-y-4" data-testid="bank-transaction-matching">
      <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-2 lg:grid-cols-4">
        <div>
          <dt className="text-muted-foreground">
            {t("office.bank.matching.date")}
          </dt>
          <dd>
            <Figure>{trDate(v.transaction_date)}</Figure>
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">
            {t("office.bank.matching.amount")}
          </dt>
          <dd>
            {v.debit_credit === "A" ? "+" : "−"}
            <Money value={v.amount} currency={v.currency} />
          </dd>
        </div>
        <div>
          <dt className="text-muted-foreground">
            {t("office.bank.matching.counterparty")}
          </dt>
          <dd>{v.counterparty_name ?? "—"}</dd>
        </div>
        <div>
          <dt className="text-muted-foreground">
            {t("office.bank.matching.status")}
          </dt>
          <dd>
            <Badge
              variant={v.match_status === "matched" ? "default" : "outline"}
            >
              {t(`office.bank.matching.status_${v.match_status}`)}
            </Badge>
          </dd>
        </div>
        <div className="sm:col-span-2 lg:col-span-4">
          <dt className="text-muted-foreground">
            {t("office.bank.matching.description")}
          </dt>
          <dd className="break-words">{v.description || "—"}</dd>
        </div>
      </dl>

      {v.matches.length > 0 && (
        <section className="space-y-2">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium">
              {t("office.bank.matching.matches_title")}
            </h3>
            {canWrite && (
              <Button
                size="sm"
                variant="outline"
                disabled={busy}
                onClick={() => unmatch.mutate()}
              >
                <Link2Off className="size-4" aria-hidden />
                {t("office.bank.matching.unmatch")}
              </Button>
            )}
          </div>
          <ul className="divide-y rounded-md border text-sm">
            {v.matches.map((m) => (
              <li
                key={m.invoice_uuid}
                className="flex flex-wrap items-center justify-between gap-2 px-3 py-2"
              >
                <Link
                  className="font-medium underline-offset-4 hover:underline"
                  href={invoiceHref(m.invoice_uuid)}
                >
                  <Figure>{m.invoice_number}</Figure>
                </Link>
                <span className="text-muted-foreground">
                  {trDate(m.issue_date)} · {m.counterparty_name ?? "—"}
                </span>
                <Money value={m.amount} currency={v.currency} />
                <Badge variant="secondary">
                  {t(`office.bank.matching.method_${m.method}`)}
                </Badge>
              </li>
            ))}
          </ul>
        </section>
      )}

      {!v.matchable ? (
        <p className="text-muted-foreground text-sm">
          {t("office.bank.matching.not_matchable")}
        </p>
      ) : (
        <section className="space-y-2">
          <div className="flex items-center justify-between gap-2">
            <h3 className="text-sm font-medium">
              {t("office.bank.matching.candidates_title")}
            </h3>
            {canWrite && v.candidates.length > 0 && (
              <Button
                size="sm"
                disabled={busy || picked.length === 0}
                onClick={() => match.mutate(picked)}
              >
                <Link2 className="size-4" aria-hidden />
                {v.matches.length > 0
                  ? t("office.bank.matching.rematch", {
                      count: String(picked.length),
                    })
                  : t("office.bank.matching.match", {
                      count: String(picked.length),
                    })}
              </Button>
            )}
          </div>
          {v.candidates.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("office.bank.matching.no_candidates")}
            </p>
          ) : (
            <ul className="divide-y rounded-md border text-sm">
              {v.candidates.map((c) => {
                const id = `cand-${c.invoice_uuid}`;
                return (
                  <li
                    key={c.invoice_uuid}
                    className="flex flex-wrap items-center gap-3 px-3 py-2"
                  >
                    {canWrite && (
                      <Checkbox
                        id={id}
                        checked={picked.includes(c.invoice_uuid)}
                        onCheckedChange={(on) =>
                          toggle(c.invoice_uuid, on === true)
                        }
                        aria-label={t("office.bank.matching.pick", {
                          invoice: c.invoice_number,
                        })}
                      />
                    )}
                    <Link
                      className="font-medium underline-offset-4 hover:underline"
                      href={invoiceHref(c.invoice_uuid)}
                    >
                      <Figure>{c.invoice_number}</Figure>
                    </Link>
                    <span className="text-muted-foreground">
                      {trDate(c.issue_date)} · {c.counterparty_name ?? "—"}
                    </span>
                    <span className="ms-auto">
                      {t("office.bank.matching.open")}{" "}
                      <Money value={c.open_amount} currency={v.currency} />
                    </span>
                    <Badge
                      variant={c.score >= 80 ? "default" : "outline"}
                      title={Object.entries(c.parts)
                        .map(([k, n]) => `${k}: ${n}`)
                        .join(", ")}
                    >
                      {t("office.bank.matching.score", {
                        score: String(c.score),
                      })}
                    </Badge>
                  </li>
                );
              })}
            </ul>
          )}
        </section>
      )}
    </div>
  );
}
