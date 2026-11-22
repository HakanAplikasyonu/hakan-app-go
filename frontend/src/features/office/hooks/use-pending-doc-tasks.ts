"use client";

import { useQuery } from "@tanstack/react-query";

import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";

/** Bekleyen istek: açık ya da gönderilmiş ama henüz eşleşmemiş. */
const PENDING_TASK = new Set(["open", "submitted"]);

export type PendingDocTask = {
  uuid: string;
  kind: string;
  title: string;
  status: string;
  overdue: boolean;
  due_at: string;
};

/**
 * Bir mükellefin seçili dönemdeki bekleyen belge istekleri: açık olanlar ve
 * mükellefin gönderip kontrol bekleyenleri. "Sıradaki iş" de aynı ikisini
 * sayar (#563). Mükellef özeti ve belge toplama aynı önbelleği paylaşır.
 */
export function usePendingDocTasks(engagement: string, period: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: ["tenant", slug.toLowerCase(), "doc-tasks", engagement, period],
    queryFn: () =>
      tenantRequest<{ items: PendingDocTask[]; total: number }>(
        slug,
        "GET",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/document-tasks`,
        { query: { status: "all", period, limit: 100 } },
      ),
    select: (d) => d.items.filter((x) => PENDING_TASK.has(x.status)),
    enabled: !!engagement && !!period,
  });
}
