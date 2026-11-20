"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import Link from "next/link";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { usePeriodText } from "../period";
import { createDocumentRequest } from "../services/engagement-documents.service";

/** Ofisin bir dönem için açtığı belge isteği (yalnız burada gereken alanlar). */
type RequestTask = {
  uuid: string;
  kind: string;
  title: string;
  status: string;
  overdue: boolean;
  due_at: string;
  created_at: string;
  submitted_at: string | null;
};

/**
 * Dönemin belge istekleri; mükellef özeti de aynı anahtarla okur (#563), bu
 * yüzden yanıt biçimi aynıdır.
 */
function useRequestTasks(engagement: string, period: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: ["tenant", slug.toLowerCase(), "doc-tasks", engagement, period],
    queryFn: () =>
      tenantRequest<{ items: RequestTask[]; total: number }>(
        slug,
        "GET",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/document-tasks`,
        { query: { status: "all", period, limit: 100 } },
      ),
  });
}

/** Aynı ekstre için en son açılan, iptal edilmemiş istek. */
export function latestRequest<
  T extends Pick<RequestTask, "kind" | "title" | "status" | "created_at">,
>(items: T[], title: string): T | undefined {
  return items
    .filter(
      (x) =>
        x.kind === "document_request" &&
        x.title === title &&
        x.status !== "cancelled",
    )
    .sort((a, b) => b.created_at.localeCompare(a.created_at))[0];
}

/**
 * Eksik ekstre (#568, #710): istenmediyse tek eylem "Mükelleften iste";
 * istendiyse ne zaman istendiği, durumu (bekleniyor / son gün geçti /
 * mükellef gönderdi) ve isteğe git bağlantısı görünür. Sayfa yenilense de
 * durum kaybolmaz; aynı ekstre ikinci kez istenmez. Son gün geçtiyse
 * "Yeniden iste" hatırlatmayı yeniler.
 */
export function StatementRequest({
  engagement,
  period,
  account,
}: {
  engagement: string;
  period: string;
  account: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const periodText = usePeriodText();
  const queryClient = useQueryClient();
  const tasks = useRequestTasks(engagement, period);
  const title = t("office.tax_returns.office.statement_request_title", {
    period: periodText(period),
    account,
  });
  const refresh = () =>
    queryClient.invalidateQueries({
      queryKey: ["tenant", slug.toLowerCase(), "doc-tasks", engagement],
    });
  const request = useMutation({
    mutationFn: () =>
      createDocumentRequest(slug, engagement, {
        title,
        note: t("office.tax_returns.office.statement_request_note"),
        due_days: 3,
        // Ekstrenin dönemi: istek o dönemin belge isteklerinde görünür
        // (yoksa son günün ayına, sonraki aya düşerdi).
        period,
      }),
    onSuccess: () => {
      toast.success(t("office.tax_returns.office.statement_requested"));
      return refresh();
    },
    onError: () => toast.error(t("office.api.load_error")),
  });
  const again = useMutation({
    mutationFn: (task: string) =>
      tenantRequest<unknown>(
        slug,
        "POST",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/document-tasks/${encodeURIComponent(task)}/rerequest`,
        { body: { due_days: 3 }, silent: true },
      ),
    onSuccess: () => {
      toast.success(t("office.tax_returns.office.statement_rerequested"));
      return refresh();
    },
    onError: () => toast.error(t("office.api.load_error")),
  });

  if (tasks.isLoading) return null;
  const task = latestRequest(tasks.data?.items ?? [], title);
  const day = (iso: string) => new Date(iso).toLocaleDateString("tr-TR");
  const stop = (e: React.MouseEvent) => e.stopPropagation();

  if (!task) {
    if (!can(permissions.tenant.docTasks.write)) return null;
    return (
      <Button
        size="sm"
        variant="link"
        className="h-auto p-0 text-xs"
        data-no-row-click
        disabled={request.isPending}
        onClick={(e) => {
          stop(e);
          request.mutate();
        }}
      >
        {t("office.tax_returns.office.statement_request")}
      </Button>
    );
  }

  const state =
    task.status === "submitted"
      ? t("office.tax_returns.office.statement_state_submitted", {
          date: day(task.submitted_at ?? task.created_at),
        })
      : task.status === "matched"
        ? t("office.tax_returns.office.statement_state_matched")
        : task.overdue
          ? t("office.tax_returns.office.statement_state_overdue", {
              due: day(task.due_at),
            })
          : t("office.tax_returns.office.statement_state_waiting", {
              due: day(task.due_at),
            });
  return (
    <span
      className="inline-flex flex-wrap items-center gap-x-2"
      data-testid="statement-request-state"
    >
      <span
        className={cn(
          task.overdue && task.status === "open"
            ? "text-status-blocking"
            : "text-text-3",
        )}
      >
        {t("office.tax_returns.office.statement_requested_on", {
          date: day(task.created_at),
        })}
        {" · "}
        {state}
      </span>
      <Link
        href={routes.tenant.engagement(slug, engagement)}
        className="text-primary underline-offset-2 hover:underline"
        data-no-row-click
        onClick={stop}
      >
        {t("office.tax_returns.office.statement_open")}
      </Link>
      {task.status === "open" &&
      task.overdue &&
      can(permissions.tenant.docTasks.write) ? (
        <Button
          size="sm"
          variant="link"
          className="h-auto p-0 text-xs"
          data-no-row-click
          disabled={again.isPending}
          onClick={(e) => {
            stop(e);
            again.mutate(task.uuid);
          }}
        >
          {t("office.tax_returns.office.statement_rerequest")}
        </Button>
      ) : null}
    </span>
  );
}
