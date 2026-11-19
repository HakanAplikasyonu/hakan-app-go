"use client";

import { Loader2, Save, Search } from "lucide-react";
import { useState } from "react";

import { EmptyState } from "@/components/common/empty-state";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Skeleton } from "@/components/ui/skeleton";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  useJournalActions,
  useJournalDocuments,
  useJournalSettings,
  useOfficeJournalSettings,
} from "../hooks/use-journals";
import {
  LUCA_INTEGRATORS,
  type JournalExportCheck,
  type LucaIntegrator,
} from "../services/journals.service";
import {
  dateText,
  DOUBLE_BOOKING_CODES,
  doubleBookingText,
  str,
} from "../lib/double-booking";
import { Figure, Panel } from "./office-ui";

export { DOUBLE_BOOKING_CODES };

/**
 * Entries the double booking guard holds back (#466), with a checkbox per
 * document: a checked document is confirmed for this export.
 */
export function DoubleBookingReview({
  check,
  confirmed,
  onToggle,
}: {
  check: JournalExportCheck;
  confirmed: Set<string>;
  onToggle: (key: string, on: boolean) => void;
}) {
  const { t, locale } = useLocale();
  if (!check.excluded.length) return null;
  const issues = check.issues.filter((i) => DOUBLE_BOOKING_CODES.has(i.code));
  return (
    <div className="border-status-warning/40 space-y-2 rounded-md border p-3 text-sm">
      <p className="text-status-warning font-medium">
        {t("office.journals.dup.title", { count: check.excluded.length })}
      </p>
      <p className="text-muted-foreground text-xs">
        {t("office.journals.dup.hint")}
      </p>
      <ul className="space-y-2">
        {check.excluded.map((x) => (
          <li key={x.journal_draft_uuid} className="space-y-1">
            <p className="text-xs font-medium">
              {t("office.journals.dup.entry", {
                entry: `${x.period} / ${x.entry_no}`,
              })}
            </p>
            {x.documents.map((key) => {
              const mine = issues.filter(
                (i) =>
                  i.entry_no === x.entry_no &&
                  str(i.params?.document_key) === key,
              );
              if (!mine.length) return null;
              const no = str(mine[0]?.params?.document_no);
              const id = `dup-${x.journal_draft_uuid}-${key}`;
              return (
                <div key={key} className="flex items-start gap-2 ps-2">
                  <Checkbox
                    id={id}
                    checked={confirmed.has(key)}
                    onCheckedChange={(v) => onToggle(key, v === true)}
                    className="mt-0.5"
                  />
                  <Label
                    htmlFor={id}
                    className="space-y-0.5 text-xs leading-normal font-normal"
                  >
                    <span className="block">
                      {no ? <Figure>{no}</Figure> : null}{" "}
                      <span className="text-muted-foreground">
                        {t("office.journals.dup.ettn")} <Figure>{key}</Figure>
                      </span>
                    </span>
                    {mine.map((i) => (
                      <span key={i.code} className="text-status-warning block">
                        {doubleBookingText(t, locale, i)}
                      </span>
                    ))}
                    <span className="block">
                      {t("office.journals.dup.confirm")}
                    </span>
                  </Label>
                </div>
              );
            })}
          </li>
        ))}
      </ul>
    </div>
  );
}

function IntegratorBoxes({
  idPrefix,
  value,
  disabled,
  onChange,
}: {
  idPrefix: string;
  value: string[];
  disabled?: boolean;
  onChange: (v: LucaIntegrator[]) => void;
}) {
  const { t } = useLocale();
  return (
    <div className="flex flex-wrap gap-4">
      {LUCA_INTEGRATORS.map((name) => {
        const id = `${idPrefix}-${name}`;
        return (
          <Label
            key={name}
            htmlFor={id}
            className="flex items-center gap-2 text-xs leading-normal font-normal"
          >
            <Checkbox
              id={id}
              disabled={disabled}
              checked={value.includes(name)}
              onCheckedChange={(on) =>
                onChange(
                  LUCA_INTEGRATORS.filter((n) =>
                    n === name ? on === true : value.includes(n),
                  ),
                )
              }
            />
            {t(`office.journals.integrator_${name}`)}
          </Label>
        );
      })}
    </div>
  );
}

/**
 * "Luca bu mükellefin e-faturalarını kendisi çekiyor" (#466): the office
 * setting and the taxpayer's own choice (or "use the office setting").
 */
export function LucaPullSettingsPanel({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const office = useOfficeJournalSettings(slug);
  const settings = useJournalSettings(slug, engagement);
  const { saveSettings } = useJournalActions(slug, engagement);
  const [officeDraft, setOfficeDraft] = useState<string[] | null>(null);
  const [ownDraft, setOwnDraft] = useState<string[] | null | undefined>(
    undefined,
  );

  if (office.query.isLoading || settings.isLoading) {
    return <Skeleton className="h-32 w-full" />;
  }
  const officeValue =
    officeDraft ?? office.query.data?.luca_pull_integrators ?? [];
  const saved = settings.data;
  // undefined: untouched; null: office setting; list: taxpayer's own.
  const own =
    ownDraft !== undefined ? ownDraft : (saved?.luca_pull_integrators ?? null);
  const effective = own ?? officeValue;
  const error = office.save.error ?? saveSettings.error;

  return (
    <Panel
      title={t("office.journals.pull.title")}
      description={t("office.journals.pull.description")}
    >
      <div className="space-y-4 p-4 text-sm">
        <div className="space-y-2">
          <p className="text-xs font-medium">
            {t("office.journals.pull.office")}
          </p>
          <IntegratorBoxes
            idPrefix="pull-office"
            value={officeValue}
            onChange={setOfficeDraft}
          />
          <Button
            size="sm"
            variant="outline"
            disabled={officeDraft === null || office.save.isPending}
            onClick={() =>
              office.save.mutate(
                { luca_pull_integrators: officeValue as LucaIntegrator[] },
                { onSuccess: () => setOfficeDraft(null) },
              )
            }
          >
            {office.save.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Save className="size-4" />
            )}
            {t("office.journals.pull.save_office")}
          </Button>
        </div>
        <div className="space-y-2 border-t pt-3">
          <p className="text-xs font-medium">
            {t("office.journals.pull.taxpayer")}
          </p>
          <Label
            htmlFor="pull-own"
            className="flex items-center gap-2 text-xs leading-normal font-normal"
          >
            <Checkbox
              id="pull-own"
              checked={own !== null}
              onCheckedChange={(on) =>
                setOwnDraft(on === true ? [...officeValue] : null)
              }
            />
            {t("office.journals.pull.own")}
          </Label>
          <IntegratorBoxes
            idPrefix="pull-taxpayer"
            value={effective}
            disabled={own === null}
            onChange={setOwnDraft}
          />
          <p className="text-muted-foreground text-xs">
            {effective.length
              ? t("office.journals.pull.effective", {
                  list: effective
                    .map((n) => t(`office.journals.integrator_${n}`))
                    .join(", "),
                })
              : t("office.journals.pull.effective_none")}
          </p>
          <Button
            size="sm"
            variant="outline"
            disabled={
              ownDraft === undefined || saveSettings.isPending || !saved
            }
            onClick={() =>
              saved &&
              saveSettings.mutate(
                {
                  invoices_reach_luca_directly:
                    saved.invoices_reach_luca_directly,
                  block_on_unknown_account: saved.block_on_unknown_account,
                  luca_pull_integrators:
                    (own as LucaIntegrator[] | null) ?? null,
                },
                { onSuccess: () => setOwnDraft(undefined) },
              )
            }
          >
            {saveSettings.isPending ? (
              <Loader2 className="size-4 animate-spin" />
            ) : (
              <Save className="size-4" />
            )}
            {t("office.journals.pull.save_taxpayer")}
          </Button>
        </div>
        {error ? (
          <p className="text-status-blocking text-xs">
            {t("office.journals.pull.save_error")}
          </p>
        ) : null}
      </div>
    </Panel>
  );
}

/** Document → file trail of the export history (#466), searchable by ETTN
 * or document number. */
export function ExportDocumentsPanel({ engagement }: { engagement: string }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const [text, setText] = useState("");
  const [q, setQ] = useState("");
  const docs = useJournalDocuments(slug, engagement, q);
  const items = docs.data?.items ?? [];
  const fmt = (iso: string) => dateText(locale, iso);
  return (
    <Panel
      title={t("office.journals.docs.title")}
      description={t("office.journals.docs.description")}
    >
      <div className="space-y-3 p-4">
        <form
          className="flex max-w-md gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            setQ(text.trim());
          }}
        >
          <Input
            className="h-8"
            placeholder={t("office.journals.docs.search")}
            aria-label={t("office.journals.docs.search")}
            value={text}
            onChange={(e) => setText(e.target.value)}
          />
          <Button size="sm" variant="outline" type="submit">
            <Search className="size-4" />
            {t("office.journals.docs.search_button")}
          </Button>
        </form>
        {docs.isLoading ? (
          <Skeleton className="h-24" />
        ) : items.length === 0 ? (
          <EmptyState title={t("office.journals.docs.empty")} />
        ) : (
          <ul className="divide-y text-xs">
            {items.map((d) => (
              <li
                key={`${d.batch_uuid}-${d.journal_draft_uuid}-${d.document_key}`}
                className="flex flex-wrap items-center justify-between gap-2 py-2"
              >
                <span className="space-y-0.5">
                  <span className="block">
                    {d.document_no ? (
                      <Figure className="font-medium">{d.document_no}</Figure>
                    ) : null}{" "}
                    {d.ettn ? (
                      <span className="text-muted-foreground">
                        {t("office.journals.dup.ettn")}{" "}
                        <Figure>{d.ettn}</Figure>
                      </span>
                    ) : null}
                  </span>
                  <span className="text-muted-foreground block">
                    {t("office.journals.docs.row", {
                      file: d.file_name,
                      entry: `${d.period} / ${d.entry_no}`,
                      date: fmt(d.exported_at),
                    })}
                  </span>
                  {d.confirmed_codes.length ? (
                    <span className="text-status-warning block">
                      {t("office.journals.docs.confirmed", {
                        date: d.confirmed_at ? fmt(d.confirmed_at) : "",
                      })}
                    </span>
                  ) : null}
                </span>
                {d.reversed_at ? (
                  <span className="text-muted-foreground">
                    {t("office.journals.docs.reversed")}
                  </span>
                ) : null}
              </li>
            ))}
          </ul>
        )}
      </div>
    </Panel>
  );
}
