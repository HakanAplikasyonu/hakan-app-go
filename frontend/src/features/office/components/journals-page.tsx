"use client";

import {
  BookOpenCheck,
  Check,
  Download,
  FileSpreadsheet,
  Loader2,
  Plus,
  Trash2,
  Undo2,
} from "lucide-react";
import { Suspense, useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { PageHeader } from "@/components/layout/page-header";
import { Button } from "@/components/ui/button";
import { DatePicker } from "@/components/ui/date-picker";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import {
  useExportDrivers,
  useJournalActions,
  useJournalDrafts,
  useJournalExports,
} from "../hooks/use-journals";
import { sub, sum, toMinor } from "../lib/decimal";
import {
  journalService,
  type JournalDraft,
} from "../services/journals.service";
import { EngagementPicker, useSelectedEngagement } from "./engagement-picker";
import { FilterSelect, Figure, Money, Panel, TableBox } from "./office-ui";

type LineRow = {
  account: string;
  description: string;
  debit: string;
  credit: string;
};
const emptyLine = (): LineRow => ({
  account: "",
  description: "",
  debit: "",
  credit: "",
});
const thisMonth = () => new Date().toISOString().slice(0, 7);

const statusClass: Record<JournalDraft["status"], string> = {
  draft: "bg-secondary text-secondary-foreground",
  approved: "bg-status-approved/12 text-status-approved",
  exported: "bg-brand/12 text-brand",
};

type Translate = (key: string) => string;

/** API error in Turkish when the code is known, else the server message (#161). */
function errorText(t: Translate, err: unknown, fallback: string) {
  if (isApiError(err)) {
    const key = `office.journals.error_${err.code}`;
    const msg = err.code ? t(key) : key;
    return msg !== key ? msg : err.message || fallback;
  }
  return err instanceof Error ? err.message : fallback;
}

/** Program label for an export driver id; the id itself when unknown. */
function driverLabel(t: Translate, name: string) {
  const key = `office.journals.driver_${name}`;
  const msg = t(key);
  return msg === key ? name : msg;
}

function NewDraft({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { create } = useJournalActions(slug, engagement);
  const [date, setDate] = useState(() => new Date().toISOString().slice(0, 10));
  const [description, setDescription] = useState("");
  const [lines, setLines] = useState<LineRow[]>([emptyLine(), emptyLine()]);
  const debit = sum(lines.map((l) => l.debit || "0"));
  const credit = sum(lines.map((l) => l.credit || "0"));
  const balanced = toMinor(debit) === toMinor(credit) && toMinor(debit) > 0;
  const set = (i: number, k: keyof LineRow, v: string) =>
    setLines((ls) => ls.map((l, j) => (j === i ? { ...l, [k]: v } : l)));

  const submit = () => {
    create.mutate(
      {
        entry_type: "mahsup",
        entry_date: `${date}T00:00:00Z`,
        description,
        lines: lines
          .filter((l) => l.account.trim())
          .map((l) => ({
            account: l.account.trim(),
            description: l.description,
            debit: l.debit || "0",
            credit: l.credit || "0",
          })),
      },
      {
        onSuccess: () => {
          setDescription("");
          setLines([emptyLine(), emptyLine()]);
        },
      },
    );
  };

  return (
    <Panel title={t("office.journals.new_title")}>
      <div className="space-y-3 p-4">
        <div className="flex flex-wrap gap-3">
          <label className="flex items-center gap-2 text-xs">
            <span className="text-muted-foreground">
              {t("office.col_date")}
            </span>
            <DatePicker
              className="h-8 w-40"
              value={date}
              onChange={(v) => setDate(v)}
            />
          </label>
          <Input
            className="h-8 max-w-md flex-1"
            placeholder={t("office.journals.description")}
            aria-label={t("office.journals.description")}
            value={description}
            onChange={(e) => setDescription(e.target.value)}
          />
        </div>
        <TableBox>
          <thead>
            <tr>
              <th>{t("office.col_account")}</th>
              <th>{t("office.journals.line_description")}</th>
              <th className="text-right">{t("office.journals.debit")}</th>
              <th className="text-right">{t("office.journals.credit")}</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {lines.map((l, i) => (
              <tr key={i}>
                <td>
                  <Input
                    className="h-8 w-36 font-mono"
                    placeholder="770.01"
                    aria-label={`${t("office.col_account")} ${i + 1}`}
                    value={l.account}
                    onChange={(e) => set(i, "account", e.target.value)}
                  />
                </td>
                <td>
                  <Input
                    className="h-8"
                    aria-label={`${t("office.journals.line_description")} ${i + 1}`}
                    value={l.description}
                    onChange={(e) => set(i, "description", e.target.value)}
                  />
                </td>
                <td className="text-right">
                  <Input
                    className="h-8 w-32 text-right font-mono"
                    inputMode="decimal"
                    aria-label={`${t("office.journals.debit")} ${i + 1}`}
                    value={l.debit}
                    onChange={(e) => set(i, "debit", e.target.value)}
                  />
                </td>
                <td className="text-right">
                  <Input
                    className="h-8 w-32 text-right font-mono"
                    inputMode="decimal"
                    aria-label={`${t("office.journals.credit")} ${i + 1}`}
                    value={l.credit}
                    onChange={(e) => set(i, "credit", e.target.value)}
                  />
                </td>
                <td>
                  <Button
                    variant="ghost"
                    size="sm"
                    aria-label={t("office.journals.remove_line")}
                    disabled={lines.length <= 2}
                    onClick={() =>
                      setLines((ls) => ls.filter((_, j) => j !== i))
                    }
                  >
                    <Trash2 className="size-4" />
                  </Button>
                </td>
              </tr>
            ))}
          </tbody>
        </TableBox>
        <div className="flex flex-wrap items-center justify-between gap-3">
          <Button
            variant="outline"
            size="sm"
            onClick={() => setLines((ls) => [...ls, emptyLine()])}
          >
            <Plus className="size-4" />
            {t("office.journals.add_line")}
          </Button>
          <div className="flex items-center gap-3 text-xs">
            <span
              className={cn(
                balanced ? "text-status-approved" : "text-status-warning",
              )}
            >
              {balanced
                ? t("office.journals.balanced")
                : t("office.journals.difference", {
                    amount: sub(debit, credit),
                  })}
            </span>
            <Figure>
              <Money value={debit} /> / <Money value={credit} />
            </Figure>
            <Button size="sm" onClick={submit} disabled={create.isPending}>
              {create.isPending ? (
                <Loader2 className="size-4 animate-spin" />
              ) : (
                <Plus className="size-4" />
              )}
              {t("office.journals.create")}
            </Button>
          </div>
        </div>
        {create.isError ? (
          <p className="text-status-blocking text-sm">
            {errorText(t, create.error, t("office.api.load_error"))}
          </p>
        ) : null}
        <p className="text-muted-foreground text-xs">
          {t("office.journals.new_hint")}
        </p>
      </div>
    </Panel>
  );
}

function ExportPanel({
  engagement,
  period,
}: {
  engagement: string;
  period: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const drivers = useExportDrivers(slug);
  const { export: run } = useJournalActions(slug, engagement);
  const items = drivers.data?.items ?? [];
  const [driver, setDriver] = useState("luca");
  const chosen = items.find((d) => d.name === driver);
  const blocked =
    run.isError && isApiError(run.error) && run.error.code === "EXPORT_BLOCKED"
      ? run.error.details
      : [];

  return (
    <Panel
      title={t("office.journals.export_title")}
      description={t("office.journals.export_description", {
        period: period || "—",
      })}
    >
      <div className="space-y-3 p-4">
        <div className="flex flex-wrap items-center gap-3">
          <FilterSelect
            label={t("office.journals.driver")}
            value={driver}
            onChange={setDriver}
            options={items.map((d) => ({
              value: d.name,
              label: driverLabel(t, d.name),
            }))}
          />
          {chosen && !chosen.verified ? (
            <span className="bg-status-warning/12 text-status-warning rounded-md px-2 py-0.5 text-xs">
              {t("office.journals.template_unverified", {
                version: chosen.template_version,
              })}
            </span>
          ) : null}
          <Button
            size="sm"
            disabled={!period || run.isPending || !chosen}
            onClick={() => run.mutate({ driver, period })}
          >
            {run.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <FileSpreadsheet className="size-4" />
            )}
            {t("office.journals.export")}
          </Button>
        </div>
        {!period ? (
          <p className="text-muted-foreground text-xs">
            {t("office.journals.pick_period")}
          </p>
        ) : null}
        {blocked.length ? (
          <div className="border-status-blocking/40 rounded-md border p-3 text-sm">
            <p className="text-status-blocking mb-1 font-medium">
              {t("office.journals.blocked")}
            </p>
            <ul className="space-y-0.5 text-xs">
              {blocked.map((d, i) => (
                <li key={i}>
                  <Figure>{d.code}</Figure> {d.field ? `· ${d.field}` : ""}{" "}
                  <span className="text-muted-foreground">{d.message}</span>
                </li>
              ))}
            </ul>
          </div>
        ) : run.isError ? (
          <p className="text-status-blocking text-sm">
            {errorText(t, run.error, t("office.api.load_error"))}
          </p>
        ) : null}
        {run.data ? (
          <div className="text-sm">
            <p className="text-status-approved">
              {t("office.journals.exported", { count: run.data.entries })}
            </p>
            <ul className="mt-1 space-y-1">
              {run.data.files.map((f) => (
                <li key={f.name}>
                  <Button
                    variant="outline"
                    size="sm"
                    onClick={() =>
                      void journalService.download(
                        slug,
                        engagement,
                        run.data.uuid,
                        f.name,
                      )
                    }
                  >
                    <Download className="size-4" />
                    {f.name}
                  </Button>
                </li>
              ))}
            </ul>
          </div>
        ) : null}
      </div>
    </Panel>
  );
}

function JournalsInner() {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { options, selected, select, isLoading, isError } =
    useSelectedEngagement(slug);
  const engagement = selected?.engagement ?? "";
  const [period, setPeriod] = useState(thisMonth);
  const drafts = useJournalDrafts(slug, engagement, period);
  const exports = useJournalExports(slug, engagement);
  const { approve, reverse } = useJournalActions(slug, engagement);
  const fmt = (d: string) =>
    new Date(d).toLocaleDateString(locale === "tr" ? "tr-TR" : "en-GB");

  const header = (
    <PageHeader
      title={t("office.journals.title")}
      description={t("office.journals.page_description")}
      icon={<BookOpenCheck className="size-6" />}
    />
  );
  if (isLoading) {
    return (
      <div className="space-y-4">
        {header}
        <Skeleton className="h-64 w-full" />
      </div>
    );
  }
  if (isError || options.length === 0) {
    return (
      <div className="space-y-4">
        {header}
        <EmptyState
          title={
            isError
              ? t("office.api.load_error")
              : t("office.api.no_engagements")
          }
        />
      </div>
    );
  }

  const rows = drafts.data?.items ?? [];
  const actionError = approve.error ?? reverse.error;
  return (
    <div className="space-y-4">
      {header}
      <div className="flex flex-wrap items-center gap-3">
        <EngagementPicker
          options={options}
          selected={selected}
          onSelect={select}
        />
        <label className="flex items-center gap-2 text-xs">
          <span className="text-muted-foreground">
            {t("office.api.col_period")}
          </span>
          <Input
            type="month"
            className="h-8 w-40"
            value={period}
            onChange={(e) => setPeriod(e.target.value)}
          />
        </label>
      </div>

      <NewDraft engagement={engagement} />

      <Panel title={t("office.journals.drafts_title")}>
        {actionError ? (
          <p className="text-status-blocking px-4 pt-3 text-sm">
            {errorText(t, actionError, t("office.api.load_error"))}
          </p>
        ) : null}
        {drafts.isLoading ? (
          <Skeleton className="m-4 h-32" />
        ) : rows.length === 0 ? (
          <EmptyState className="m-4" title={t("office.journals.empty")} />
        ) : (
          <TableBox>
            <thead>
              <tr>
                <th>{t("office.journals.entry_no")}</th>
                <th>{t("office.col_date")}</th>
                <th>{t("office.col_type")}</th>
                <th className="text-right">{t("office.journals.debit")}</th>
                <th className="text-right">{t("office.journals.credit")}</th>
                <th>{t("office.col_status")}</th>
                <th />
              </tr>
            </thead>
            <tbody>
              {rows.map((d) => (
                <tr key={d.uuid}>
                  <td>
                    <Figure>
                      {d.period} / {d.entry_no}
                    </Figure>
                  </td>
                  <td>
                    <Figure>{fmt(d.entry_date)}</Figure>
                  </td>
                  <td className="text-xs">{d.entry_type}</td>
                  <td className="text-right">
                    <Money value={d.debit} />
                  </td>
                  <td className="text-right">
                    <Money value={d.credit} />
                  </td>
                  <td>
                    <span
                      className={cn(
                        "rounded-md px-2 py-0.5 text-xs font-medium",
                        statusClass[d.status],
                      )}
                    >
                      {t(`office.journals.status_${d.status}`)}
                    </span>
                  </td>
                  <td className="text-right">
                    {d.status === "draft" ? (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={approve.isPending}
                        onClick={() => approve.mutate(d.uuid)}
                      >
                        <Check className="size-4" />
                        {t("office.journals.approve")}
                      </Button>
                    ) : d.status === "exported" ? (
                      <Button
                        size="sm"
                        variant="ghost"
                        disabled={reverse.isPending}
                        onClick={() => {
                          const reason = window.prompt(
                            t("office.journals.reverse_prompt"),
                          );
                          if (reason?.trim())
                            reverse.mutate({ draft: d.uuid, reason });
                        }}
                      >
                        <Undo2 className="size-4" />
                        {t("office.journals.reverse")}
                      </Button>
                    ) : null}
                  </td>
                </tr>
              ))}
            </tbody>
          </TableBox>
        )}
      </Panel>

      <ExportPanel engagement={engagement} period={period} />

      {exports.data?.items.length ? (
        <Panel title={t("office.journals.history_title")}>
          <ul className="divide-y text-sm">
            {exports.data.items.map((b) => (
              <li
                key={b.uuid}
                className="flex flex-wrap items-center justify-between gap-2 px-4 py-2"
              >
                <span>
                  <Figure className="font-medium">{b.driver}</Figure>{" "}
                  <span className="text-muted-foreground text-xs">
                    {b.period ?? ""} ·{" "}
                    {t("office.journals.history_row", {
                      entries: b.entries,
                      reversed: b.reversed,
                    })}{" "}
                    · <Figure>{fmt(b.created_at)}</Figure>
                  </span>
                </span>
                <span className="flex flex-wrap gap-1">
                  {b.files.map((f) => (
                    <Button
                      key={f.name}
                      variant="ghost"
                      size="sm"
                      onClick={() =>
                        void journalService.download(
                          slug,
                          engagement,
                          b.uuid,
                          f.name,
                        )
                      }
                    >
                      <Download className="size-4" />
                      {f.name}
                    </Button>
                  ))}
                </span>
              </li>
            ))}
          </ul>
        </Panel>
      ) : null}
    </div>
  );
}

export function JournalsPage() {
  return (
    <Suspense fallback={<Skeleton className="h-64 w-full" />}>
      <JournalsInner />
    </Suspense>
  );
}
