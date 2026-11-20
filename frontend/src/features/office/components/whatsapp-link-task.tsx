"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Link2 } from "lucide-react";
import { useState } from "react";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { Skeleton } from "@/components/ui/skeleton";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { tenantRequest } from "@/lib/api/tenant-request";
import { useLocale } from "@/providers/locale-provider";
import { appToast } from "@/providers/toast-provider";

import type { WhatsAppCapture } from "../services/whatsapp.service";

type OpenTask = { uuid: string; kind: string; title: string; due_at: string };

/**
 * Görevsiz kaydedilmiş belgeyi mükellefin açık belge isteklerinden birine
 * bağlar (#571). Mükellefin tek açık isteği varsa belge kaydedilirken
 * kendiliğinden bağlanır; birden fazlaysa sistem tahmin etmez, ofis buradan
 * seçer.
 */
export function WhatsAppLinkTask({ capture }: { capture: WhatsAppCapture }) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const [open, setOpen] = useState(false);
  const engagement = capture.taxpayer.engagement_uuid;
  const base = `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;
  const tasks = useQuery({
    queryKey: [
      "tenant",
      slug.toLowerCase(),
      "doc-tasks",
      engagement,
      "open-requests",
    ],
    queryFn: () =>
      tenantRequest<{ items: OpenTask[] }>(
        slug,
        "GET",
        `${base}/document-tasks`,
        { query: { status: "open", limit: 100 } },
      ),
    select: (d) => d.items.filter((x) => x.kind === "document_request"),
    enabled: open,
  });
  const link = useMutation({
    mutationFn: (task: string) =>
      tenantRequest<WhatsAppCapture>(
        slug,
        "PUT",
        `${base}/whatsapp/captures/${encodeURIComponent(capture.uuid)}/task`,
        { body: { task_uuid: task } },
      ),
    onSuccess: (c) => {
      appToast.success(
        t("office.whatsapp.link_done", { title: c.task_title ?? "" }),
      );
      setOpen(false);
      void qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase(), "whatsapp"],
      });
      void qc.invalidateQueries({
        queryKey: ["tenant", slug.toLowerCase(), "doc-tasks", engagement],
      });
    },
    onError: (e) =>
      appToast.error(
        isApiError(e) ? e.message : t("office.whatsapp.link_failed"),
      ),
  });
  const items = tasks.data ?? [];
  return (
    <>
      <Button size="sm" variant="outline" onClick={() => setOpen(true)}>
        <Link2 className="size-4" aria-hidden />
        {t("office.whatsapp.link_task")}
      </Button>
      <Dialog open={open} onOpenChange={setOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>{t("office.whatsapp.link_title")}</DialogTitle>
            <DialogDescription>
              {t("office.whatsapp.link_description", {
                file: capture.file_name,
                taxpayer: capture.taxpayer.title,
              })}
            </DialogDescription>
          </DialogHeader>
          {tasks.isLoading ? (
            <Skeleton className="h-16 w-full" />
          ) : items.length === 0 ? (
            <p className="text-muted-foreground text-sm">
              {t("office.whatsapp.link_empty")}
            </p>
          ) : (
            <ul className="divide-y rounded-md border text-sm">
              {items.map((x) => (
                <li
                  key={x.uuid}
                  className="flex items-center justify-between gap-3 px-3 py-2"
                >
                  <span className="min-w-0">
                    <span className="block truncate font-medium">
                      {x.title}
                    </span>
                    <span className="text-muted-foreground text-xs">
                      {t("office.whatsapp.link_due", {
                        date: new Date(x.due_at).toLocaleDateString(
                          locale === "tr" ? "tr-TR" : "en-GB",
                        ),
                      })}
                    </span>
                  </span>
                  <Button
                    size="sm"
                    disabled={link.isPending}
                    onClick={() => link.mutate(x.uuid)}
                    aria-label={t("office.whatsapp.link_pick_aria", {
                      title: x.title,
                    })}
                  >
                    {t("office.whatsapp.link_pick")}
                  </Button>
                </li>
              ))}
            </ul>
          )}
        </DialogContent>
      </Dialog>
    </>
  );
}
