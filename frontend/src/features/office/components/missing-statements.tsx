"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useState } from "react";

import { StatusPill } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { usePeriodText } from "../period/use-period-text";
import {
  type BankStatementGap,
  bankCoverageKeys,
  bankCoverageService,
} from "../services/bank-coverage.service";
import {
  type StatementAsk,
  StatementRequestDialog,
} from "./bank-statement-request";
import { Panel } from "./office-ui";

const PAGE = 10;

/**
 * Ana sayfa → "Eksik ekstreler" (Oğuz'un geri bildirimi): mükelleflerin
 * kullanımdaki banka hesaplarında ekstresi gelmemiş kapanmış aylar, satır
 * başına mükellef + hesap + ay. Satırdan tek tıkla belge isteği açılır;
 * mükellefin bütün eksikleri topluca istenir. Ofis ayarı kapalıysa ya da
 * eksik yoksa görünmez.
 */
export function MissingStatementsPanel() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const periodText = usePeriodText();
  const canRead = can(permissions.tenant.bank.read);
  const canRequest = can(permissions.tenant.docTasks.write);
  const [offset, setOffset] = useState(0);
  const [asking, setAsking] = useState<{
    engagement: string;
    taxpayer: string;
    asks: StatementAsk[];
    all: boolean;
  } | null>(null);
  const gaps = useQuery({
    queryKey: [...bankCoverageKeys.gaps(slug), offset],
    queryFn: () => bankCoverageService.gaps(slug, PAGE, offset),
    enabled: canRead,
  });

  if (!canRead) return null;
  if (gaps.isLoading) return <Skeleton className="h-24 w-full rounded-xl" />;
  const data = gaps.data;
  if (!data?.enabled || (data.total === 0 && offset === 0)) return null;

  const ask = (g: BankStatementGap, all: boolean) =>
    setAsking({
      engagement: g.engagement_uuid,
      taxpayer: g.taxpayer_title,
      all,
      asks: all
        ? data.items
            .filter((x) => x.engagement_uuid === g.engagement_uuid)
            .map((x) => ({
              cash_box_uuid: x.cash_box_uuid,
              period: x.period,
              title: x.title,
            }))
        : [
            {
              cash_box_uuid: g.cash_box_uuid,
              period: g.period,
              title: g.title,
            },
          ],
    });

  return (
    <Panel
      id="eksik-ekstreler"
      title={t("office.bank.coverage.todo_title", { count: data.total })}
      description={t("office.bank.coverage.todo_description")}
    >
      <ul className="divide-y" data-testid="missing-statements">
        {data.items.map((g, i) => {
          const first =
            i === 0 || data.items[i - 1].engagement_uuid !== g.engagement_uuid;
          return (
            <li
              key={`${g.cash_box_uuid}-${g.period}`}
              className="flex flex-wrap items-center gap-3 px-4 py-2.5 text-sm"
              data-testid="missing-statement-row"
            >
              <span className="min-w-0 flex-1">
                <Link
                  href={routes.tenant.engagement(slug, g.engagement_uuid)}
                  className="block truncate font-semibold hover:underline"
                >
                  {g.taxpayer_title}
                </Link>
                <span className="text-text-2 block truncate text-xs">
                  {g.bank_id
                    ? t(`office.bank.banks.${g.bank_id}`)
                    : g.account_name}
                  {` · ${g.currency}`}
                  {g.iban_last4 ? ` · ···${g.iban_last4}` : ""}
                  {` · ${periodText(g.period)}`}
                </span>
              </span>
              {g.requested ? (
                <StatusPill tone="info">
                  {t("office.bank.coverage.already_requested")}
                </StatusPill>
              ) : (
                <StatusPill tone="danger">
                  {t("office.bank.coverage.status_missing")}
                </StatusPill>
              )}
              {canRequest && !g.requested ? (
                <Button size="sm" variant="outline" onClick={() => ask(g, false)}>
                  {t("office.bank.coverage.ask")}
                </Button>
              ) : null}
              {canRequest && first && g.taxpayer_missing > 1 ? (
                <Button size="sm" variant="ghost" onClick={() => ask(g, true)}>
                  {t("office.bank.coverage.request_all", {
                    count: g.taxpayer_missing,
                  })}
                </Button>
              ) : null}
            </li>
          );
        })}
      </ul>
      {data.total > PAGE ? (
        <div className="flex items-center justify-between gap-2 border-t px-4 py-2 text-xs">
          <span className="text-text-2">
            {t("office.bank.coverage.page", {
              from: offset + 1,
              to: Math.min(offset + PAGE, data.total),
              total: data.total,
            })}
          </span>
          <span className="flex gap-1">
            <Button
              size="sm"
              variant="ghost"
              disabled={offset === 0}
              onClick={() => setOffset(Math.max(0, offset - PAGE))}
            >
              {t("office.bank.coverage.prev")}
            </Button>
            <Button
              size="sm"
              variant="ghost"
              disabled={offset + PAGE >= data.total}
              onClick={() => setOffset(offset + PAGE)}
            >
              {t("office.bank.coverage.next")}
            </Button>
          </span>
        </div>
      ) : null}
      {asking ? (
        <StatementRequestDialog
          engagement={asking.engagement}
          taxpayer={asking.taxpayer}
          asks={asking.asks}
          all={asking.all}
          open
          onOpenChange={(o) => {
            if (!o) setAsking(null);
          }}
        />
      ) : null}
    </Panel>
  );
}
