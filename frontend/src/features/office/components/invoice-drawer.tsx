"use client";

import { useMutation } from "@tanstack/react-query";
import { TriangleAlert } from "lucide-react";
import Link from "next/link";
import { useMemo } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Sheet,
  SheetContent,
  SheetDescription,
  SheetTitle,
} from "@/components/ui/sheet";
import { Skeleton } from "@/components/ui/skeleton";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  useEngagementInvoice,
  useInvoiceUBL,
} from "../hooks/use-office-invoices";
import { formatAmount } from "../lib/decimal";
import { toPanelInvoice } from "../lib/from-api";
import { createDocumentRequest } from "../services/engagement-documents.service";
import { AmountWaterfall } from "./amount-waterfall";
import { InvoiceStateBadge } from "./invoices-page";
import { Figure, Money, useWarningText } from "./office-ui";

const DOC_TASKS_WRITE = "tenant.doc_tasks.write";
const SEVERITY_RANK = { blocker: 0, warning: 1, info: 2 } as const;

/** "7c1e…a90f": uzun ETTN'nin baş ve sonu. */
function shortEttn(ettn?: string) {
  if (!ettn) return "";
  return ettn.length > 12 ? `${ettn.slice(0, 4)}…${ettn.slice(-4)}` : ettn;
}

/**
 * Faturalar listesinin sağ çekmecesi (arayüz 6/9, #329; tuval OfisBelgeler):
 * tür, karşı taraf, ETTN ve numara, en ağır açık uyarı, tutar şelalesi ve
 * kalemler. "Faturayı görüntüle" tam ayrıntıya, "Mükelleften düzeltme iste"
 * mükellefe belge isteği açar.
 */
export function InvoiceDrawer({
  engagement,
  invoice: id,
  taxpayerTitle,
  onClose,
}: {
  engagement: string;
  invoice: string | null;
  taxpayerTitle?: string;
  onClose: () => void;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const detail = useEngagementInvoice(slug, engagement, id ?? "");
  const ubl = useInvoiceUBL(
    slug,
    engagement,
    id ?? "",
    !!id && !!detail.data?.has_ubl,
  );
  const warningText = useWarningText();
  const d = id ? detail.data : undefined;
  const invoice = useMemo(
    () => (d ? toPanelInvoice(d, engagement, ubl.data) : null),
    [d, engagement, ubl.data],
  );
  const worst = useMemo(
    () =>
      [...(d?.warnings ?? [])]
        .filter((w) => w.status === "open")
        .sort((a, b) => SEVERITY_RANK[a.severity] - SEVERITY_RANK[b.severity])
        .at(0),
    [d],
  );
  const worstText = worst
    ? warningText({
        code: worst.code,
        params: Object.fromEntries(
          Object.entries(worst.params ?? {}).map(([k, v]) => [k, String(v)]),
        ),
      })
    : "";
  const request = useMutation({
    mutationFn: () =>
      createDocumentRequest(slug, engagement, {
        title: t("office.drawer.fix_request_title", {
          number: d?.invoice_number ?? "",
        }),
        note: worst ? `${worstText} (${worst.code})` : undefined,
        due_days: 7,
      }),
    onSuccess: () => toast.success(t("office.drawer.fix_request_sent")),
    onError: (err) =>
      toast.error(err.message || t("office.drawer.fix_request_error")),
  });
  const dateText = d
    ? new Date(`${d.issue_date}T00:00:00`).toLocaleDateString(
        locale === "tr" ? "tr-TR" : "en-GB",
        { day: "numeric", month: "short", year: "numeric" },
      )
    : "";

  return (
    <Sheet open={!!id} onOpenChange={(open) => !open && onClose()}>
      <SheetContent
        side="right"
        className="flex w-full flex-col gap-0 overflow-y-auto p-0 sm:max-w-xl"
      >
        {!d || !invoice ? (
          <div className="space-y-3 p-6">
            <SheetTitle className="sr-only">
              {t("office.drawer.loading")}
            </SheetTitle>
            {detail.isError ? (
              <p className="text-danger text-sm">
                {t("office.api.load_error")}
              </p>
            ) : (
              <>
                <Skeleton className="h-6 w-40" />
                <Skeleton className="h-8 w-72" />
                <Skeleton className="h-48 w-full" />
              </>
            )}
          </div>
        ) : (
          <>
            <div className="border-line space-y-2 border-b p-5 pe-12">
              <div className="flex flex-wrap items-center gap-2">
                <span className="bg-brand-tint text-brand-text rounded-full px-2.5 py-0.5 text-xs font-semibold">
                  {d.profile ? `${d.type} · ${d.profile}` : d.type}
                </span>
                <InvoiceStateBadge status={d.status} />
              </div>
              <SheetTitle className="font-display text-xl font-bold">
                {d.counterparty_name || d.invoice_number}
              </SheetTitle>
              <SheetDescription asChild>
                <div className="text-text-2 space-y-0.5 text-sm">
                  <div>
                    <Figure className="text-xs">
                      {d.ettn ? `ETTN ${shortEttn(d.ettn)} · ` : ""}
                      {d.invoice_number}
                    </Figure>
                  </div>
                  <div>
                    {[
                      t(`office.direction_${invoice.direction}`),
                      taxpayerTitle,
                      dateText,
                    ]
                      .filter(Boolean)
                      .join(" · ")}
                  </div>
                </div>
              </SheetDescription>
            </div>

            <div className="flex-1 space-y-5 p-5">
              {worst ? (
                <div
                  role="alert"
                  className={
                    worst.severity === "blocker"
                      ? "border-danger bg-danger-tint text-danger flex gap-2.5 rounded-lg border p-3 text-sm"
                      : "border-warn bg-warn-tint text-warn flex gap-2.5 rounded-lg border p-3 text-sm"
                  }
                >
                  <TriangleAlert
                    className="mt-0.5 size-4 shrink-0"
                    aria-hidden
                  />
                  <p>
                    <span className="font-semibold">
                      {t(`office.severity_${worst.severity}`)}:
                    </span>{" "}
                    <span className="text-foreground">{worstText}</span>{" "}
                    <Figure className="text-xs">({worst.code})</Figure>
                  </p>
                </div>
              ) : null}

              <section aria-labelledby="drawer-waterfall">
                <h3
                  id="drawer-waterfall"
                  className="text-text-3 mb-2 text-xs font-semibold tracking-wide uppercase"
                >
                  {t("office.waterfall.title")}
                </h3>
                <div className="border-line overflow-hidden rounded-lg border">
                  <AmountWaterfall invoice={invoice} />
                </div>
              </section>

              <section aria-labelledby="drawer-lines">
                <h3
                  id="drawer-lines"
                  className="text-text-3 mb-2 text-xs font-semibold tracking-wide uppercase"
                >
                  {t("office.lines")}
                </h3>
                <div className="border-line overflow-x-auto rounded-lg border">
                  <table className="w-full text-sm">
                    <thead className="bg-surface-2 text-text-2 text-xs">
                      <tr>
                        <th className="px-3 py-2 text-start font-medium">
                          {t("office.col_item")}
                        </th>
                        <th className="px-3 py-2 text-end font-medium">
                          {t("office.col_quantity")}
                        </th>
                        <th className="px-3 py-2 text-end font-medium">
                          {t("office.col_line_total")}
                        </th>
                      </tr>
                    </thead>
                    <tbody className="divide-line divide-y">
                      {invoice.lines.map((l) => (
                        <tr key={l.id}>
                          <td className="px-3 py-2">{l.name}</td>
                          <td className="px-3 py-2 text-end whitespace-nowrap">
                            <Figure>
                              {formatAmount(l.quantity)} {l.unit}
                            </Figure>
                          </td>
                          <td className="px-3 py-2 text-end">
                            <Money value={l.lineTotal} currency={d.currency} />
                          </td>
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
              </section>
            </div>

            <div className="border-line bg-surface sticky bottom-0 flex flex-wrap gap-2 border-t p-4">
              <Button asChild className="min-h-11 md:min-h-9">
                <Link href={routes.tenant.invoice(slug, d.uuid, engagement)}>
                  {t("office.drawer.open_invoice")}
                </Link>
              </Button>
              {can(DOC_TASKS_WRITE) ? (
                <Button
                  type="button"
                  variant="outline"
                  className="min-h-11 md:min-h-9"
                  disabled={request.isPending || request.isSuccess}
                  onClick={() => request.mutate()}
                >
                  {request.isSuccess
                    ? t("office.drawer.fix_request_sent")
                    : t("office.drawer.fix_request")}
                </Button>
              ) : null}
            </div>
          </>
        )}
      </SheetContent>
    </Sheet>
  );
}
