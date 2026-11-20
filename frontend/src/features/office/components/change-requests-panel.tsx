"use client";

import { ChevronDown, ChevronUp, FileClock } from "lucide-react";
import Link from "next/link";
import { useState } from "react";
import { toast } from "sonner";

import { StatusPill, formatCellDate } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  readDiff,
  useChangeRequests,
  type ChangeRequest,
} from "../hooks/use-change-requests";

/**
 * Kilitli döneme gelen değişiklik talepleri (#518, B2): dönem şeridinin
 * altında. Kilitli bir döneme yeni ya da değişmiş fatura gelince kayıt
 * değişmez, talep bekler; müşavir farkı görüp uygular ya da reddeder.
 */
export function ChangeRequestsPanel({
  engagement,
  open,
}: {
  engagement: string;
  /** Dönem şeridindeki açık talep toplamı; 0 iken panel hiç sorgu atmaz. */
  open: number;
}) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canRead = can(Permission.TenantPeriodsRead);
  const canDecide = can(Permission.TenantChangeRequestsDecide);
  // Komuta Merkezi / Mükellefler'den "?talepler=ac" ile gelinince liste açık
  // gelir (#710). Panel sayım gelmeden çizilmediği için sunucu çiziminde
  // fark oluşmaz.
  const [expanded, setExpanded] = useState(
    () =>
      typeof window !== "undefined" &&
      new URLSearchParams(window.location.search).get("talepler") === "ac",
  );
  const cr = useChangeRequests(engagement, canRead && open > 0 && expanded);

  if (!canRead || open === 0) return null;
  const items = cr.list.data?.items ?? [];

  return (
    <section
      aria-labelledby="change-requests-title"
      className="border-warn bg-warn-tint rounded-xl border p-4"
    >
      <div className="flex flex-wrap items-center gap-3">
        <FileClock aria-hidden className="text-warn size-5 shrink-0" />
        <div className="min-w-0 flex-1">
          <h2
            id="change-requests-title"
            className="font-display text-base font-semibold"
          >
            {t("office.change_requests.title", { count: open })}
          </h2>
          <p className="text-text-2 text-sm">
            {t("office.change_requests.hint")}
          </p>
        </div>
        <Button
          variant="outline"
          aria-expanded={expanded}
          aria-controls="change-requests-list"
          onClick={() => setExpanded((v) => !v)}
        >
          {expanded ? (
            <ChevronUp aria-hidden className="size-4" />
          ) : (
            <ChevronDown aria-hidden className="size-4" />
          )}
          {expanded
            ? t("office.change_requests.hide")
            : t("office.change_requests.show")}
        </Button>
      </div>
      {expanded ? (
        <div id="change-requests-list" className="mt-4 space-y-3">
          {cr.list.isLoading ? (
            <p className="text-text-3 text-sm">{t("common.loading")}</p>
          ) : cr.list.isError ? (
            <p role="alert" className="text-danger text-sm">
              {cr.list.error.message}
            </p>
          ) : items.length === 0 ? (
            <p className="text-text-3 text-sm">
              {t("office.change_requests.none")}
            </p>
          ) : (
            items.map((c) => (
              <RequestCard
                key={c.uuid}
                req={c}
                engagement={engagement}
                canDecide={canDecide}
                pending={
                  cr.decide.isPending && cr.decide.variables?.uuid === c.uuid
                }
                onDecide={(action) =>
                  cr.decide.mutate(
                    { uuid: c.uuid, action },
                    {
                      onSuccess: () =>
                        toast.success(
                          t(`office.change_requests.${action}_done`),
                        ),
                      onError: (err) =>
                        toast.error(
                          isApiError(err) && err.status === 409
                            ? t("office.change_requests.stale")
                            : isApiError(err)
                              ? err.message
                              : t("office.change_requests.failed"),
                        ),
                    },
                  )
                }
              />
            ))
          )}
        </div>
      ) : null}
    </section>
  );
}

function RequestCard({
  req,
  engagement,
  canDecide,
  pending,
  onDecide,
}: {
  req: ChangeRequest;
  engagement: string;
  canDecide: boolean;
  pending: boolean;
  onDecide: (action: "apply" | "reject") => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const d = readDiff(req.diff);
  return (
    <article className="bg-surface border-line space-y-3 rounded-lg border p-3">
      <div className="flex flex-wrap items-center gap-2 text-sm">
        <StatusPill tone={req.kind === "create" ? "info" : "warn"}>
          {t(`office.change_requests.kind_${req.kind}`)}
        </StatusPill>
        <span className="font-mono tabular-nums">{req.period}</span>
        {d.source ? (
          <span className="text-text-3">
            {t("office.change_requests.from", { source: d.source })}
          </span>
        ) : null}
        <span className="text-text-3 ml-auto text-xs">
          {formatCellDate(req.created_at, locale, "datetime")}
        </span>
      </div>
      {d.fields.length > 0 ? (
        <Table className="w-full text-sm">
          <TableCaption className="sr-only">
            {t("office.change_requests.fields")}
          </TableCaption>
          <TableHeader className="text-xs">
            <TableRow className="border-line hover:bg-transparent">
              <TableHead
                scope="col"
                className="text-text-3 h-auto px-0 py-1 pr-3 font-medium"
              >
                {t("office.change_requests.col_field")}
              </TableHead>
              <TableHead
                scope="col"
                className="text-text-3 h-auto px-0 py-1 pr-3 font-medium"
              >
                {t("office.change_requests.col_old")}
              </TableHead>
              <TableHead
                scope="col"
                className="text-text-3 h-auto px-0 py-1 font-medium"
              >
                {t("office.change_requests.col_new")}
              </TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {d.fields.map((f) => (
              <TableRow
                key={f.field}
                className="border-line hover:bg-transparent"
              >
                <TableHead
                  scope="row"
                  className="text-foreground h-auto px-0 py-1.5 pr-3 font-mono text-xs font-normal"
                >
                  {f.field}
                </TableHead>
                <TableCell className="text-text-3 px-0 py-1.5 pr-3 break-all whitespace-normal line-through">
                  {f.old}
                </TableCell>
                <TableCell className="px-0 py-1.5 font-medium break-all">
                  {f.new}
                </TableCell>
              </TableRow>
            ))}
          </TableBody>
        </Table>
      ) : (
        <p className="text-text-3 text-sm">
          {req.kind === "create"
            ? t("office.change_requests.new_invoice")
            : t("office.change_requests.no_fields")}
        </p>
      )}
      <div className="flex flex-wrap items-center gap-2">
        {req.invoice_uuid ? (
          <Button asChild variant="ghost" size="sm">
            <Link
              href={routes.tenant.engagementInvoice(
                slug,
                engagement,
                req.invoice_uuid,
              )}
            >
              {t("office.change_requests.open_invoice")}
            </Link>
          </Button>
        ) : null}
        {canDecide ? (
          <span className="ml-auto flex gap-2">
            <Button
              variant="outline"
              size="sm"
              disabled={pending}
              onClick={() => onDecide("reject")}
            >
              {t("office.change_requests.reject")}
            </Button>
            <Button
              size="sm"
              disabled={pending}
              onClick={() => onDecide("apply")}
            >
              {t("office.change_requests.apply")}
            </Button>
          </span>
        ) : null}
      </div>
    </article>
  );
}
