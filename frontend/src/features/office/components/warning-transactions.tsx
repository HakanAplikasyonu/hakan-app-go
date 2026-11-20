"use client";

import { useQuery } from "@tanstack/react-query";
import { CheckCircle2, ChevronDown, ChevronRight } from "lucide-react";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import type { WarningTransaction } from "../lib/warning-transactions";
import {
  bankMatchingKeys,
  bankMatchingService,
} from "../services/bank-matching.service";
import { BankTransactionMatchingPanel } from "./bank-transaction-matching";
import { Figure, Money } from "./office-ui";

/** Panelde bir arada gösterilen hareket sayısı; fazlası Banka ekranında. */
const MAX_SHOWN = 15;

function trDate(iso: string) {
  const [y, m, d] = iso.split("-");
  return y && m && d ? `${d}.${m}.${y}` : iso;
}

/**
 * Dönem uyarısının hareketleri (#570): uyarı tek kayda bağlı olmadığında
 * (faturasız tahsilat, belgesiz ödeme, sınıflanmamış hareket) müşavir her
 * hareketi panelden açar ve çözer (faturayla eşleştirir ya da hesaba
 * işler). Hepsi çözülünce uyarı kendiliğinden kapanır.
 */
export function WarningTransactions({
  engagement,
  items,
}: {
  engagement: string;
  items: WarningTransaction[];
}) {
  const { t } = useLocale();
  // Tek hareket varsa açık gelir: uyarı zaten onu soruyor.
  const [open, setOpen] = useState<string | null>(
    items.length === 1 ? items[0].uuid : null,
  );
  if (items.length === 0) return null;
  const shown = items.slice(0, MAX_SHOWN);
  return (
    <section className="space-y-2" data-testid="warning-transactions">
      <h4 className="text-sm font-semibold">
        {t("office.warning_sheet.transactions_title", {
          count: String(items.length),
        })}
      </h4>
      <p className="text-text-3 text-xs">
        {t("office.warning_sheet.transactions_hint")}
      </p>
      <ul className="border-line divide-y rounded-xl border">
        {shown.map((item) => (
          <TransactionRow
            key={item.uuid}
            engagement={engagement}
            item={item}
            open={open === item.uuid}
            onToggle={() => setOpen(open === item.uuid ? null : item.uuid)}
          />
        ))}
      </ul>
      {items.length > shown.length ? (
        <p className="text-text-3 text-xs">
          {t("office.warning_sheet.transactions_more", {
            count: String(items.length - shown.length),
          })}
        </p>
      ) : null}
    </section>
  );
}

function TransactionRow({
  engagement,
  item,
  open,
  onToggle,
}: {
  engagement: string;
  item: WarningTransaction;
  open: boolean;
  onToggle: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  // Panelle aynı anahtar: açılınca yeniden istenmez, eşleşince satır yenilenir.
  const q = useQuery({
    queryKey: bankMatchingKeys.one(slug, engagement, item.uuid),
    queryFn: () => bankMatchingService.get(slug, engagement, item.uuid),
  });
  const v = q.data;
  const name =
    v?.counterparty_name || item.counterparty || v?.description || "—";
  const panelId = `warning-transaction-${item.uuid}`;
  return (
    <li>
      <Button
        variant="ghost"
        className="h-auto w-full justify-start gap-2 rounded-none px-3 py-2 text-left font-normal"
        aria-expanded={open}
        aria-controls={panelId}
        onClick={onToggle}
      >
        {open ? (
          <ChevronDown className="size-4 shrink-0" aria-hidden />
        ) : (
          <ChevronRight className="size-4 shrink-0" aria-hidden />
        )}
        {q.isPending ? (
          <Skeleton className="h-4 w-48" />
        ) : (
          <>
            {v ? (
              <Figure className="text-text-3 shrink-0 text-xs">
                {trDate(v.transaction_date)}
              </Figure>
            ) : null}
            <span className="min-w-0 flex-1 truncate">{name}</span>
            {v ? (
              <span className="shrink-0">
                {v.debit_credit === "A" ? "+" : "−"}
                <Money value={v.amount} currency={v.currency} />
              </span>
            ) : null}
            {v?.match_status === "matched" ? (
              <Badge variant="secondary" className="shrink-0">
                <CheckCircle2 className="size-3" aria-hidden />
                {t("office.bank.matching.status_matched")}
              </Badge>
            ) : null}
          </>
        )}
      </Button>
      {open ? (
        <div id={panelId} className="border-line border-t p-3">
          <BankTransactionMatchingPanel
            engagement={engagement}
            transaction={item.uuid}
          />
        </div>
      ) : null}
    </li>
  );
}
