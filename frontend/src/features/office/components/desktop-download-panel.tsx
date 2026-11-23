"use client";

import { Download, Monitor, MonitorSmartphone } from "lucide-react";
import type { ReactNode } from "react";

import { StatusChip } from "@/components/common/status-chip";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { date, fileSize } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

import { useLatestDesktopRelease } from "../hooks/use-agents";
import {
  DESKTOP_RELEASES_PAGE,
  type DesktopDownload,
} from "../services/desktop-release.service";
import { Panel } from "./office-ui";

/**
 * Masaüstü ajanını indirme (Oğuz'un demo geri bildirimi): en yeni
 * `desktop-v*` GitHub Release'indeki .dmg ve .msi/.exe paketleri. İmzasız
 * paketin ilk açılışında çıkan işletim sistemi uyarısı da anlatılır.
 */
export function DesktopDownloadPanel() {
  const { t, locale } = useLocale();
  const release = useLatestDesktopRelease();
  const r = release.data;

  return (
    <Panel
      id="indir"
      title={t("office.agents.download_title")}
      description={t("office.agents.download_description")}
    >
      <div className="space-y-4 p-4" data-testid="desktop-download">
        {release.isLoading ? (
          <div className="flex flex-wrap gap-2">
            <Skeleton className="h-9 w-40" />
            <Skeleton className="h-9 w-40" />
          </div>
        ) : release.isError ? (
          <div className="space-y-2 text-sm">
            <p>{t("office.agents.download_error")}</p>
            <div className="flex flex-wrap gap-2">
              <Button
                variant="outline"
                size="sm"
                onClick={() => void release.refetch()}
              >
                {t("office.agents.download_retry")}
              </Button>
              <Button asChild variant="ghost" size="sm">
                <a
                  href={DESKTOP_RELEASES_PAGE}
                  target="_blank"
                  rel="noopener noreferrer"
                >
                  {t("office.agents.download_releases_page")}
                </a>
              </Button>
            </div>
          </div>
        ) : !r ? (
          <div className="space-y-1" data-testid="desktop-download-none">
            <p className="font-medium">
              {t("office.agents.download_none_title")}
            </p>
            <p className="text-muted-foreground text-sm">
              {t("office.agents.download_none_description")}
            </p>
          </div>
        ) : (
          <>
            <div className="text-muted-foreground flex flex-wrap items-center gap-2 text-xs">
              <span>
                {r.publishedAt
                  ? t("office.agents.download_version_date", {
                      version: r.version,
                      date: date(r.publishedAt, "dd.MM.yyyy", locale),
                    })
                  : t("office.agents.download_version", {
                      version: r.version,
                    })}
              </span>
              {r.prerelease ? (
                <StatusChip
                  tone="warning"
                  label={t("office.agents.download_beta")}
                />
              ) : null}
            </div>
            <div className="flex flex-wrap gap-2">
              <DownloadButton
                file={r.mac}
                label={t("office.agents.download_mac")}
                missing={t("office.agents.download_missing")}
                icon={<MonitorSmartphone className="size-4" />}
                testId="desktop-download-mac"
              />
              <DownloadButton
                file={r.windows}
                label={t("office.agents.download_windows")}
                missing={t("office.agents.download_missing")}
                icon={<Monitor className="size-4" />}
                testId="desktop-download-windows"
              />
            </div>
          </>
        )}

        <div className="bg-muted/40 space-y-1 rounded-lg p-3 text-sm">
          <p className="font-medium">
            {t("office.agents.download_unsigned_title")}
          </p>
          <p className="text-muted-foreground">
            {t("office.agents.download_unsigned_why")}
          </p>
          <ul className="text-muted-foreground list-disc space-y-1 pl-5">
            <li>{t("office.agents.download_unsigned_mac")}</li>
            <li>{t("office.agents.download_unsigned_windows")}</li>
          </ul>
        </div>
      </div>
    </Panel>
  );
}

function DownloadButton({
  file,
  label,
  missing,
  icon,
  testId,
}: {
  file?: DesktopDownload;
  label: string;
  missing: string;
  icon: ReactNode;
  testId: string;
}) {
  if (!file) {
    return (
      <span className="inline-flex flex-col gap-1">
        <Button variant="outline" disabled data-testid={testId}>
          {icon}
          {label}
        </Button>
        <span className="text-muted-foreground text-xs">{missing}</span>
      </span>
    );
  }
  return (
    <Button asChild>
      <a
        href={file.url}
        rel="noopener noreferrer"
        data-testid={testId}
        title={file.size ? `${file.name} · ${fileSize(file.size)}` : file.name}
      >
        {icon}
        {label}
        <Download className="size-4 opacity-70" />
      </a>
    </Button>
  );
}
