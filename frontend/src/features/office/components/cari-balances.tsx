"use client";

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { StatusPill } from "@/components/tables/cells";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { MonthPicker } from "@/components/ui/month-picker";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  trialBalanceKeys,
  trialBalanceService,
  type CariBalanceAccount,
  type CariBalanceGroup,
} from "../services/trial-balance.service";
import { Money, Panel } from "./office-ui";

/** İşaretli bakiye → borç / alacak bakiyesi sütunları. */
function balanceCells(balance: string) {
  const b = balance.trim();
  if (b.startsWith("-")) return { debit: "", credit: b.slice(1) };
  return { debit: Number(b) === 0 ? "" : b, credit: "" };
}

function matches(a: CariBalanceAccount, q: string) {
  if (!q) return true;
  const s = q.toLocaleLowerCase("tr-TR");
  return (
    a.code.toLocaleLowerCase("tr-TR").includes(s) ||
    a.name.toLocaleLowerCase("tr-TR").includes(s) ||
    (a.tax_id ?? "").includes(s)
  );
}

/**
 * Cari mizanı (#780 C): her carinin borç / alacak / bakiyesi, ana hesap
 * altında. Ana hesap toplamları canlı mizandaki satırla aynıdır; tutarın
 * hangi carilerden oluştuğu pay sütununda görünür. Ters bakiyeli cari ve
 * hiçbir cariye yazılmamış tutar ayrıca işaretlenir.
 */
export function CariBalances({
  engagement,
  period,
  includeDrafts,
  onPeriod,
  onDrafts,
  onOpenLive,
}: {
  engagement: string;
  period: string;
  includeDrafts: boolean;
  onPeriod: (period: string) => void;
  onDrafts: (on: boolean) => void;
  onOpenLive: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const [q, setQ] = useState("");
  const [reverseOnly, setReverseOnly] = useState(false);
  const query = { period, includeDrafts };
  const report = useQuery({
    queryKey: trialBalanceKeys.cari(slug, engagement, query),
    queryFn: () => trialBalanceService.cari(slug, engagement, query),
  });
  const rep = report.data;

  const groups: CariBalanceGroup[] = (rep?.groups ?? [])
    .map((g) => ({
      ...g,
      accounts: g.accounts.filter(
        (a) => matches(a, q.trim()) && (!reverseOnly || a.reverse),
      ),
    }))
    .filter((g) => g.accounts.length > 0);

  return (
    <div className="space-y-4">
      <Panel
        title={t("office.trial_balance.cari_title")}
        description={t("office.trial_balance.cari_description", {
          from: rep?.from_period ?? `${period.slice(0, 4)}-01`,
          to: period,
        })}
        actions={
          <Button size="sm" variant="outline" onClick={onOpenLive}>
            {t("office.trial_balance.cari_open_live")}
          </Button>
        }
      >
        <div className="flex flex-wrap items-end gap-4 p-4">
          <div className="space-y-1">
            <Label htmlFor="cari-balances-period">
              {t("office.trial_balance.live_period")}
            </Label>
            <MonthPicker
              id="cari-balances-period"
              value={period}
              onChange={onPeriod}
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="cari-balances-search">
              {t("office.trial_balance.cari_search")}
            </Label>
            <Input
              id="cari-balances-search"
              className="w-64"
              value={q}
              placeholder={t("office.trial_balance.cari_search_placeholder")}
              onChange={(e) => setQ(e.target.value)}
            />
          </div>
          <Label className="flex h-9 w-fit items-center gap-2 text-sm leading-normal font-normal">
            <Checkbox
              checked={includeDrafts}
              onCheckedChange={(v) => onDrafts(v === true)}
            />
            {t("office.trial_balance.include_drafts")}
          </Label>
          <Label className="flex h-9 w-fit items-center gap-2 text-sm leading-normal font-normal">
            <Checkbox
              checked={reverseOnly}
              onCheckedChange={(v) => setReverseOnly(v === true)}
            />
            {t("office.trial_balance.cari_reverse_only")}
          </Label>
        </div>
        {rep && rep.groups.length > 0 ? (
          <dl
            className="grid grid-cols-2 gap-3 border-t p-4 text-sm sm:grid-cols-4"
            data-testid="cari-balances-summary"
          >
            <div>
              <dt className="text-text-2">
                {t("office.trial_balance.cari_count")}
              </dt>
              <dd className="font-medium">{rep.totals.count}</dd>
            </div>
            <div>
              <dt className="text-text-2">
                {t("office.trial_balance.cari_debit_balances")}
              </dt>
              <dd className="font-medium">
                <Money value={rep.totals.debit_balance} />
              </dd>
            </div>
            <div>
              <dt className="text-text-2">
                {t("office.trial_balance.cari_credit_balances")}
              </dt>
              <dd className="font-medium">
                <Money value={rep.totals.credit_balance} />
              </dd>
            </div>
            <div>
              <dt className="text-text-2">
                {t("office.trial_balance.cari_reverse_count")}
              </dt>
              <dd className="font-medium">
                {rep.totals.reverse_count > 0 ? (
                  <StatusPill tone="warn">
                    {rep.totals.reverse_count}
                  </StatusPill>
                ) : (
                  0
                )}
              </dd>
            </div>
          </dl>
        ) : null}
      </Panel>

      {report.isLoading ? (
        <Skeleton className="h-64 w-full" />
      ) : report.isError || !rep ? (
        <p className="text-danger text-sm">{t("office.api.load_error")}</p>
      ) : rep.groups.length === 0 ? (
        <EmptyState
          title={t("office.trial_balance.cari_empty_title")}
          description={t("office.trial_balance.cari_empty_body")}
        />
      ) : groups.length === 0 ? (
        <EmptyState
          title={t("office.trial_balance.cari_no_match_title")}
          description={t("office.trial_balance.cari_no_match_body")}
        />
      ) : (
        <div className="overflow-x-auto rounded-xl border">
          <Table
            aria-label={t("office.trial_balance.cari_title")}
            data-testid="cari-balances-table"
          >
            <TableHeader>
              <TableRow>
                <TableHead>{t("office.trial_balance.col_code")}</TableHead>
                <TableHead>{t("office.trial_balance.cari_col_name")}</TableHead>
                <TableHead className="text-end">
                  {t("office.trial_balance.col_debit")}
                </TableHead>
                <TableHead className="text-end">
                  {t("office.trial_balance.col_credit")}
                </TableHead>
                <TableHead className="text-end">
                  {t("office.trial_balance.col_debit_balance")}
                </TableHead>
                <TableHead className="text-end">
                  {t("office.trial_balance.col_credit_balance")}
                </TableHead>
                <TableHead className="text-end">
                  {t("office.trial_balance.cari_col_share")}
                </TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {groups.map((g) => (
                <CariGroupRows key={g.main_code} group={g} />
              ))}
            </TableBody>
            <TableFooter>
              <TableRow>
                <TableCell className="px-3 py-2" colSpan={2}>
                  {t("office.trial_balance.total")}
                </TableCell>
                <TableCell className="px-3 py-2 text-end">
                  <Money value={rep.totals.debit} />
                </TableCell>
                <TableCell className="px-3 py-2 text-end">
                  <Money value={rep.totals.credit} />
                </TableCell>
                <TableCell className="px-3 py-2 text-end">
                  <Money value={rep.totals.debit_balance} />
                </TableCell>
                <TableCell className="px-3 py-2 text-end">
                  <Money value={rep.totals.credit_balance} />
                </TableCell>
                <TableCell />
              </TableRow>
            </TableFooter>
          </Table>
        </div>
      )}
    </div>
  );
}

function CariGroupRows({ group: g }: { group: CariBalanceGroup }) {
  const { t } = useLocale();
  const gb = balanceCells(g.balance);
  return (
    <>
      <TableRow
        className="bg-surface-2 font-medium"
        data-testid={`cari-group-${g.main_code}`}
      >
        <TableCell className="px-3 py-2">{g.main_code}</TableCell>
        <TableCell className="px-3 py-2">
          {g.name}{" "}
          <span className="text-text-2 text-xs font-normal">
            {t("office.trial_balance.cari_group_count", { count: g.count })}
          </span>
        </TableCell>
        <TableCell className="px-3 py-2 text-end">
          <Money value={g.debit} />
        </TableCell>
        <TableCell className="px-3 py-2 text-end">
          <Money value={g.credit} />
        </TableCell>
        <TableCell className="px-3 py-2 text-end">
          {gb.debit ? <Money value={gb.debit} /> : null}
        </TableCell>
        <TableCell className="px-3 py-2 text-end">
          {gb.credit ? <Money value={gb.credit} /> : null}
        </TableCell>
        <TableCell />
      </TableRow>
      {g.accounts.map((a) => {
        const b = balanceCells(a.balance);
        return (
          <TableRow key={a.code} data-testid={`cari-row-${a.code}`}>
            <TableCell className="px-3 py-1.5 ps-6">{a.code}</TableCell>
            <TableCell className="px-3 py-1.5">
              <div className="flex flex-wrap items-center gap-2">
                <span>
                  {a.unassigned
                    ? t("office.trial_balance.cari_unassigned")
                    : a.name || t("office.trial_balance.cari_unnamed")}
                </span>
                {a.tax_id ? (
                  <span className="text-text-2 text-xs">
                    {t("office.trial_balance.cari_tax_id", { id: a.tax_id })}
                  </span>
                ) : null}
                {a.reverse ? (
                  <StatusPill tone="warn">
                    {t("office.trial_balance.cari_reverse")}
                  </StatusPill>
                ) : null}
                {a.unassigned ? (
                  <StatusPill tone="info">
                    {t("office.trial_balance.cari_unassigned_hint")}
                  </StatusPill>
                ) : null}
              </div>
            </TableCell>
            <TableCell className="px-3 py-1.5 text-end">
              <Money value={a.debit} />
            </TableCell>
            <TableCell className="px-3 py-1.5 text-end">
              <Money value={a.credit} />
            </TableCell>
            <TableCell className="px-3 py-1.5 text-end">
              {b.debit ? <Money value={b.debit} /> : null}
            </TableCell>
            <TableCell className="px-3 py-1.5 text-end">
              {b.credit ? <Money value={b.credit} /> : null}
            </TableCell>
            <TableCell className="text-text-2 px-3 py-1.5 text-end">
              {a.share ? `%${a.share.replace(".", ",")}` : ""}
            </TableCell>
          </TableRow>
        );
      })}
    </>
  );
}
