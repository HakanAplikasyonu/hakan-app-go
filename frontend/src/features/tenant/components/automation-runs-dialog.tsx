"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { useState } from "react";

import { Loading } from "@/components/common/loading";
import { DateCell, StatusPill, type StatusTone } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  automationKeys,
  automationService,
  type AutomationRun,
} from "@/features/tenant/services/automation.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

export const runTone: Record<AutomationRun["status"], StatusTone> = {
  scheduled: "info",
  draft: "warn",
  sent: "ok",
  skipped: "neutral",
  failed: "danger",
  rejected: "neutral",
};

/** Kanal başına sonuç: "SMS: deneme modu · E-posta: gönderildi". */
function ChannelResults({ run }: { run: AutomationRun }) {
  const { t } = useLocale();
  const entries = Object.entries(run.result ?? {});
  if (entries.length === 0) return null;
  return (
    <ul className="text-text-2 space-y-0.5 text-xs">
      {entries.map(([ch, r]) => (
        <li key={ch}>
          {t(`tenant.automation_channel_${ch}`)}:{" "}
          {t(`tenant.automation_result_${r.status}`)}
          {r.error ? ` (${r.error})` : ""}
        </li>
      ))}
      {run.sms_followup_status === "sent" ? (
        <li>{t("tenant.automation_followup_sent")}</li>
      ) : null}
    </ul>
  );
}

function RunsTable({
  runs,
  showRule,
}: {
  runs: AutomationRun[];
  showRule: boolean;
}) {
  const { t } = useLocale();
  if (runs.length === 0) {
    return (
      <p className="text-text-2 text-sm">{t("tenant.automation_runs_empty")}</p>
    );
  }
  return (
    <Table data-testid="automation-runs">
      <TableHeader>
        <TableRow>
          <TableHead>{t("tenant.automation_col_date")}</TableHead>
          {showRule ? (
            <TableHead>{t("tenant.automation_col_name")}</TableHead>
          ) : null}
          <TableHead>{t("tenant.automation_col_taxpayer")}</TableHead>
          <TableHead>{t("tenant.automation_col_status")}</TableHead>
          <TableHead>{t("tenant.automation_col_result")}</TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {runs.map((r) => (
          <TableRow key={r.uuid}>
            <TableCell>
              <DateCell value={r.sent_at ?? r.created_at} style="datetime" />
            </TableCell>
            {showRule ? <TableCell>{r.rule_name}</TableCell> : null}
            <TableCell>
              <div>{r.taxpayer_title}</div>
              <p className="text-text-3 line-clamp-2 max-w-md text-xs">
                {r.body}
              </p>
            </TableCell>
            <TableCell>
              <StatusPill tone={runTone[r.status]}>
                {t(`tenant.automation_run_${r.status}`)}
              </StatusPill>
            </TableCell>
            <TableCell>
              <ChannelResults run={r} />
              {r.note ? <p className="text-text-3 text-xs">{r.note}</p> : null}
            </TableCell>
          </TableRow>
        ))}
      </TableBody>
    </Table>
  );
}

/**
 * Gönderim geçmişi: ofisin bütün otomasyon gönderimleri ya da tek kuralın
 * çalışmaları ve düzenleme geçmişi. "Bana söylemediniz" diyen mükellefe
 * gösterilecek kanıt burada.
 */
export function AutomationHistoryDialog({
  slug,
  open,
  onOpenChange,
  ruleUuid,
}: {
  slug: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  ruleUuid?: string;
}) {
  const { t } = useLocale();
  const all = useQuery({
    queryKey: automationKeys.runs(slug, "all"),
    queryFn: () => automationService.runs(slug, undefined, 200),
    enabled: open && !ruleUuid,
  });
  const one = useQuery({
    queryKey: automationKeys.history(slug, ruleUuid ?? ""),
    queryFn: () => automationService.history(slug, ruleUuid ?? ""),
    enabled: open && !!ruleUuid,
  });
  const loading = ruleUuid ? one.isLoading : all.isLoading;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>
            {ruleUuid && one.data
              ? t("tenant.automation_history_rule_title", {
                  name: one.data.rule.name,
                })
              : t("tenant.automation_history_title")}
          </DialogTitle>
          <DialogDescription>
            {t("tenant.automation_history_description")}
          </DialogDescription>
        </DialogHeader>
        {loading ? <Loading label={t("common.loading")} /> : null}
        {!ruleUuid && all.data ? (
          <RunsTable runs={all.data.items} showRule />
        ) : null}
        {ruleUuid && one.data ? (
          <div className="space-y-4">
            <RunsTable runs={one.data.runs} showRule={false} />
            <div className="space-y-2">
              <h3 className="text-sm font-semibold">
                {t("tenant.automation_edits_title")}
              </h3>
              <ul
                className="divide-border divide-y text-sm"
                data-testid="automation-edits"
              >
                {one.data.edits.map((e, i) => (
                  <li
                    key={i}
                    className="flex flex-wrap items-center gap-2 py-1.5"
                  >
                    <StatusPill tone="neutral">
                      {t(`tenant.automation_edit_${e.action}`)}
                    </StatusPill>
                    <span className="text-text-2">
                      {e.actor_name || t("tenant.automation_actor_system")}
                    </span>
                    <DateCell
                      value={e.at}
                      style="datetime"
                      className="text-text-3 ml-auto"
                    />
                  </li>
                ))}
              </ul>
            </div>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}

/**
 * Bekleyen taslaklar: "Taslak oluştur, ofis onaylasın" kurallarının
 * mükellefe gitmeden önce ofisin onayını bekleyen mesajları. Tek tek ya da
 * toplu onaylanır (gönderilir) veya reddedilir.
 */
export function AutomationDraftsDialog({
  slug,
  open,
  onOpenChange,
  canWrite,
}: {
  slug: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const [picked, setPicked] = useState<string[]>([]);
  const query = useQuery({
    queryKey: automationKeys.runs(slug, "draft"),
    queryFn: () => automationService.runs(slug, "draft", 500),
    enabled: open,
  });
  const decide = useAppMutation({
    mutationFn: (v: { uuids: string[]; approve: boolean }) =>
      automationService.decide(slug, v.uuids, v.approve),
    onSuccess: (res) => {
      setPicked([]);
      void qc.invalidateQueries({ queryKey: automationKeys.all(slug) });
      appToast.success(
        t("tenant.automation_decided", {
          sent: String(res.sent),
          rejected: String(res.rejected),
          failed: String(res.failed),
        }),
      );
    },
  });
  const items = query.data?.items ?? [];
  const allPicked = items.length > 0 && picked.length === items.length;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="max-h-[90vh] overflow-y-auto sm:max-w-4xl">
        <DialogHeader>
          <DialogTitle>{t("tenant.automation_drafts_title")}</DialogTitle>
          <DialogDescription>
            {t("tenant.automation_drafts_description")}
          </DialogDescription>
        </DialogHeader>
        {query.isLoading ? <Loading label={t("common.loading")} /> : null}
        {query.data && items.length === 0 ? (
          <p className="text-text-2 text-sm">
            {t("tenant.automation_drafts_empty")}
          </p>
        ) : null}
        {items.length > 0 ? (
          <div className="space-y-3" data-testid="automation-drafts">
            {canWrite ? (
              <div className="flex flex-wrap items-center gap-2">
                <Checkbox
                  id="automation-drafts-all"
                  checked={allPicked}
                  onCheckedChange={(v) =>
                    setPicked(v === true ? items.map((r) => r.uuid) : [])
                  }
                />
                <span className="text-sm">
                  {t("tenant.automation_drafts_pick_all", {
                    count: String(picked.length),
                  })}
                </span>
                <Button
                  type="button"
                  size="sm"
                  className="ml-auto"
                  disabled={picked.length === 0 || decide.isPending}
                  onClick={() =>
                    decide.mutate({ uuids: picked, approve: true })
                  }
                >
                  {t("tenant.automation_drafts_approve_selected")}
                </Button>
                <Button
                  type="button"
                  size="sm"
                  variant="outline"
                  disabled={picked.length === 0 || decide.isPending}
                  onClick={() =>
                    decide.mutate({ uuids: picked, approve: false })
                  }
                >
                  {t("tenant.automation_drafts_reject_selected")}
                </Button>
              </div>
            ) : null}
            <ul className="divide-border divide-y">
              {items.map((r) => (
                <li key={r.uuid} className="flex gap-3 py-3 text-sm">
                  {canWrite ? (
                    <Checkbox
                      aria-label={r.taxpayer_title}
                      checked={picked.includes(r.uuid)}
                      onCheckedChange={(v) =>
                        setPicked((p) =>
                          v === true
                            ? [...p, r.uuid]
                            : p.filter((x) => x !== r.uuid),
                        )
                      }
                    />
                  ) : null}
                  <div className="min-w-0 flex-1 space-y-1">
                    <div className="flex flex-wrap items-center gap-2">
                      <span className="font-medium">{r.taxpayer_title}</span>
                      <span className="text-text-3">{r.rule_name}</span>
                      <span className="text-text-3">
                        {r.channels
                          .map((c) => t(`tenant.automation_channel_${c}`))
                          .join(", ")}
                      </span>
                    </div>
                    {r.subject ? (
                      <p className="font-medium">{r.subject}</p>
                    ) : null}
                    <p className="text-text-2 whitespace-pre-wrap">{r.body}</p>
                  </div>
                  {canWrite ? (
                    <div className="flex shrink-0 flex-col gap-1">
                      <Button
                        type="button"
                        size="sm"
                        disabled={decide.isPending}
                        onClick={() =>
                          decide.mutate({ uuids: [r.uuid], approve: true })
                        }
                      >
                        {t("tenant.automation_drafts_approve")}
                      </Button>
                      <Button
                        type="button"
                        size="sm"
                        variant="ghost"
                        disabled={decide.isPending}
                        onClick={() =>
                          decide.mutate({ uuids: [r.uuid], approve: false })
                        }
                      >
                        {t("tenant.automation_drafts_reject")}
                      </Button>
                    </div>
                  ) : null}
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </DialogContent>
    </Dialog>
  );
}
