"use client";

import { ScrollText } from "lucide-react";
import Link from "next/link";

import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
} from "@/components/common/card";
import { StatusChip } from "@/components/common/status-chip";
import { routes } from "@/config/routes";
import { useMyLegalAcceptances } from "@/features/legal/hooks/use-legal";
import {
  LEGAL_CONTEXT_LABEL_KEYS,
  LEGAL_KINDS,
  legalKindLabelKey,
} from "@/features/legal/lib/kinds";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { Button } from "@/components/ui/button";

/**
 * "KVKK onaylarım" on the profile (issue #57, KVKK md. 11): every text the
 * user accepted, in which flow and on whose behalf, plus links to the
 * public texts.
 */
export function MyLegalAcceptancesCard() {
  const { t, locale } = useLocale();
  const query = useMyLegalAcceptances();
  const items = query.data?.items ?? [];

  return (
    <Card className="shadow-none">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <ScrollText className="text-muted-foreground size-5" aria-hidden />
          {t("legal.my_title")}
        </CardTitle>
        <p className="text-muted-foreground text-sm">
          {t("legal.my_description")}
        </p>
      </CardHeader>
      <CardContent className="space-y-4">
        {query.isLoading ? (
          <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
        ) : query.isError ? (
          <div className="flex flex-wrap items-center gap-2 text-sm">
            <span className="text-destructive">{t("legal.my_error")}</span>
            <Button
              size="sm"
              variant="outline"
              onClick={() => void query.refetch()}
            >
              {t("common.retry")}
            </Button>
          </div>
        ) : items.length === 0 ? (
          <p className="text-muted-foreground text-sm">{t("legal.my_empty")}</p>
        ) : (
          <ul className="divide-y">
            {items.map((item) => {
              const onBehalf = [item.organization, item.taxpayer]
                .filter(Boolean)
                .join(" · ");
              return (
                <li
                  key={item.uuid}
                  className="flex flex-col gap-1 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-start sm:justify-between sm:gap-4"
                >
                  <div className="min-w-0 space-y-0.5 text-sm">
                    <div className="flex flex-wrap items-center gap-2 font-medium">
                      <span>
                        {item.title || t(legalKindLabelKey(item.kind))}
                      </span>
                      <span className="text-muted-foreground font-normal">
                        {t("legal.version", { version: item.version })}
                      </span>
                      {item.withdrawn_at ? (
                        <StatusChip
                          label={t("legal.my_withdrawn")}
                          tone="default"
                        />
                      ) : null}
                    </div>
                    <div className="text-muted-foreground text-xs">
                      {t(LEGAL_CONTEXT_LABEL_KEYS[item.context])}
                      {onBehalf ? ` · ${onBehalf}` : null}
                      {" · "}
                      {t("legal.my_accepted_at", {
                        date: datetime(item.accepted_at, undefined, locale),
                      })}
                      {item.withdrawn_at ? (
                        <>
                          {" · "}
                          {t("legal.my_withdrawn_at", {
                            date: datetime(
                              item.withdrawn_at,
                              undefined,
                              locale,
                            ),
                          })}
                        </>
                      ) : null}
                    </div>
                  </div>
                  <Link
                    href={routes.legal(item.kind, {
                      version: item.version,
                      lang: item.language,
                    })}
                    className="text-primary shrink-0 text-sm underline-offset-2 hover:underline"
                  >
                    {t("legal.my_view_text")}
                  </Link>
                </li>
              );
            })}
          </ul>
        )}

        <div className="border-border border-t pt-4">
          <p className="text-muted-foreground mb-2 text-xs font-medium tracking-wide uppercase">
            {t("legal.nav_label")}
          </p>
          <ul className="flex flex-wrap gap-x-4 gap-y-1 text-sm">
            {LEGAL_KINDS.map((kind) => (
              <li key={kind}>
                <Link
                  href={routes.legal(kind)}
                  className="text-primary underline-offset-2 hover:underline"
                >
                  {t(legalKindLabelKey(kind))}
                </Link>
              </li>
            ))}
          </ul>
        </div>
      </CardContent>
    </Card>
  );
}
