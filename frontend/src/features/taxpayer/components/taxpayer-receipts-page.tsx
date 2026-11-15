"use client";

import { useQuery } from "@tanstack/react-query";
import { useState } from "react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  taxpayerPanelKeys,
  taxpayerPanelService,
} from "@/features/taxpayer/services/taxpayer.service";
import { useLocale } from "@/providers/locale-provider";

import { formatDate, formatMoney } from "./format";

const PAGE = 20;

const statusVariant: Record<
  string,
  "success" | "warning" | "danger" | "secondary"
> = {
  approved: "success",
  draft: "warning",
  duplicate: "secondary",
  failed: "danger",
  rejected: "danger",
};

/** `/m/{uuid}/receipts`: captured receipts (fiş) and their review status. */
export function TaxpayerReceiptsPage({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const [offset, setOffset] = useState(0);
  const q = useQuery({
    queryKey: taxpayerPanelKeys.receipts(taxpayer, offset),
    queryFn: () =>
      taxpayerPanelService.receipts(taxpayer, { limit: PAGE, offset }),
  });
  const data = q.data;
  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("taxpayer.receipts_title")}</CardTitle>
      </CardHeader>
      <CardContent>
        {q.isLoading ? (
          <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
        ) : q.isError ? (
          <p className="text-destructive text-sm">{t("taxpayer.load_error")}</p>
        ) : !data || data.items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("taxpayer.receipts_empty")}
          </p>
        ) : (
          <>
            <ul className="divide-y">
              {data.items.map((r) => (
                <li
                  key={r.uuid}
                  className="flex items-center justify-between gap-3 py-2 text-sm"
                >
                  <div>
                    <div>
                      {r.vendor_name ||
                        t(`taxpayer.receipt_kind_${r.kind ?? "other"}`)}
                    </div>
                    <div className="text-muted-foreground text-xs">
                      {formatDate(r.date || r.created_at, locale)}
                    </div>
                  </div>
                  <div className="flex items-center gap-3">
                    <span className="figure">
                      {r.total_amount
                        ? formatMoney(r.total_amount, "TRY", locale)
                        : "—"}
                    </span>
                    <Badge variant={statusVariant[r.status] ?? "secondary"}>
                      {t(`taxpayer.receipt_status_${r.status}`)}
                    </Badge>
                  </div>
                </li>
              ))}
            </ul>
            <div className="text-muted-foreground mt-4 flex items-center justify-between text-sm">
              <span>{t("taxpayer.total_count", { count: data.total })}</span>
              <div className="flex gap-2">
                <Button
                  variant="outline"
                  size="sm"
                  disabled={offset === 0}
                  onClick={() => setOffset(Math.max(0, offset - PAGE))}
                >
                  {t("taxpayer.prev")}
                </Button>
                <Button
                  variant="outline"
                  size="sm"
                  disabled={offset + PAGE >= data.total}
                  onClick={() => setOffset(offset + PAGE)}
                >
                  {t("taxpayer.next")}
                </Button>
              </div>
            </div>
          </>
        )}
      </CardContent>
    </Card>
  );
}
