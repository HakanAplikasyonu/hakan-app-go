"use client";

import { ChevronLeft, ChevronRight, ExternalLink, X } from "lucide-react";
import Link from "next/link";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import { Fragment, useCallback, useEffect } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetHeader,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { useRelatedRecords } from "../hooks/use-related";
import { focusNeighbor } from "../lib/focus-step";
import {
  FOCUS_PARAM,
  panelTarget,
  parseFocus,
  relatedHref,
} from "../lib/related-href";
import type {
  RelatedRecord,
  RelatedSourceKind,
} from "../services/related.service";
import { BankTransactionMatchingPanel } from "./bank-transaction-matching";
import { ManualLinkForm, ManualUnlinkButton } from "./journal-manual-link";
import { Figure, Money, Panel, SeverityBadge } from "./office-ui";

/** Çeviri yoksa ham değer (bilinmeyen durum ya da öneri türü). */
function useTr() {
  const { t } = useLocale();
  return (key: string, fallback: string, vars?: Record<string, string>) => {
    const v = t(key, vars);
    return v === key ? fallback : v;
  };
}

function trDate(iso: string | null) {
  if (!iso) return "—";
  const [y, m, d] = iso.split("-");
  return y && m && d ? `${d}.${m}.${y}` : iso;
}

function RecordLabel({ item }: { item: RelatedRecord }) {
  const tr = useTr();
  if (item.kind === "suggestion")
    return <>{tr(`office.related.suggestion.${item.label}`, item.label)}</>;
  if (item.kind === "journal_draft")
    return (
      <>
        {item.detail
          ? tr("office.related.entry_no", item.detail, { no: item.detail })
          : null}
        {item.label ? ` · ${item.label}` : ""}
      </>
    );
  if (item.kind === "account")
    return (
      <>
        <Figure>{item.label}</Figure>
        {item.detail ? ` · ${item.detail}` : ""}
      </>
    );
  // Uyarı ve kural: kodu değil adı (#576); çevirisi yoksa kod.
  if (item.kind === "warning" || item.kind === "rule")
    return (
      <>
        {tr(
          `office.warning_group.${item.label}`,
          tr(`office.warning.${item.label}`, item.label),
        )}
      </>
    );
  return <>{item.label || "—"}</>;
}

function Detail({ item }: { item: RelatedRecord }) {
  const tr = useTr();
  if (!item.detail) return null;
  if (item.kind === "warning" || item.kind === "rule") {
    const sev = item.detail;
    return sev === "info" || sev === "warning" || sev === "blocker" ? (
      <SeverityBadge severity={sev} />
    ) : null;
  }
  if (item.kind === "bank_transaction")
    return (
      <span className="text-text-2 text-xs">
        {tr(`office.related.debit_credit.${item.detail}`, item.detail)}
      </span>
    );
  if (item.kind === "invoice" || item.kind === "expense")
    return (
      <span className="text-text-2 block max-w-64 truncate text-xs">
        {item.detail}
      </span>
    );
  return null;
}

/** İlişkili kaydı panelin içinde açan geri çağrı (#564). */
export type OpenRelated = (target: {
  kind: RelatedSourceKind;
  uuid: string;
  label: string;
}) => void;

/**
 * Bir kaydın ilişkili kayıtları (#469): tür, kayıt, tarih, tutar, durum.
 * Bağlantılar mükellef detayında açılır; sekme çubuğu kaybolmaz.
 * `onOpen` verilirse panelin gösterebildiği kayıtlar (banka hareketi,
 * fatura, Z raporu, masraf) aynı panelde açılır, kendi sayfası ikincil
 * bağlantı olarak kalır (#564).
 */
export function RelatedRecordsTable({
  engagement,
  kind,
  uuid,
  onOpen,
}: {
  engagement: string;
  kind: RelatedSourceKind;
  uuid: string;
  onOpen?: OpenRelated;
}) {
  const { can } = usePermission();
  // Fişe elle kayıt bağlama (#710): fişin ilişkili kayıtlarında.
  const linkable =
    kind === "journal_draft" && can(permissions.tenant.journals.write);
  const table = (
    <RelatedTable
      engagement={engagement}
      kind={kind}
      uuid={uuid}
      onOpen={onOpen}
      linkable={linkable}
    />
  );
  if (!linkable) return table;
  return (
    <>
      {table}
      <ManualLinkForm engagement={engagement} draft={uuid} />
    </>
  );
}

function RelatedTable({
  engagement,
  kind,
  uuid,
  onOpen,
  linkable,
}: {
  engagement: string;
  kind: RelatedSourceKind;
  uuid: string;
  onOpen?: OpenRelated;
  linkable: boolean;
}) {
  const { t } = useLocale();
  const tr = useTr();
  const { slug } = useTenant();
  const query = useRelatedRecords(slug, engagement, kind, uuid);

  if (query.isLoading) return <Skeleton className="m-4 h-24" />;
  if (query.isError)
    return (
      <p role="alert" className="text-danger p-4 text-sm">
        {t("office.related.load_error")}
      </p>
    );
  const all = query.data?.items ?? [];
  // Gerçek bağlar önce; tahminler (bekleyen öneri, onaylanmamış karşı
  // hesap) ayrı "Olası" başlığı altında, açıkça etiketli (#669).
  const items = all.filter((x) => x.relation !== "possible");
  const possible = all.filter((x) => x.relation === "possible");
  if (all.length === 0)
    return (
      <p className="text-text-2 p-4 text-sm" data-testid="related-empty">
        {t("office.related.empty")}
      </p>
    );
  return (
    <Table className="w-full text-sm" data-testid="related-records">
      <TableHeader className="text-xs">
        <TableRow className="border-line hover:bg-transparent">
          <TableHead
            scope="col"
            className="text-text-2 h-auto px-4 py-2 font-medium"
          >
            {t("office.related.col_kind")}
          </TableHead>
          <TableHead
            scope="col"
            className="text-text-2 h-auto px-4 py-2 font-medium"
          >
            {t("office.related.col_record")}
          </TableHead>
          <TableHead
            scope="col"
            className="text-text-2 h-auto px-4 py-2 font-medium"
          >
            {t("office.related.col_date")}
          </TableHead>
          <TableHead
            scope="col"
            className="text-text-2 h-auto px-4 py-2 text-right font-medium"
          >
            {t("office.related.col_amount")}
          </TableHead>
          <TableHead
            scope="col"
            className="text-text-2 h-auto px-4 py-2 font-medium"
          >
            {t("office.related.col_status")}
          </TableHead>
        </TableRow>
      </TableHeader>
      <TableBody>
        {items.length === 0 ? (
          <TableRow className="border-line hover:bg-transparent">
            <TableCell
              colSpan={5}
              className="text-text-2 px-4 py-2 text-sm"
              data-testid="related-empty"
            >
              {t("office.related.empty")}
            </TableCell>
          </TableRow>
        ) : null}
        {[...items, ...possible].map((item, i) => {
          // Uyarı ve kural adı; ham kod değil (#576).
          const recordName = (r: RelatedRecord) =>
            r.kind === "warning" || r.kind === "rule"
              ? tr(
                  `office.warning_group.${r.label}`,
                  tr(`office.warning.${r.label}`, r.label),
                )
              : r.label;
          const href = relatedHref(slug, engagement, item);
          const kindLabel = t(`office.related.kind.${item.kind}`);
          const firstPossible = i === items.length && possible.length > 0;
          return (
            <Fragment key={`${item.kind}-${item.uuid ?? item.label}-${i}`}>
              {firstPossible ? (
                <TableRow
                  className="border-line bg-surface-2 hover:bg-surface-2"
                  data-testid="related-possible-heading"
                >
                  <TableCell colSpan={5} className="px-4 py-2">
                    <div className="text-sm font-semibold">
                      {t("office.related.possible_title")}
                    </div>
                    <div className="text-text-2 text-xs">
                      {t("office.related.possible_hint")}
                    </div>
                  </TableCell>
                </TableRow>
              ) : null}
              <TableRow
                className="border-line hover:bg-transparent"
                data-testid={
                  item.relation === "possible"
                    ? "related-possible"
                    : "related-definite"
                }
              >
                <TableCell className="px-4 py-2 align-top whitespace-nowrap">
                  <div className="font-medium">{kindLabel}</div>
                  <div className="text-text-3 text-xs">
                    {t(`office.related.relation.${item.relation}`)}
                  </div>
                </TableCell>
                <TableCell className="px-4 py-2 align-top">
                  {onOpen && panelTarget(item) ? (
                    <span className="inline-flex items-center gap-1">
                      <Button
                        type="button"
                        variant="link"
                        className="text-brand-text h-auto p-0 text-left font-normal whitespace-normal"
                        aria-label={`${kindLabel}: ${recordName(item)}`}
                        data-testid="related-open-in-panel"
                        onClick={() => {
                          const target = panelTarget(item);
                          if (target)
                            onOpen({ ...target, label: recordName(item) });
                        }}
                      >
                        <RecordLabel item={item} />
                      </Button>
                      {href ? (
                        <Link
                          href={href}
                          className="text-text-3 hover:text-brand-text"
                          aria-label={t("office.related.open_page", {
                            record: `${kindLabel}: ${recordName(item)}`,
                          })}
                          title={t("office.related.open_page", {
                            record: kindLabel,
                          })}
                        >
                          <ExternalLink className="size-3.5" aria-hidden />
                        </Link>
                      ) : null}
                    </span>
                  ) : href ? (
                    <Link
                      href={href}
                      className="text-brand-text hover:underline"
                      aria-label={`${kindLabel}: ${recordName(item)}`}
                    >
                      <RecordLabel item={item} />
                    </Link>
                  ) : (
                    <RecordLabel item={item} />
                  )}
                  <Detail item={item} />
                </TableCell>
                <TableCell className="px-4 py-2 align-top whitespace-nowrap">
                  <Figure>{trDate(item.date)}</Figure>
                </TableCell>
                <TableCell className="px-4 py-2 text-right align-top whitespace-nowrap">
                  {item.amount ? (
                    <Money
                      value={item.amount}
                      currency={item.currency ?? "TRY"}
                    />
                  ) : (
                    "—"
                  )}
                </TableCell>
                <TableCell className="px-4 py-2 align-top whitespace-nowrap">
                  {tr(`office.related.status.${item.status}`, item.status)}
                  {linkable &&
                  item.relation === "manual" &&
                  item.uuid &&
                  (item.kind === "invoice" ||
                    item.kind === "bank_transaction" ||
                    item.kind === "customer") ? (
                    <div>
                      <ManualUnlinkButton
                        engagement={engagement}
                        draft={uuid}
                        kind={item.kind}
                        uuid={item.uuid}
                        label={recordName(item)}
                      />
                    </div>
                  ) : null}
                </TableCell>
              </TableRow>
            </Fragment>
          );
        })}
      </TableBody>
    </Table>
  );
}

/** Detay sayfalarındaki "İlişkili kayıtlar" bölümü. */
export function RelatedRecordsPanel({
  engagement,
  kind,
  uuid,
}: {
  engagement: string;
  kind: RelatedSourceKind;
  uuid: string;
}) {
  const { t } = useLocale();
  return (
    <Panel
      title={t("office.related.title")}
      description={t("office.related.description")}
    >
      <RelatedRecordsTable engagement={engagement} kind={kind} uuid={uuid} />
    </Panel>
  );
}

/**
 * Mükellef detayında `?kayit=<tür>:<uuid>` ile seçili kaydın ayrıntısı ve
 * ilişkili kayıtları (#469). Sağdan açılan, kipsiz yan panel (#564): liste
 * yerinde kalır, sayfa başa zıplamaz; başka satıra tıklayınca panel o
 * kayda geçer. Dışarı tıklamak paneli kapatmaz (satır seçimiyle
 * yarışmasın); "Seçimi kaldır", Esc ya da × kapatır.
 */
export function FocusedRecord({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const params = useSearchParams();
  const pathname = usePathname();
  const router = useRouter();
  const focus = parseFocus(params.get(FOCUS_PARAM));
  const rest = new URLSearchParams(params.toString());
  rest.delete(FOCUS_PARAM);
  const qs = rest.toString();
  const closeHref = `${pathname}${qs ? `?${qs}` : ""}`;
  const kind = focus?.kind;
  const uuid = focus?.uuid;
  // Listedeki önceki / sonraki kayda geç (#564): komşu, sayfadaki satır
  // bağlantılarından okunur; liste yerinde kalır, panel o kayda geçer.
  const step = useCallback(
    (dir: 1 | -1) => {
      if (!kind || !uuid) return;
      const panel = document.querySelector('[data-testid="focused-record"]');
      const hrefs = Array.from(
        document.querySelectorAll<HTMLAnchorElement>(
          `a[href*="${FOCUS_PARAM}="]`,
        ),
      )
        .filter((a) => !panel?.contains(a))
        .map((a) => a.getAttribute("href") ?? "");
      const next = focusNeighbor(hrefs, { kind, uuid }, dir);
      if (next) router.replace(next, { scroll: false });
      else toast.info(t("office.related.focus_edge"));
    },
    [kind, uuid, router, t],
  );
  useEffect(() => {
    if (!kind) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.altKey || e.ctrlKey || e.metaKey) return;
      const el = e.target;
      if (
        el instanceof Element &&
        el.closest("input, textarea, select, [contenteditable=true]")
      )
        return;
      if (e.key === "j") step(1);
      else if (e.key === "k") step(-1);
      else return;
      e.preventDefault();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [kind, step]);
  return (
    <Sheet
      open={focus !== null}
      modal={false}
      onOpenChange={(o) => {
        if (!o) router.replace(closeHref, { scroll: false });
      }}
    >
      <SheetContent
        side="right"
        className="w-full overflow-y-auto sm:max-w-2xl"
        data-testid="focused-record"
        onInteractOutside={(e) => e.preventDefault()}
      >
        {focus ? (
          <div className="space-y-5">
            <SheetHeader className="text-left">
              <div className="flex flex-wrap items-center justify-between gap-2 pe-8">
                <SheetTitle id="focused-record-title">
                  {t("office.related.focus_title", {
                    kind: t(`office.related.kind.${focus.kind}`),
                  })}
                </SheetTitle>
                <div className="flex items-center gap-1">
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t("office.related.focus_prev")}
                    aria-keyshortcuts="k"
                    onClick={() => step(-1)}
                  >
                    <ChevronLeft className="size-4" aria-hidden />
                  </Button>
                  <Button
                    size="icon"
                    variant="ghost"
                    aria-label={t("office.related.focus_next")}
                    aria-keyshortcuts="j"
                    onClick={() => step(1)}
                  >
                    <ChevronRight className="size-4" aria-hidden />
                  </Button>
                  <Button asChild size="sm" variant="ghost">
                    <Link href={closeHref} scroll={false}>
                      <X className="size-4" aria-hidden />
                      {t("office.related.focus_close")}
                    </Link>
                  </Button>
                </div>
              </div>
              <SheetDescription>
                {t("office.related.description")}
              </SheetDescription>
            </SheetHeader>
            {focus.kind === "bank_transaction" && (
              <BankTransactionMatchingPanel
                engagement={engagement}
                transaction={focus.uuid}
              />
            )}
            <RelatedRecordsTable
              engagement={engagement}
              kind={focus.kind}
              uuid={focus.uuid}
            />
          </div>
        ) : null}
      </SheetContent>
    </Sheet>
  );
}
