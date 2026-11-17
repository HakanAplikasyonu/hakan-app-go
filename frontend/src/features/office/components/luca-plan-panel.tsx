"use client";

import { Download, Loader2, Wand2 } from "lucide-react";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  useCariSubaccounts,
  useLucaPlanActions,
  useLucaPlanFiles,
} from "../hooks/use-luca-plan";
import { Figure, Panel } from "./office-ui";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

function errorText(t: Translate, err: unknown) {
  if (isApiError(err)) {
    const key = `office.luca_plan.error_${err.code}`;
    const msg = err.code ? t(key) : key;
    return msg !== key ? msg : err.message || t("office.api.load_error");
  }
  return t("office.api.load_error");
}

/**
 * Luca "Hesap Planı Aktar" dosyası (#464): carilere alt hesap kodu, yalnızca
 * Luca'da olmayan hesaplar ya da tüm plan (CSV, Windows-1254) ve indirme
 * geçmişi. Biçim Luca'da doğrulanmadı; ekran bunu söyler.
 */
export function LucaPlanPanel({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const subs = useCariSubaccounts(slug, engagement);
  const files = useLucaPlanFiles(slug, engagement);
  const { generate, download } = useLucaPlanActions(slug, engagement);
  const items = subs.data?.items ?? [];
  const history = files.data?.items ?? [];
  const missing = items.filter((s) => !s.in_plan).length;
  const canWrite = can(permissions.tenant.accountPlan.write);
  const error = generate.error ?? download.error;

  return (
    <Panel
      title={t("office.luca_plan.title")}
      description={t("office.luca_plan.description")}
    >
      <div className="space-y-4 p-4 text-sm">
        <p className="bg-status-warning/12 text-status-warning rounded-md px-3 py-2 text-xs">
          {t("office.luca_plan.unverified")}
        </p>

        <div className="flex flex-wrap items-center gap-2">
          {canWrite ? (
            <Button
              size="sm"
              variant="outline"
              disabled={generate.isPending}
              onClick={() => generate.mutate()}
            >
              {generate.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Wand2 className="size-4" />
              )}
              {t("office.luca_plan.generate")}
            </Button>
          ) : null}
          <Button
            size="sm"
            disabled={download.isPending}
            onClick={() => download.mutate("new")}
          >
            {download.isPending && download.variables === "new" ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Download className="size-4" />
            )}
            {t("office.luca_plan.download_new", { count: missing })}
          </Button>
          <Button
            size="sm"
            variant="outline"
            disabled={download.isPending}
            onClick={() => download.mutate("full")}
          >
            {download.isPending && download.variables === "full" ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Download className="size-4" />
            )}
            {t("office.luca_plan.download_full")}
          </Button>
        </div>
        {generate.data ? (
          <p className="text-status-approved text-xs">
            {t("office.luca_plan.generated", {
              created: generate.data.created.length,
              existing: generate.data.existing,
            })}
          </p>
        ) : null}
        {error ? (
          <p className="text-status-blocking text-xs">{errorText(t, error)}</p>
        ) : null}

        <div className="space-y-2">
          <p className="text-xs font-medium">
            {t("office.luca_plan.subaccounts")}
          </p>
          {subs.isLoading ? (
            <Skeleton className="h-16" />
          ) : items.length === 0 ? (
            <p className="text-muted-foreground text-xs">
              {t("office.luca_plan.subaccounts_empty")}
            </p>
          ) : (
            <ul className="divide-y text-xs">
              {items.map((s) => (
                <li
                  key={s.code}
                  className="flex flex-wrap items-center justify-between gap-2 py-1.5"
                >
                  <span>
                    <Figure className="font-mono font-medium">{s.code}</Figure>{" "}
                    {s.name}
                  </span>
                  <span className="flex items-center gap-2">
                    <span className="text-muted-foreground">
                      {t(`office.luca_plan.source_${s.source}`)}
                    </span>
                    <StatusChip
                      tone={s.in_plan ? "success" : "warning"}
                      label={
                        s.in_plan
                          ? t("office.luca_plan.in_plan")
                          : t("office.luca_plan.not_in_plan")
                      }
                    />
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>

        <div className="space-y-2 border-t pt-3">
          <p className="text-xs font-medium">{t("office.luca_plan.history")}</p>
          {files.isLoading ? (
            <Skeleton className="h-12" />
          ) : history.length === 0 ? (
            <p className="text-muted-foreground text-xs">
              {t("office.luca_plan.history_empty")}
            </p>
          ) : (
            <ul className="divide-y text-xs">
              {history.map((f) => (
                <li key={f.uuid} className="space-y-0.5 py-1.5">
                  <span className="block">
                    <Figure className="font-medium">{f.file_name}</Figure>{" "}
                    <span className="text-muted-foreground">
                      {t(`office.luca_plan.kind_${f.kind}`)}
                    </span>
                  </span>
                  <span className="text-muted-foreground block">
                    {t("office.luca_plan.history_row", {
                      date: datetime(f.created_at, undefined, locale),
                      count: f.account_count,
                      compared:
                        f.compared_file_name ||
                        t("office.luca_plan.no_plan_imported"),
                    })}
                  </span>
                </li>
              ))}
            </ul>
          )}
        </div>
      </div>
    </Panel>
  );
}
