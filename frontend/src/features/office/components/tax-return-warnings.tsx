"use client";

import { ExternalLink, PanelRightOpen } from "lucide-react";
import Link from "next/link";
import { useEffect, useMemo, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { useWarnings } from "../hooks/use-warnings";
import { warningRecordHref, warningRecordKind } from "../lib/related-href";
import type { RelatedSourceKind } from "../services/related.service";
import type { ApiDataWarning as DataWarning } from "../services/warnings.service";
import { BankTransactionMatchingPanel } from "./bank-transaction-matching";
import { InvoiceQuickView } from "./invoice-quick-view";
import { SeverityBadge } from "./office-ui";
import { RelatedRecordsTable } from "./related-records";
import { ApiWarningActions } from "./warning-actions";
import { useApiWarningText, useEntityLabel } from "./warnings-page";

/** Bir kaydın uyarıları; kaydı olmayanlar (dönem geneli) tek grupta. */
export type WarningGroup = {
  key: string;
  entityType: string;
  entityUuid: string | null;
  kind: RelatedSourceKind | null;
  items: DataWarning[];
};

/**
 * Uyarıları kayda göre gruplar (#528): aynı kaydın sorunları tek satırda
 * düzeltilir. Sıra: en ağır uyarısı olan kayıt önce, dönem geneli sonda.
 */
export function groupWarnings(items: DataWarning[]): WarningGroup[] {
  const rank: Record<string, number> = { blocker: 0, warning: 1, info: 2 };
  const groups = new Map<string, WarningGroup>();
  for (const w of items) {
    const uuid = w.entity_uuid ?? null;
    const kind = uuid ? warningRecordKind(w.entity_type) : null;
    // Kalem uyarısı faturasının grubuna düşer (uuid fatura uuid'si).
    const type = kind === "invoice" ? "invoice" : w.entity_type;
    const key = uuid ? `${type}:${uuid}` : "period";
    const g = groups.get(key) ?? {
      key,
      entityType: uuid ? type : "period",
      entityUuid: uuid,
      kind,
      items: [],
    };
    g.items.push(w);
    groups.set(key, g);
  }
  const worst = (g: WarningGroup) =>
    Math.min(...g.items.map((w) => rank[w.severity] ?? 3));
  return [...groups.values()].sort((a, b) => {
    if (a.key === "period" || b.key === "period")
      return a.key === "period" ? 1 : -1;
    return worst(a) - worst(b) || b.items.length - a.items.length;
  });
}

/**
 * Beyanı etkileyen açık uyarılar (#528): kayıt kayıt gruplu; her kayıt
 * aynı ekranda yan panelde açılır (banka hareketinde fatura eşleştirme,
 * ilişkili kayıtlar, uyarı kararları). Uyarılar değişince `onChanged`
 * beyanı yeniden hesaplatır; sorun kalmayınca "Beyanı hazırla" açılır.
 */
export function TaxReturnWarnings({
  engagement,
  period,
  codes,
  onChanged,
}: {
  engagement: string;
  period: string;
  codes: string[];
  onChanged?: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const text = useApiWarningText();
  const entity = useEntityLabel();
  // Açık panelin grubu: uyarıları çözülüp listeden düşse de panel açık
  // kalır ("uyarı kalmadı" der).
  const [open, setOpen] = useState<WarningGroup | null>(null);
  const q = useWarnings(
    slug,
    { period, status: "open", limit: 100, offset: 0 },
    engagement,
  );

  // Bir karar ya da düzeltme uyarı listesini yenilediğinde beyan da
  // yeniden hesaplanır (ilk yükleme hariç).
  const seen = useRef<number | null>(null);
  useEffect(() => {
    if (!q.dataUpdatedAt) return;
    if (seen.current !== null && seen.current !== q.dataUpdatedAt)
      onChanged?.();
    seen.current = q.dataUpdatedAt;
  }, [q.dataUpdatedAt, onChanged]);

  const groups = useMemo(
    () =>
      groupWarnings(
        (q.data?.items ?? []).filter((w) => codes.includes(w.code)),
      ),
    [q.data, codes],
  );
  if (codes.length === 0) return null;
  const all = routes.tenant.engagementWarnings(slug, engagement);
  const current = open
    ? (groups.find((g) => g.key === open.key) ?? { ...open, items: [] })
    : null;

  return (
    <section
      aria-labelledby="tr-warnings"
      className="border-warn bg-surface rounded-xl border p-4"
    >
      <div className="mb-3 flex flex-wrap items-baseline justify-between gap-2">
        <div>
          <h3 id="tr-warnings" className="font-display text-base font-semibold">
            {t("office.tax_returns.blocking_title", { count: codes.length })}
          </h3>
          <p className="text-text-3 text-xs">
            {t("office.tax_returns.blocking_hint")}
          </p>
        </div>
        <Link href={all} className="text-brand text-sm hover:underline">
          {t("office.tax_returns.blocking_all")}
        </Link>
      </div>
      {q.isLoading ? (
        <p className="text-text-3 text-sm">{t("common.loading")}</p>
      ) : groups.length === 0 ? (
        <p className="text-text-3 text-sm">
          {t("office.tax_returns.blocking_none")}
        </p>
      ) : (
        <ul className="divide-line divide-y" data-testid="tax-return-problems">
          {groups.map((g) => (
            <li key={g.key} className="py-3">
              <div className="mb-1.5 flex flex-wrap items-center gap-2">
                <span className="text-sm font-semibold">
                  {g.key === "period"
                    ? t("office.tax_returns.problem_period")
                    : entity(g.entityType)}
                </span>
                <span className="text-text-3 text-xs">
                  {t("office.tax_returns.problem_count", {
                    count: g.items.length,
                  })}
                </span>
                <GroupOpen
                  group={g}
                  engagement={engagement}
                  onOpen={() => setOpen(g)}
                />
              </div>
              <WarningRows items={g.items} text={text} />
            </li>
          ))}
        </ul>
      )}

      <Sheet open={current !== null} onOpenChange={(o) => !o && setOpen(null)}>
        <SheetContent
          side="right"
          className="w-full overflow-y-auto sm:max-w-2xl"
          data-testid="tax-return-record-panel"
        >
          {current && current.kind && current.entityUuid ? (
            <RecordPanel
              group={current}
              kind={current.kind}
              uuid={current.entityUuid}
              engagement={engagement}
              onChanged={onChanged}
            />
          ) : null}
        </SheetContent>
      </Sheet>
    </section>
  );
}

function GroupOpen({
  group,
  engagement,
  onOpen,
}: {
  group: WarningGroup;
  engagement: string;
  onOpen: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const entity = useEntityLabel();
  const record = entity(group.entityType);
  if (group.kind && group.entityUuid)
    return (
      <Button
        size="sm"
        variant="outline"
        className="ms-auto max-md:min-h-11"
        onClick={onOpen}
      >
        <PanelRightOpen className="size-4" aria-hidden />
        {t("office.tax_returns.open_fix", { record })}
      </Button>
    );
  // Fiş: inceleme kuyruğunda düzeltilir.
  const first = group.items[0];
  const href = first
    ? warningRecordHref(slug, { ...first, engagement_uuid: engagement })
    : null;
  return href ? (
    <Link
      href={href}
      className="text-brand ms-auto text-sm font-medium hover:underline"
    >
      {t("office.warnings_api.open_record", { record })}
    </Link>
  ) : null;
}

function WarningRows({
  items,
  text,
}: {
  items: DataWarning[];
  text: (w: DataWarning) => string;
}) {
  return (
    <ul className="space-y-1.5">
      {items.map((w) => (
        <li key={w.uuid} className="flex flex-wrap items-center gap-3">
          <SeverityBadge severity={w.severity} />
          <span className="min-w-56 flex-1 text-sm">{text(w)}</span>
          <ApiWarningActions warning={w} />
        </li>
      ))}
    </ul>
  );
}

/**
 * Yan panel: kaydın kendisi (banka hareketinde fatura eşleştirme),
 * uyarıları ve ilişkili kayıtları; sekmeye gitmeden düzeltilir.
 */
function RecordPanel({
  group,
  kind,
  uuid,
  engagement,
  onChanged,
}: {
  group: WarningGroup;
  kind: RelatedSourceKind;
  uuid: string;
  engagement: string;
  onChanged?: () => void;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const text = useApiWarningText();
  const entity = useEntityLabel();
  const record = entity(group.entityType);
  const href =
    kind === "invoice"
      ? routes.tenant.engagementInvoice(slug, engagement, uuid)
      : warningRecordHref(slug, {
          engagement_uuid: engagement,
          entity_type: group.entityType,
          entity_uuid: uuid,
        });
  return (
    <div className="space-y-5">
      <SheetHeader className="text-left">
        <SheetTitle>
          {t("office.tax_returns.panel_title", { record })}
        </SheetTitle>
        <SheetDescription>
          {t("office.tax_returns.panel_hint")}
        </SheetDescription>
      </SheetHeader>

      {kind === "bank_transaction" ? (
        <BankTransactionMatchingPanel
          engagement={engagement}
          transaction={uuid}
          onChanged={onChanged}
        />
      ) : null}
      {kind === "invoice" ? (
        <InvoiceQuickView engagement={engagement} invoice={uuid} />
      ) : null}

      <section className="space-y-2">
        <h4 className="text-sm font-semibold">
          {t("office.tax_returns.panel_warnings")}
        </h4>
        {group.items.length === 0 ? (
          <p className="text-text-3 text-sm">
            {t("office.tax_returns.panel_no_warnings")}
          </p>
        ) : (
          <WarningRows items={group.items} text={text} />
        )}
      </section>

      <section className="space-y-2">
        <h4 className="text-sm font-semibold">{t("office.related.title")}</h4>
        <RelatedRecordsTable engagement={engagement} kind={kind} uuid={uuid} />
      </section>

      {href ? (
        <Button asChild variant="outline" size="sm">
          <Link href={href}>
            <ExternalLink className="size-4" aria-hidden />
            {t("office.tax_returns.panel_open_tab", { record })}
          </Link>
        </Button>
      ) : null}
    </div>
  );
}
