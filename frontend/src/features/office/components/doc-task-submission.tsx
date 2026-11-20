"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Paperclip, RotateCcw } from "lucide-react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { tenantRequest } from "@/lib/api/tenant-request";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import { downloadFile } from "../services/download";

/** Yeniden istenen belgenin son günü: bugünden bu kadar gün sonra. */
const REREQUEST_DUE_DAYS = 3;

type TaskUpload = { uuid: string; file_name: string | null; kind: string };
type TaskDetail = { uuid: string; status: string; uploads?: TaskUpload[] };

/**
 * Mükellefin belge isteğine gönderdiği dosyalar (#571): "Geldi, kontrol et"
 * satırında dosya açılır (indirilir) ve belge doğruysa istek buradan
 * tamamlanır. Önce yalnızca etiket vardı; ofis gelen belgeyi göremiyordu.
 */
export function DocTaskSubmission({
  engagement,
  task,
}: {
  engagement: string;
  task: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const { prompt } = useDialogs();
  const base = `/v1/tenant/engagements/${encodeURIComponent(engagement)}/document-tasks/${encodeURIComponent(task)}`;
  const q = useQuery({
    queryKey: [
      "tenant",
      slug.toLowerCase(),
      "doc-tasks",
      engagement,
      "detail",
      task,
    ],
    queryFn: () => tenantRequest<TaskDetail>(slug, "GET", base),
  });
  const complete = useMutation({
    mutationFn: () =>
      tenantRequest<TaskDetail>(slug, "POST", `${base}/complete`),
    onSuccess: () => {
      toast.success(t("office.engagement.task_completed"));
      void qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase(), "doc-tasks", engagement],
      });
    },
    onError: (e) =>
      toast.error(
        isApiError(e) ? e.message : t("office.engagement.task_complete_failed"),
      ),
  });
  // Gelen belge yanlış ya da eksikse (#571): neyin eksik olduğu yazılır,
  // istek yeniden açılır ve mükellefe yeniden bildirilir.
  const rerequest = useMutation({
    mutationFn: (note: string) =>
      tenantRequest<TaskDetail>(slug, "POST", `${base}/rerequest`, {
        body: { note, due_days: REREQUEST_DUE_DAYS },
      }),
    onSuccess: () => {
      toast.success(t("office.engagement.task_rerequested"));
      void qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase(), "doc-tasks", engagement],
      });
    },
    onError: (e) =>
      toast.error(
        isApiError(e)
          ? e.message
          : t("office.engagement.task_rerequest_failed"),
      ),
  });
  const open = useMutation({
    mutationFn: (u: TaskUpload) =>
      downloadFile(
        slug,
        `${base}/uploads/${encodeURIComponent(u.uuid)}/file`,
        {},
        u.file_name ?? "belge",
      ),
    onError: () => toast.error(t("office.engagement.task_file_failed")),
  });

  if (q.isLoading) return <Skeleton className="h-6 w-40" />;
  const uploads = q.data?.uploads ?? [];
  return (
    <div
      className="flex flex-wrap items-center gap-1.5"
      data-testid="doc-task-submission"
    >
      {uploads.map((u, i) => (
        <Button
          key={u.uuid}
          size="sm"
          variant="outline"
          className="h-7 max-w-56 px-2 text-xs"
          disabled={open.isPending}
          onClick={() => open.mutate(u)}
        >
          <Paperclip className="size-3.5" aria-hidden />
          <span className="truncate">
            {u.file_name ??
              t("office.engagement.task_file", { index: String(i + 1) })}
          </span>
        </Button>
      ))}
      {can(permissions.tenant.docTasks.write) ? (
        <>
          <Button
            size="sm"
            className="h-7 px-2 text-xs"
            disabled={complete.isPending || rerequest.isPending}
            onClick={() => complete.mutate()}
          >
            <Check className="size-3.5" aria-hidden />
            {t("office.engagement.task_complete")}
          </Button>
          <Button
            size="sm"
            variant="ghost"
            className="h-7 px-2 text-xs"
            disabled={complete.isPending || rerequest.isPending}
            onClick={async () => {
              const note = await prompt({
                title: t("office.engagement.task_rerequest"),
                description: t("office.engagement.task_rerequest_prompt"),
                confirmLabel: t("office.engagement.task_rerequest"),
              });
              if (note?.trim()) rerequest.mutate(note.trim());
            }}
          >
            <RotateCcw className="size-3.5" aria-hidden />
            {t("office.engagement.task_rerequest")}
          </Button>
        </>
      ) : null}
    </div>
  );
}
