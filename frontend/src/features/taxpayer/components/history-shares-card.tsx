"use client";

import { Share2 } from "lucide-react";
import { useId, useState, type FormEvent } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { DatePicker } from "@/components/ui/date-picker";
import { Label } from "@/components/ui/label";
import {
  useCreateHistoryShare,
  useHistoryShares,
  useRevokeHistoryShare,
} from "@/features/taxpayer/hooks/use-history-shares";
import { historyShareRangeLabel } from "@/features/taxpayer/lib/history-share";
import type { HistoryShare } from "@/features/taxpayer/services/history-shares.service";
import { date } from "@/lib/utils/format";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * "Geçmiş veri paylaşımı" on the mükellef home (issue #8, D2): the new office
 * sees documents from before the engagement only with the owner's consent,
 * which can be revoked. Only owners can read the list; for anyone else the
 * call is refused and the card stays hidden.
 */
export function HistorySharesCard({ taxpayer }: { taxpayer: string }) {
  const { t, locale } = useLocale();
  const { confirm } = useDialogs();
  const query = useHistoryShares(taxpayer);
  const create = useCreateHistoryShare(taxpayer);
  const revoke = useRevokeHistoryShare(taxpayer);
  const fromId = useId();
  const toId = useId();
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [rangeError, setRangeError] = useState<string | null>(null);

  if (!query.isSuccess) return null;
  const items = query.data.items;

  const onSubmit = (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    // YYYY-MM-DD strings compare in date order.
    if (fromDate && toDate && fromDate > toDate) {
      setRangeError(t("taxpayer.history_range_invalid"));
      return;
    }
    setRangeError(null);
    create.mutate(
      {
        ...(fromDate ? { from_date: fromDate } : {}),
        ...(toDate ? { to_date: toDate } : {}),
      },
      {
        onSuccess: () => {
          setFromDate("");
          setToDate("");
        },
      },
    );
  };

  const onRevoke = async (share: HistoryShare) => {
    const ok = await confirm({
      title: t("taxpayer.history_revoke_title", {
        office: share.organization,
      }),
      description: t("taxpayer.history_revoke_description"),
      confirmLabel: t("taxpayer.history_revoke"),
    });
    if (ok) revoke.mutate(share.uuid);
  };

  return (
    <Card>
      <CardHeader>
        <CardTitle>{t("taxpayer.history_title")}</CardTitle>
        <CardDescription>{t("taxpayer.history_description")}</CardDescription>
      </CardHeader>
      <CardContent className="space-y-6">
        <form
          className="space-y-3"
          onSubmit={onSubmit}
          aria-label={t("taxpayer.history_form_label")}
        >
          <div className="grid gap-3 sm:grid-cols-[1fr_1fr_auto] sm:items-end">
            <div className="space-y-1.5">
              <Label htmlFor={fromId}>{t("taxpayer.history_from")}</Label>
              <DatePicker
                id={fromId}
                value={fromDate}
                onChange={setFromDate}
                captionLayout="dropdown"
                aria-invalid={rangeError ? true : undefined}
              />
            </div>
            <div className="space-y-1.5">
              <Label htmlFor={toId}>{t("taxpayer.history_to")}</Label>
              <DatePicker
                id={toId}
                value={toDate}
                onChange={setToDate}
                captionLayout="dropdown"
                aria-invalid={rangeError ? true : undefined}
              />
            </div>
            <Button type="submit" disabled={create.isPending}>
              <Share2 className="size-4" />
              {t("taxpayer.history_submit")}
            </Button>
          </div>
          <p className="text-muted-foreground text-xs">
            {t("taxpayer.history_form_hint")}
          </p>
          {rangeError ? (
            <p role="alert" className="text-destructive text-sm">
              {rangeError}
            </p>
          ) : null}
        </form>

        {items.length === 0 ? (
          <p className="text-muted-foreground text-sm">
            {t("taxpayer.history_empty")}
          </p>
        ) : (
          <ul className="divide-y">
            {items.map((share) => (
              <li
                key={share.uuid}
                className="flex flex-col gap-2 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-center sm:justify-between"
              >
                <div className="text-sm">
                  <div className="flex items-center gap-2 font-medium">
                    {share.organization}
                    <StatusChip
                      label={
                        share.active
                          ? t("taxpayer.history_status_active")
                          : t("taxpayer.history_status_revoked")
                      }
                      tone={share.active ? "success" : "default"}
                    />
                  </div>
                  <div className="text-muted-foreground text-xs">
                    {t("taxpayer.history_range", {
                      range: historyShareRangeLabel(share, t, locale),
                    })}
                    {" · "}
                    {t("taxpayer.history_granted", {
                      date: date(share.granted_at, "dd.MM.yyyy", locale),
                    })}
                    {share.revoked_at ? (
                      <>
                        {" · "}
                        {t("taxpayer.history_revoked_at", {
                          date: date(share.revoked_at, "dd.MM.yyyy", locale),
                        })}
                      </>
                    ) : null}
                  </div>
                </div>
                {share.active ? (
                  <Button
                    type="button"
                    size="sm"
                    variant="outline"
                    disabled={revoke.isPending}
                    onClick={() => void onRevoke(share)}
                  >
                    {t("taxpayer.history_revoke")}
                  </Button>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}
