"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { FolderInput } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Suspense, useMemo, useState } from "react";
import { toast } from "sonner";

import { PageHeader } from "@/components/layout/page-header";
import { StatusPill } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { smsKeys, smsService } from "@/features/tenant/services/sms.service";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { usePeriodMatrixAt } from "../hooks/use-office-data";
import { usePendingDocTasks } from "../hooks/use-pending-doc-tasks";
import { currentPeriod, usePeriodText, useUrlOfficePeriod } from "../period";
import {
  documentCollectionService,
  type MissingDocumentItem,
  type MissingDocumentsRequestResult,
  type MissingRequestInput,
} from "../services/document-collection.service";
import type {
  PeriodMatrixCell,
  PeriodMatrixRow,
} from "../services/periods.service";
import { Panel } from "./office-ui";

type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

/** Eksik belgenin ekrandaki adı ("Ziraat ekstresi", "Z raporları"). */
export function missingLabel(item: MissingDocumentItem, t: Translate) {
  return t(`office.collection.kind_${item.kind}`, { name: item.name });
}

/** Satırın istenmemiş eksikleri. */
function openItems(cell: PeriodMatrixCell | undefined) {
  return (cell?.missing_items ?? []).filter((i) => !i.requested);
}

/** Ofisin SMS'i açık mı (Ayarlar > SMS); okuma izni yoksa SMS sunulmaz. */
function useSmsAvailable() {
  const { slug } = useTenant();
  const { can } = usePermission();
  const q = useQuery({
    queryKey: smsKeys.settings(slug),
    queryFn: () => smsService.settings(slug),
    enabled: can(permissions.tenant.settings.read),
    staleTime: 5 * 60_000,
  });
  return {
    available: !!q.data?.enabled,
    testMode: !!q.data && (q.data.test_mode || !q.data.live_allowed),
  };
}

/** İstek sonrası toast: kaç istek açıldı, kaçı zaten açıktı, SMS ne oldu. */
function useResultToast() {
  const { t } = useLocale();
  return (results: MissingDocumentsRequestResult[]) => {
    const created = results.reduce((n, r) => n + r.created.length, 0);
    const existing = results.reduce((n, r) => n + r.existing.length, 0);
    const parts: string[] = [];
    parts.push(
      created > 0
        ? t("office.collection.result_created", { count: created })
        : t("office.collection.result_none"),
    );
    if (existing > 0)
      parts.push(t("office.collection.result_existing", { count: existing }));
    const sms = results.map((r) => r.sms).filter((s) => !!s);
    if (sms.length > 0) {
      const sent = sms.filter(
        (s) => s.status === "sent" || s.status === "pending",
      ).length;
      const dry = sms.filter((s) => s.status === "dry_run").length;
      const noPhone = sms.filter((s) => s.code === "no_contact").length;
      if (sent) parts.push(t("office.collection.sms_sent", { count: sent }));
      if (dry) parts.push(t("office.collection.sms_dry_run", { count: dry }));
      if (noPhone)
        parts.push(t("office.collection.sms_no_contact", { count: noPhone }));
    }
    toast.success(parts.join(" "));
  };
}

/** İstek sonrası matris ve görev listeleri tazelenir. */
function useRefresh() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return (engagement?: string) => {
    const key = slug.toLowerCase();
    void qc.invalidateQueries({
      queryKey: ["tenant", key, "periods", "matrix"],
    });
    void qc.invalidateQueries({
      queryKey: engagement
        ? ["tenant", key, "doc-tasks", engagement]
        : ["tenant", key, "doc-tasks"],
    });
  };
}

/** Kanal seçimi: uygulama + e-posta her zaman; SMS ofiste açıksa. */
function ChannelChoice({
  id,
  sms,
  onSms,
}: {
  id: string;
  sms: boolean;
  onSms: (v: boolean) => void;
}) {
  const { t } = useLocale();
  const { available, testMode } = useSmsAvailable();
  return (
    <div
      className="flex flex-wrap items-center gap-x-4 gap-y-2 text-sm"
      data-testid="collection-channel"
    >
      <span className="text-text-2">{t("office.collection.channel_base")}</span>
      {available ? (
        <span className="flex items-center gap-2">
          <Switch
            id={id}
            checked={sms}
            onCheckedChange={onSms}
            data-testid="collection-sms"
          />
          <Label htmlFor={id}>
            {testMode
              ? t("office.collection.channel_sms_test")
              : t("office.collection.channel_sms")}
          </Label>
        </span>
      ) : (
        <span className="text-text-3 text-xs">
          {t("office.collection.channel_sms_off")}
        </span>
      )}
    </div>
  );
}

/** Bir eksik belge: adı ve "İste" ya da "İstendi". */
function MissingChip({
  item,
  canWrite,
  busy,
  onRequest,
}: {
  item: MissingDocumentItem;
  canWrite: boolean;
  busy: boolean;
  onRequest: () => void;
}) {
  const { t } = useLocale();
  return (
    <span
      className={cn(
        "inline-flex items-center gap-2 rounded-md border px-2 py-1 text-xs",
        item.requested
          ? "border-line bg-surface-2 text-text-2"
          : "border-warn bg-warn-tint",
      )}
      data-testid="missing-chip"
      data-kind={item.kind}
      data-requested={item.requested ? "true" : "false"}
    >
      <span>{missingLabel(item, t)}</span>
      {item.requested ? (
        <span className="text-text-3">{t("office.collection.requested")}</span>
      ) : canWrite ? (
        <Button
          size="sm"
          variant="link"
          className="h-auto p-0 text-xs"
          disabled={busy}
          onClick={onRequest}
          data-testid="missing-chip-request"
        >
          {t("office.collection.request_one")}
        </Button>
      ) : null}
    </span>
  );
}

type Row = PeriodMatrixRow & { cell: PeriodMatrixCell };

/** `?engagement=<uuid>`: belge toplama tek mükellefe süzülür (Sıradaki iş). */
const COLLECTION_ENGAGEMENT_PARAM = "engagement";

/**
 * Süzülen mükellefin bekleyen belge istekleri: her satır isteğin kendisine
 * (mükellef özetinde vurgulu satır) götürür, genel listeye değil.
 */
function EngagementOpenRequests({
  engagement,
  period,
}: {
  engagement: string;
  period: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const tasks = usePendingDocTasks(engagement, period);
  const items = tasks.data ?? [];
  if (items.length === 0) return null;
  const title = (x: (typeof items)[number]) =>
    x.kind === "undocumented_payment" || x.kind === "uninvoiced_collection"
      ? t(`office.engagement.task_${x.kind}`)
      : x.title || t("office.engagement.task_generic");
  return (
    <section
      aria-labelledby="collection-requests-title"
      className="border-line rounded-lg border p-3"
      data-testid="collection-open-requests"
    >
      <h3 id="collection-requests-title" className="text-sm font-semibold">
        {t("office.collection.open_requests_title", { count: items.length })}
      </h3>
      <p className="text-text-2 mb-2 text-xs">
        {t("office.collection.open_requests_hint")}
      </p>
      <ul className="divide-line divide-y">
        {items.map((x) => (
          <li
            key={x.uuid}
            className="flex flex-wrap items-center justify-between gap-2 py-2"
          >
            <span className="min-w-0 text-sm">
              <span className="block truncate">{title(x)}</span>
              <span className="text-text-3 text-xs">
                {x.status === "submitted"
                  ? t("office.engagement.task_submitted")
                  : x.overdue
                    ? t("office.engagement.task_overdue")
                    : t("office.engagement.task_waiting")}
              </span>
            </span>
            <Button asChild size="sm" variant="outline">
              <Link
                href={routes.tenant.engagementDocTask(slug, engagement, x.uuid)}
                data-testid="collection-open-request"
              >
                {t("office.collection.open_request")}
              </Link>
            </Button>
          </li>
        ))}
      </ul>
    </section>
  );
}

/**
 * `/t/{slug}/document-collection` (#780, ayın 1–5'i): seçili dönemde eksik
 * belgesi olan mükellefler; her eksik için "İste", satırda "Eksiklerin
 * hepsini iste", üstte seçilenlerden toplu istek ve kanal seçimi.
 */
export function OfficeDocumentCollectionPage() {
  return (
    <Suspense fallback={null}>
      <CollectionPage />
    </Suspense>
  );
}

function CollectionPage() {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.docTasks.write);
  const periodText = usePeriodText();
  const { focus: period } = useUrlOfficePeriod();
  const matrix = usePeriodMatrixAt(period);
  const [sms, setSms] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [busy, setBusy] = useState<string | null>(null);
  const showResult = useResultToast();
  const refresh = useRefresh();
  const { available: smsAvailable } = useSmsAvailable();
  // Sıradaki iş'ten "Eksik belgelere bak": yalnız o mükellef.
  const params = useSearchParams();
  const router = useRouter();
  const pathname = usePathname();
  const only = params.get(COLLECTION_ENGAGEMENT_PARAM);
  const onlyTitle = only
    ? matrix.data?.items.find((r) => r.engagement_uuid === only)
        ?.taxpayer_title
    : undefined;
  const clearOnly = () => {
    const next = new URLSearchParams(params.toString());
    next.delete(COLLECTION_ENGAGEMENT_PARAM);
    const qs = next.toString();
    router.replace(qs ? `${pathname}?${qs}` : pathname, { scroll: false });
  };

  const rows = useMemo<Row[]>(
    () =>
      (matrix.data?.items ?? [])
        .filter((r) => !only || r.engagement_uuid === only)
        .map((r) => ({ ...r, cell: r.cells.find((c) => c.period === period) }))
        .filter((r): r is Row => !!r.cell && r.cell.missing_documents > 0)
        .sort(
          (a, b) =>
            openItems(b.cell).length - openItems(a.cell).length ||
            a.taxpayer_title.localeCompare(b.taxpayer_title, "tr"),
        ),
    [matrix.data, period, only],
  );
  const totals = useMemo(() => {
    let items = 0;
    let requested = 0;
    for (const r of rows) {
      items += r.cell.missing_items.length;
      requested += r.cell.missing_items.filter((i) => i.requested).length;
    }
    return { items, requested };
  }, [rows]);
  const requestable = rows.filter((r) => openItems(r.cell).length > 0);
  const chosen = requestable.filter((r) => selected.has(r.engagement_uuid));
  const openMonth = period >= currentPeriod();

  const send = useMutation({
    mutationFn: async (
      jobs: { engagement: string; body: MissingRequestInput }[],
    ) => {
      const out: MissingDocumentsRequestResult[] = [];
      // Sırayla: ofisin SMS hızı ve sunucu yükü için.
      for (const j of jobs) {
        out.push(
          await documentCollectionService.request(slug, j.engagement, j.body),
        );
      }
      return out;
    },
    onSuccess: (results, jobs) => {
      showResult(results);
      setSelected(new Set());
      refresh(jobs.length === 1 ? jobs[0].engagement : undefined);
    },
    onError: () => toast.error(t("office.api.load_error")),
    onSettled: () => setBusy(null),
  });
  const run = (
    key: string,
    jobs: { engagement: string; body: MissingRequestInput }[],
  ) => {
    if (jobs.length === 0) return;
    setBusy(key);
    send.mutate(jobs);
  };
  const smsOn = sms && smsAvailable;

  return (
    <div className="space-y-5" data-testid="document-collection-page">
      <PageHeader
        title={t("office.collection.title")}
        description={t("office.collection.description", {
          period: periodText(period),
        })}
        icon={<FolderInput className="size-6" />}
      />
      {openMonth ? (
        <p className="border-line bg-surface-2 text-text-2 rounded-lg border p-3 text-sm">
          {t("office.collection.open_month", { period: periodText(period) })}
        </p>
      ) : null}
      <Panel
        title={
          matrix.data
            ? t("office.collection.summary", {
                taxpayers: rows.length,
                items: totals.items,
                requested: totals.requested,
              })
            : t("office.collection.title")
        }
        actions={
          canWrite ? (
            <Button
              size="sm"
              disabled={chosen.length === 0 || send.isPending}
              data-testid="collection-request-selected"
              onClick={() =>
                run(
                  "selected",
                  chosen.map((r) => ({
                    engagement: r.engagement_uuid,
                    body: { period, sms: smsOn },
                  })),
                )
              }
            >
              {t("office.collection.request_selected", {
                count: chosen.length,
              })}
            </Button>
          ) : null
        }
      >
        <div className="space-y-3 p-4">
          {only ? (
            <div
              className="bg-surface-2 border-line flex flex-wrap items-center justify-between gap-2 rounded-lg border p-3 text-sm"
              data-testid="collection-only"
            >
              <span>
                {t("office.collection.only_taxpayer", {
                  name: onlyTitle ?? t("office.collection.only_unknown"),
                })}
              </span>
              <Button
                size="sm"
                variant="secondary"
                onClick={clearOnly}
                data-testid="collection-only-clear"
              >
                {t("office.collection.only_clear")}
              </Button>
            </div>
          ) : null}
          {only ? (
            <EngagementOpenRequests engagement={only} period={period} />
          ) : null}
          {canWrite ? (
            <ChannelChoice id="collection-sms" sms={sms} onSms={setSms} />
          ) : null}
          {matrix.isLoading ? (
            <p className="text-text-3 text-sm">{t("common.loading")}</p>
          ) : matrix.isError ? (
            <p role="alert" className="text-danger text-sm">
              {t("office.load_failed")}
            </p>
          ) : rows.length === 0 ? (
            <p className="text-text-3 text-sm" data-testid="collection-empty">
              {only
                ? t("office.collection.empty_taxpayer", {
                    name: onlyTitle ?? t("office.collection.only_unknown"),
                    period: periodText(period),
                  })
                : t("office.collection.empty", { period: periodText(period) })}
            </p>
          ) : (
            <Table data-testid="collection-table">
              <TableCaption className="sr-only">
                {t("office.collection.title")}
              </TableCaption>
              <TableHeader>
                <TableRow>
                  <TableHead scope="col" className="w-10">
                    {canWrite ? (
                      <Checkbox
                        aria-label={t("office.collection.select_all")}
                        checked={
                          requestable.length > 0 &&
                          chosen.length === requestable.length
                        }
                        onCheckedChange={(v) =>
                          setSelected(
                            v
                              ? new Set(
                                  requestable.map((r) => r.engagement_uuid),
                                )
                              : new Set(),
                          )
                        }
                      />
                    ) : null}
                  </TableHead>
                  <TableHead scope="col">
                    {t("office.collection.col_taxpayer")}
                  </TableHead>
                  <TableHead scope="col">
                    {t("office.collection.col_missing")}
                  </TableHead>
                  <TableHead scope="col">
                    <span className="sr-only">
                      {t("office.collection.col_action")}
                    </span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {rows.map((r) => {
                  const open = openItems(r.cell);
                  const e = r.engagement_uuid;
                  return (
                    <TableRow key={e} data-testid="collection-row">
                      <TableCell>
                        {canWrite && open.length > 0 ? (
                          <Checkbox
                            aria-label={t("office.collection.select_row", {
                              name: r.taxpayer_title,
                            })}
                            checked={selected.has(e)}
                            onCheckedChange={(v) =>
                              setSelected((prev) => {
                                const next = new Set(prev);
                                if (v) next.add(e);
                                else next.delete(e);
                                return next;
                              })
                            }
                          />
                        ) : null}
                      </TableCell>
                      <TableCell className="align-top">
                        <Link
                          href={routes.tenant.engagement(slug, e)}
                          className="font-medium underline-offset-2 hover:underline"
                        >
                          {r.taxpayer_title}
                        </Link>
                        <span className="text-text-3 block font-mono text-xs">
                          {r.taxpayer_tax_id}
                        </span>
                      </TableCell>
                      <TableCell>
                        <span className="flex flex-wrap gap-2">
                          {r.cell.missing_items.map((item) => (
                            <MissingChip
                              key={item.title}
                              item={item}
                              canWrite={canWrite}
                              busy={send.isPending}
                              onRequest={() =>
                                run(`${e}:${item.title}`, [
                                  {
                                    engagement: e,
                                    body: {
                                      period,
                                      sms: smsOn,
                                      items: [
                                        { kind: item.kind, name: item.name },
                                      ],
                                    },
                                  },
                                ])
                              }
                            />
                          ))}
                        </span>
                      </TableCell>
                      <TableCell className="text-right">
                        {canWrite && open.length > 1 ? (
                          <Button
                            size="sm"
                            variant="outline"
                            disabled={send.isPending}
                            data-testid="collection-request-row"
                            onClick={() =>
                              run(e, [
                                { engagement: e, body: { period, sms: smsOn } },
                              ])
                            }
                          >
                            {busy === e
                              ? t("common.loading")
                              : t("office.collection.request_all")}
                          </Button>
                        ) : open.length === 0 ? (
                          <StatusPill tone="ok">
                            {t("office.collection.all_requested")}
                          </StatusPill>
                        ) : null}
                      </TableCell>
                    </TableRow>
                  );
                })}
              </TableBody>
            </Table>
          )}
        </div>
      </Panel>
    </div>
  );
}

/**
 * Pano ve beyannameler ekranındaki şerit: seçili dönemde istenmemiş eksik
 * belge varsa sayısı ve belge toplamaya bağlantı (#780).
 */
export function DocumentCollectionBanner({
  rows,
  period,
}: {
  rows: PeriodMatrixRow[];
  period: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const periodText = usePeriodText();
  let taxpayers = 0;
  let items = 0;
  for (const r of rows) {
    const n = openItems(r.cells.find((c) => c.period === period)).length;
    if (n > 0) {
      taxpayers++;
      items += n;
    }
  }
  if (items === 0) return null;
  return (
    <section
      data-testid="document-collection-banner"
      aria-labelledby="document-collection-banner-title"
      className="border-warn bg-warn-tint flex flex-wrap items-center gap-3 rounded-xl border p-3"
    >
      <FolderInput aria-hidden className="text-warn size-5 shrink-0" />
      <div className="min-w-0 flex-1">
        <h2
          id="document-collection-banner-title"
          className="text-sm font-semibold"
        >
          {t("office.collection.banner_title", {
            period: periodText(period),
            taxpayers,
            items,
          })}
        </h2>
        <p className="text-text-2 text-sm">
          {t("office.collection.banner_hint")}
        </p>
      </div>
      <Button asChild variant="outline" size="sm">
        <Link href={routes.tenant.documentCollection(slug, period)}>
          {t("office.collection.open")}
        </Link>
      </Button>
    </section>
  );
}

/**
 * Mükellef özetinde seçili dönemin eksik belgeleri (#780): aynı "İste" ve
 * "Eksiklerin hepsini iste" düğmeleri. Eksik yoksa görünmez.
 */
export function EngagementMissingDocuments({
  engagement,
  cell,
}: {
  engagement: string;
  cell: PeriodMatrixCell | undefined;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canWrite = can(permissions.tenant.docTasks.write);
  const periodText = usePeriodText();
  const [sms, setSms] = useState(false);
  const { available } = useSmsAvailable();
  const showResult = useResultToast();
  const refresh = useRefresh();
  const send = useMutation({
    mutationFn: (body: MissingRequestInput) =>
      documentCollectionService.request(slug, engagement, body),
    onSuccess: (res) => {
      showResult([res]);
      refresh(engagement);
    },
    onError: () => toast.error(t("office.api.load_error")),
  });
  const items = cell?.missing_items ?? [];
  if (!cell || items.length === 0) return null;
  const open = openItems(cell);
  const smsOn = sms && available;
  return (
    <section
      aria-labelledby="missing-documents-title"
      className="bg-surface border-line rounded-xl border p-4 md:p-5"
      data-testid="engagement-missing-documents"
    >
      <div className="mb-3 flex flex-wrap items-center justify-between gap-2">
        <h2
          id="missing-documents-title"
          className="font-display text-base font-bold"
        >
          {t("office.collection.engagement_title", {
            period: periodText(cell.period),
          })}
        </h2>
        {canWrite && open.length > 1 ? (
          <Button
            size="sm"
            variant="outline"
            disabled={send.isPending}
            data-testid="engagement-missing-request-all"
            onClick={() => send.mutate({ period: cell.period, sms: smsOn })}
          >
            {t("office.collection.request_all")}
          </Button>
        ) : null}
      </div>
      <p className="text-text-2 mb-3 text-sm">
        {t("office.collection.engagement_hint")}
      </p>
      <div className="mb-3 flex flex-wrap gap-2">
        {items.map((item) => (
          <MissingChip
            key={item.title}
            item={item}
            canWrite={canWrite}
            busy={send.isPending}
            onRequest={() =>
              send.mutate({
                period: cell.period,
                sms: smsOn,
                items: [{ kind: item.kind, name: item.name }],
              })
            }
          />
        ))}
      </div>
      {canWrite && open.length > 0 ? (
        <ChannelChoice
          id={`missing-sms-${engagement}`}
          sms={sms}
          onSms={setSms}
        />
      ) : null}
    </section>
  );
}
