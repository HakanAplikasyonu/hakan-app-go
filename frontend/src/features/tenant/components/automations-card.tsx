"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";
import { History, Pencil, Plus, Trash2 } from "lucide-react";
import { useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { StatusPill } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Switch } from "@/components/ui/switch";
import { TabStrip, TabStripItem } from "@/components/ui/tab-strip";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { AutomationRuleDialog } from "@/features/tenant/components/automation-rule-dialog";
import {
  AutomationDraftsDialog,
  AutomationHistoryDialog,
} from "@/features/tenant/components/automation-runs-dialog";
import {
  automationKeys,
  automationService,
  type AutomationRule,
} from "@/features/tenant/services/automation.service";
import { useAppMutation } from "@/lib/query/mutation";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

type Filter = "all" | "active" | "passive";

/**
 * Ayarlar → Otomasyonlar (#780 C, İletişim merkezi): kural listesi, ofis
 * geneli ana anahtar, bekleyen taslaklar ve gönderim geçmişi. Hazır kurallar
 * pasif gelir; satırdaki anahtarla tek tıkla etkinleşir.
 */
export function AutomationsCard({
  slug,
  canWrite,
}: {
  slug: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const qc = useQueryClient();
  const { confirmDelete } = useDialogs();
  const [filter, setFilter] = useState<Filter>("all");
  const [editing, setEditing] = useState<AutomationRule | undefined>();
  const [ruleOpen, setRuleOpen] = useState(false);
  const [historyOpen, setHistoryOpen] = useState(false);
  const [historyRule, setHistoryRule] = useState<string | undefined>();
  const [draftsOpen, setDraftsOpen] = useState(false);

  const query = useQuery({
    queryKey: automationKeys.all(slug),
    queryFn: () => automationService.overview(slug),
  });
  const invalidate = () =>
    qc.invalidateQueries({ queryKey: automationKeys.all(slug) });

  const setEnabled = useAppMutation({
    mutationFn: (enabled: boolean) =>
      automationService.setEnabled(slug, enabled),
    onSuccess: () => void invalidate(),
  });
  const setActive = useAppMutation({
    mutationFn: (v: { uuid: string; active: boolean }) =>
      automationService.setActive(slug, v.uuid, v.active),
    onSuccess: () => void invalidate(),
  });
  const remove = useAppMutation({
    mutationFn: (uuid: string) => automationService.remove(slug, uuid),
    onSuccess: () => {
      appToast.success(t("tenant.automation_deleted"));
      void invalidate();
    },
  });

  if (query.isLoading) return <Loading />;
  if (query.isError || !query.data) {
    return (
      <ErrorState
        title={t("common.error_generic")}
        onRetry={() => void query.refetch()}
      />
    );
  }
  const data = query.data;
  const rows = data.items.filter((r) =>
    filter === "all" ? true : filter === "active" ? r.active : !r.active,
  );

  const triggerLabel = (r: AutomationRule) => {
    if (r.trigger_kind === "event") {
      return `${t("tenant.automation_trigger_event")}: ${t(
        `tenant.automation_event_${r.trigger.event.replaceAll(".", "_")}`,
      )}`;
    }
    if (r.trigger_kind === "payment") {
      return `${t("tenant.automation_trigger_payment")}: ${r.trigger.payment_date}`;
    }
    if (r.trigger.mode === "before_due") {
      return t("tenant.automation_summary_before_due", {
        days: r.trigger.before_due_days,
      });
    }
    return t("tenant.automation_summary_days", {
      days: r.trigger.days
        .map((d) => (d === 31 ? t("tenant.automation_last_day") : String(d)))
        .join(", "),
    });
  };

  const targetLabel = (r: AutomationRule) => {
    const base =
      r.target_kind === "selected"
        ? t("tenant.automation_target_count", {
            count: r.target_engagements.length,
          })
        : t(`tenant.automation_target_${r.target_kind}`);
    return r.only_due
      ? `${base} · ${t("tenant.automation_only_due_short")}`
      : base;
  };

  const askDelete = async (r: AutomationRule) => {
    const ok = await confirmDelete({
      title: t("tenant.automation_delete_title"),
      description: t("tenant.automation_delete_description", { name: r.name }),
    });
    if (ok) remove.mutate(r.uuid);
  };

  const stats = [
    { label: t("tenant.automation_stat_total"), value: data.stats.total },
    { label: t("tenant.automation_stat_active"), value: data.stats.active },
    { label: t("tenant.automation_stat_sent"), value: data.stats.sent },
  ];

  return (
    <div className="space-y-4">
      <div className="grid gap-3 sm:grid-cols-3">
        {stats.map((s) => (
          <div key={s.label} className="border-line rounded-md border p-3">
            <div className="text-text-2 text-xs">{s.label}</div>
            <div className="text-xl font-semibold tabular-nums">{s.value}</div>
          </div>
        ))}
      </div>

      <div className="border-line flex items-center justify-between gap-3 rounded-md border p-3">
        <div>
          <div className="text-sm font-medium">
            {t("tenant.automation_master")}
          </div>
          <div className="text-text-2 text-xs">
            {t("tenant.automation_master_help")}
          </div>
        </div>
        <Switch
          aria-label={t("tenant.automation_master")}
          checked={data.enabled}
          disabled={!canWrite || setEnabled.isPending}
          onCheckedChange={(v) => setEnabled.mutate(v)}
        />
      </div>

      <div className="flex flex-wrap items-center justify-between gap-2">
        <TabStrip aria-label={t("tenant.automation_filter")}>
          {(["all", "active", "passive"] as const).map((f) => (
            <TabStripItem
              key={f}
              selected={filter === f}
              onClick={() => setFilter(f)}
            >
              {t(`tenant.automation_filter_${f}`)}
            </TabStripItem>
          ))}
        </TabStrip>
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" onClick={() => setDraftsOpen(true)}>
            {t("tenant.automation_drafts_button", {
              count: data.stats.drafts,
            })}
          </Button>
          <Button
            variant="outline"
            onClick={() => {
              setHistoryRule(undefined);
              setHistoryOpen(true);
            }}
          >
            {t("tenant.automation_runs_button")}
          </Button>
          {canWrite ? (
            <Button
              onClick={() => {
                setEditing(undefined);
                setRuleOpen(true);
              }}
            >
              <Plus className="size-4" aria-hidden />
              {t("tenant.automation_new")}
            </Button>
          ) : null}
        </div>
      </div>

      {rows.length === 0 ? (
        <p className="text-text-2 py-6 text-center text-sm">
          {t("tenant.automation_empty")}
        </p>
      ) : (
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>{t("tenant.automation_col_name")}</TableHead>
              <TableHead>{t("tenant.automation_trigger")}</TableHead>
              <TableHead>{t("tenant.automation_target")}</TableHead>
              <TableHead>{t("tenant.automation_col_text")}</TableHead>
              <TableHead>{t("tenant.automation_col_status")}</TableHead>
              <TableHead className="text-right">
                {t("tenant.automation_col_sent")}
              </TableHead>
              <TableHead className="text-right">
                <span className="sr-only">
                  {t("tenant.automation_col_actions")}
                </span>
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {rows.map((r) => (
              <TableRow key={r.uuid} data-testid="automation-row">
                <TableCell className="font-medium">
                  {r.name}
                  {r.approval_mode === "draft" ? (
                    <span className="text-text-2 block text-xs">
                      {t("tenant.automation_approval_draft")}
                    </span>
                  ) : null}
                </TableCell>
                <TableCell className="text-sm">{triggerLabel(r)}</TableCell>
                <TableCell className="text-sm">{targetLabel(r)}</TableCell>
                <TableCell className="text-text-2 max-w-64 truncate text-sm">
                  {r.body}
                </TableCell>
                <TableCell>
                  <div className="flex items-center gap-2">
                    <Switch
                      aria-label={t("tenant.automation_toggle", {
                        name: r.name,
                      })}
                      checked={r.active}
                      disabled={!canWrite || setActive.isPending}
                      onCheckedChange={(v) =>
                        setActive.mutate({ uuid: r.uuid, active: v })
                      }
                    />
                    <StatusPill tone={r.active ? "ok" : "neutral"}>
                      {t(
                        r.active
                          ? "tenant.automation_status_active"
                          : "tenant.automation_status_passive",
                      )}
                    </StatusPill>
                  </div>
                </TableCell>
                <TableCell className="text-right tabular-nums">
                  {r.send_count}
                </TableCell>
                <TableCell className="text-right whitespace-nowrap">
                  <Button
                    variant="ghost"
                    size="icon"
                    aria-label={t("tenant.automation_action_history")}
                    onClick={() => {
                      setHistoryRule(r.uuid);
                      setHistoryOpen(true);
                    }}
                  >
                    <History className="size-4" aria-hidden />
                  </Button>
                  {canWrite ? (
                    <>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t("tenant.automation_action_edit")}
                        onClick={() => {
                          setEditing(r);
                          setRuleOpen(true);
                        }}
                      >
                        <Pencil className="size-4" aria-hidden />
                      </Button>
                      <Button
                        variant="ghost"
                        size="icon"
                        aria-label={t("tenant.automation_action_delete")}
                        onClick={() => void askDelete(r)}
                      >
                        <Trash2 className="size-4" aria-hidden />
                      </Button>
                    </>
                  ) : null}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      )}

      <AutomationRuleDialog
        slug={slug}
        open={ruleOpen}
        onOpenChange={setRuleOpen}
        rule={editing}
        events={data.events}
      />
      <AutomationHistoryDialog
        slug={slug}
        open={historyOpen}
        onOpenChange={setHistoryOpen}
        ruleUuid={historyRule}
      />
      <AutomationDraftsDialog
        slug={slug}
        open={draftsOpen}
        onOpenChange={setDraftsOpen}
        canWrite={canWrite}
      />
    </div>
  );
}
