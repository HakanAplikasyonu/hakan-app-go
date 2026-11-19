"use client";

import type { ColumnDef, RowSelectionState } from "@tanstack/react-table";
import { ArrowRight, ListChecks, Play, X } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { useEffect, useMemo, useState } from "react";

import { Label } from "@/components/ui/label";
import { EntityTable } from "@/components/entity";
import { createColumn, createSelectColumnDef } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Input } from "@/components/ui/input";
import { Switch } from "@/components/ui/switch";
import { routes } from "@/config/routes";
import { useOfficePeriod } from "@/features/office/period";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useCopilotWorkflow } from "../hooks/use-copilot";
import {
  ACTION_PERMISSIONS,
  actionHref,
  isWriteAction,
  stepHref,
} from "../lib/actions";
import {
  nextActionLabel,
  periodLabel,
  stepDetail,
  stepLabel,
} from "../lib/labels";
import { bulkSteps, fromTextField, nextStepOf } from "../lib/next-steps";
import type {
  CopilotStepKey,
  CopilotWorkflowRow,
} from "../services/copilot.service";
import { CommandBar } from "./command-bar";
import { RecentPlans } from "./recent-plans";
import { useCopilot } from "./copilot-provider";
import { StepChain } from "./step-chain";

type Row = CopilotWorkflowRow;

const PAGE_SIZE = 100;

/** `?adim=documents`: yalnız o adımı bekleyen mükellefler (#562). */
export const STEP_PARAM = "adim";
const STEP_KEYS: CopilotStepKey[] = ["documents", "bank", "review"];

/**
 * `/t/{slug}/copilot` (#392): "Sıradaki iş". Every engagement walks the
 * period close chain; the next step is one click (and one approval) away,
 * one by one or in bulk, with keyboard shortcuts. The command bar plans a
 * sentence into approved steps.
 */
export function CopilotPage() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const router = useRouter();
  const { focus } = useOfficePeriod();
  const copilot = useCopilot();
  const [search, setSearch] = useState("");
  const [onlyOpen, setOnlyOpen] = useState(true);
  const [selection, setSelection] = useState<RowSelectionState>({});
  const [activeRaw, setActive] = useState(0);
  const query = useCopilotWorkflow(slug, {
    period: focus,
    search,
    limit: PAGE_SIZE,
    offset: 0,
  });
  const params = useSearchParams();
  const pathname = usePathname();
  const rawStep = params.get(STEP_PARAM) as CopilotStepKey | null;
  const stepFilter = rawStep && STEP_KEYS.includes(rawStep) ? rawStep : null;
  const all = useMemo(() => query.data?.items ?? [], [query.data]);
  const rows = useMemo(
    () =>
      all.filter(
        (r) =>
          (!onlyOpen || r.next_step) &&
          (!stepFilter ||
            r.steps.some((s) => s.key === stepFilter && s.status === "todo")),
      ),
    [all, onlyOpen, stepFilter],
  );
  // The keyboard cursor stays on the list when it shrinks.
  const active = Math.max(0, Math.min(activeRaw, rows.length - 1));
  const closed = all.filter((r) => !r.next_step).length;
  const selectedRows = rows.filter((r) => selection[r.engagement_uuid]);
  const bulk = bulkSteps(selectedRows);
  const allowed = (action: Row["steps"][number]["action"]) =>
    !!action && ACTION_PERMISSIONS[action].every((p) => can(p));

  const runNext = (row: Row) => {
    const step = nextStepOf(row);
    if (!step?.action) return;
    if (!isWriteAction(step.action)) {
      router.push(actionHref(slug, step.action, row.engagement_uuid));
      return;
    }
    copilot.review(bulkSteps([row]));
  };

  // j / k (or ↓ / ↑) move, Enter runs the next step, x selects, b runs the
  // selection; never while typing.
  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.metaKey || e.ctrlKey || e.altKey || fromTextField(e.target)) return;
      if (document.querySelector("[role=dialog]")) return;
      const row = rows[active];
      switch (e.key) {
        case "j":
        case "ArrowDown":
          e.preventDefault();
          setActive(Math.min(rows.length - 1, active + 1));
          break;
        case "k":
        case "ArrowUp":
          e.preventDefault();
          setActive(Math.max(0, active - 1));
          break;
        case "Enter":
          if (
            row &&
            (e.target === document.body ||
              e.target instanceof HTMLTableRowElement)
          ) {
            e.preventDefault();
            runNext(row);
          }
          break;
        case "x":
          if (row) {
            e.preventDefault();
            setSelection((s) => ({
              ...s,
              [row.engagement_uuid]: !s[row.engagement_uuid],
            }));
          }
          break;
        case "b":
          if (bulk.length > 0) {
            e.preventDefault();
            copilot.review(bulk);
          }
          break;
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  });

  const columns = useMemo<ColumnDef<Row>[]>(
    () => [
      createSelectColumnDef<Row>(),
      createColumn<Row>({
        accessorKey: "taxpayer_title",
        labelKey: "office.copilot.col_taxpayer",
        cell: ({ row }) => (
          <div className="min-w-0">
            <Link
              className="font-medium hover:underline"
              href={routes.tenant.engagement(
                slug,
                row.original.engagement_uuid,
              )}
            >
              {row.original.taxpayer_title}
            </Link>
            <p className="text-text-3 font-mono text-xs tabular-nums">
              {row.original.taxpayer_tax_id}
            </p>
          </div>
        ),
      }),
      createColumn<Row>({
        id: "chain",
        labelKey: "office.copilot.col_chain",
        enableSorting: false,
        cell: ({ row }) => (
          <div className="space-y-1">
            <StepChain
              steps={row.original.steps}
              next={row.original.next_step}
              hrefFor={(k) => stepHref(slug, k, row.original.engagement_uuid)}
            />
            <p className="text-text-3 text-xs tabular-nums">
              {t("office.copilot.progress", {
                done: row.original.done_steps,
                total: row.original.active_steps,
              })}
            </p>
          </div>
        ),
      }),
      createColumn<Row>({
        id: "next",
        labelKey: "office.copilot.col_next",
        enableSorting: false,
        cell: ({ row }) => {
          const step = nextStepOf(row.original);
          if (!step?.action) {
            return (
              <span className="text-ok text-sm">
                {row.original.steps.some((s) => s.status === "waiting")
                  ? t("office.copilot.waiting_only")
                  : t("office.copilot.closed")}
              </span>
            );
          }
          const write = isWriteAction(step.action);
          return (
            <div className="flex flex-wrap items-center justify-between gap-2">
              <div className="min-w-0">
                <p className="text-sm font-medium">{stepLabel(t, step.key)}</p>
                <p className="text-text-2 text-xs">
                  {stepDetail(t, step.key, step.status, step.count)}
                </p>
              </div>
              <Button
                size="sm"
                variant={write ? "default" : "outline"}
                disabled={write && !allowed(step.action)}
                onClick={(e) => {
                  e.stopPropagation();
                  runNext(row.original);
                }}
              >
                {write ? (
                  <Play aria-hidden className="size-3.5" />
                ) : (
                  <ArrowRight aria-hidden className="size-3.5" />
                )}
                {nextActionLabel(t, step.action)}
              </Button>
            </div>
          );
        },
      }),
    ],
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [slug, t, can],
  );

  return (
    <div className="space-y-5">
      <div>
        <h1 className="font-display text-2xl font-bold">
          {t("office.copilot.title")}
        </h1>
        <p className="text-text-2 text-sm">
          {t("office.copilot.description", {
            period: periodLabel(locale, focus),
          })}
        </p>
      </div>
      <Card>
        <CardContent className="space-y-5">
          <CommandBar />
          <RecentPlans />
        </CardContent>
      </Card>
      <Card>
        <CardHeader className="space-y-3">
          <CardTitle className="text-base">
            {t("office.copilot.list_title", {
              period: periodLabel(locale, focus),
            })}
          </CardTitle>
          <div className="flex flex-wrap items-center justify-between gap-3">
            <div className="flex flex-wrap items-center gap-3">
              <Input
                type="search"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder={t("office.copilot.search")}
                aria-label={t("office.copilot.search")}
                className="w-64 max-w-full"
              />
              <Label className="flex items-center gap-2 text-sm leading-normal font-normal">
                <Switch checked={onlyOpen} onCheckedChange={setOnlyOpen} />
                {t("office.copilot.only_open")}
              </Label>
              {stepFilter ? (
                <Button
                  size="sm"
                  variant="secondary"
                  onClick={() => router.replace(pathname, { scroll: false })}
                >
                  {t("office.copilot.step_filter", {
                    step: stepLabel(t, stepFilter),
                  })}
                  <X aria-hidden className="size-3.5" />
                </Button>
              ) : null}
              {closed > 0 ? (
                <span className="text-text-3 text-xs">
                  {t("office.copilot.closed_count", { count: closed })}
                </span>
              ) : null}
            </div>
            <Button
              variant="secondary"
              disabled={bulk.length === 0}
              onClick={() => copilot.review(bulk)}
            >
              <ListChecks aria-hidden className="size-4" />
              {t("office.copilot.bulk_run", { count: bulk.length })}
            </Button>
          </div>
        </CardHeader>
        <CardContent className="space-y-3">
          <EntityTable
            columns={columns}
            data={rows}
            getRowId={(row) => row.engagement_uuid}
            isLoading={query.isLoading}
            isError={query.isError}
            onRetry={() => void query.refetch()}
            emptyTitle={
              onlyOpen && all.length > 0
                ? t("office.copilot.empty_all_closed")
                : t("office.copilot.empty_title")
            }
            emptyDescription={t("office.copilot.empty_description")}
            manual={{ filtering: false, sorting: false, pagination: false }}
            isRowActive={(row) =>
              rows[active]?.engagement_uuid === row.engagement_uuid
            }
            state={{
              rowSelection: selection,
              onRowSelectionChange: setSelection,
            }}
            features={{ persistKey: "office-copilot-v1", rowSelection: true }}
          />
          <p className="text-text-3 text-xs">{t("office.copilot.shortcuts")}</p>
        </CardContent>
      </Card>
    </div>
  );
}
