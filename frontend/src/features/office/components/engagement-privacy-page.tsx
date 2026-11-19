"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Download, ShieldCheck } from "lucide-react";
import { useId, useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { BosDurum } from "@/components/hakan";
import { StatusPill, type StatusTone } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import { RadioGroup, RadioGroupItem } from "@/components/ui/radio-group";
import { Skeleton } from "@/components/ui/skeleton";
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { datetime } from "@/lib/utils/format";
import { useDialogs } from "@/providers/dialog-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  privacyKeys,
  privacyService,
  type PrivacyRequest,
} from "../services/privacy.service";

const statusTone: Record<PrivacyRequest["status"], StatusTone> = {
  pending: "warn",
  completed: "ok",
  rejected: "neutral",
  failed: "danger",
};

/**
 * `/t/{slug}/engagements/{uuid}/privacy` (#518 B3): mükellefin KVKK ilgili
 * kişi talepleri. Talep açılır (verinin dışa aktarımı ya da silme /
 * anonimleştirme), ofis sahibi ya da yöneticisi onaylar ya da gerekçeyle
 * reddeder; onaylanan dışa aktarım süresi dolana kadar indirilir.
 */
export function EngagementPrivacyPage({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const canRequest = can(Permission.TenantPrivacyRequest);
  const canApprove = can(Permission.TenantPrivacyApprove);
  const list = useQuery({
    queryKey: privacyKeys.list(slug, engagement),
    queryFn: () => privacyService.list(slug, engagement),
    enabled: canRequest,
  });

  if (!canRequest)
    return (
      <BosDurum
        baslik={t("office.privacy.forbidden_title")}
        aciklama={t("office.privacy.forbidden")}
      />
    );
  const items = list.data?.items ?? [];
  return (
    <div className="space-y-5">
      <div>
        <h2 className="font-display text-xl font-bold">
          {t("office.privacy.title")}
        </h2>
        <p className="text-text-2 text-sm">{t("office.privacy.description")}</p>
      </div>

      <NewRequestForm engagement={engagement} />

      <section aria-labelledby="privacy-list-title" className="space-y-3">
        <h3 id="privacy-list-title" className="text-sm font-semibold">
          {t("office.privacy.list_title")}
        </h3>
        {list.isLoading ? (
          <Skeleton className="h-24 w-full" />
        ) : list.isError ? (
          <ErrorState
            title={t("office.privacy.load_failed")}
            description={list.error.message}
            onRetry={() => void list.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : items.length === 0 ? (
          <BosDurum
            baslik={t("office.privacy.empty_title")}
            aciklama={t("office.privacy.empty")}
          />
        ) : (
          <ul className="space-y-3" data-testid="privacy-requests">
            {items.map((r) => (
              <RequestCard
                key={r.uuid}
                req={r}
                engagement={engagement}
                canApprove={canApprove}
              />
            ))}
          </ul>
        )}
      </section>
    </div>
  );
}

function NewRequestForm({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const qc = useQueryClient();
  const id = useId();
  const [kind, setKind] = useState<PrivacyRequest["kind"]>("export");
  const [reason, setReason] = useState("");
  const create = useMutation({
    mutationFn: () =>
      privacyService.create(slug, engagement, {
        kind,
        subject_type: "taxpayer",
        reason: reason.trim(),
      }),
    onSuccess: () => {
      setReason("");
      void qc.invalidateQueries({
        queryKey: privacyKeys.list(slug, engagement),
      });
      toast.success(t("office.privacy.created"));
    },
    onError: (e) =>
      toast.error(
        isApiError(e) ? e.message : t("office.privacy.create_failed"),
      ),
  });
  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (!create.isPending) create.mutate();
  };
  return (
    <form
      onSubmit={submit}
      className="bg-surface border-line space-y-3 rounded-xl border p-4"
      aria-labelledby={`${id}-title`}
    >
      <h3 id={`${id}-title`} className="text-sm font-semibold">
        {t("office.privacy.new_title")}
      </h3>
      <RadioGroup
        value={kind}
        onValueChange={(v) => setKind(v as PrivacyRequest["kind"])}
        className="grid gap-2 sm:grid-cols-2"
        aria-label={t("office.privacy.kind")}
      >
        {(["export", "erasure"] as const).map((k) => (
          <Label
            key={k}
            htmlFor={`${id}-${k}`}
            className="border-line flex cursor-pointer items-start gap-3 rounded-lg border p-3 font-normal"
          >
            <RadioGroupItem id={`${id}-${k}`} value={k} className="mt-0.5" />
            <span>
              <span className="block text-sm font-semibold">
                {t(`office.privacy.kind_${k}`)}
              </span>
              <span className="text-text-2 block text-xs">
                {t(`office.privacy.kind_${k}_hint`)}
              </span>
            </span>
          </Label>
        ))}
      </RadioGroup>
      <div className="space-y-1">
        <Label htmlFor={`${id}-reason`}>{t("office.privacy.reason")}</Label>
        <Textarea
          id={`${id}-reason`}
          rows={2}
          maxLength={2000}
          value={reason}
          onChange={(e) => setReason(e.target.value)}
          placeholder={t("office.privacy.reason_placeholder")}
        />
      </div>
      <div className="flex flex-wrap items-center gap-3">
        <Button type="submit" disabled={create.isPending}>
          {t("office.privacy.create")}
        </Button>
        <span className="text-text-3 text-xs">
          {t("office.privacy.create_hint")}
        </span>
      </div>
    </form>
  );
}

function RequestCard({
  req,
  engagement,
  canApprove,
}: {
  req: PrivacyRequest;
  engagement: string;
  canApprove: boolean;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { confirm, prompt } = useDialogs();
  const qc = useQueryClient();
  const decide = useMutation({
    mutationFn: (v: { action: "approve" | "reject"; note?: string }) =>
      privacyService.decide(slug, engagement, req.uuid, v.action, v.note),
    onSuccess: (_, v) => {
      void qc.invalidateQueries({
        queryKey: privacyKeys.list(slug, engagement),
      });
      toast.success(t(`office.privacy.${v.action}_done`));
    },
    onError: (e) =>
      toast.error(
        isApiError(e) && e.status === 409
          ? t("office.privacy.already_decided")
          : isApiError(e)
            ? e.message
            : t("office.privacy.decide_failed"),
      ),
  });
  const download = useMutation({
    mutationFn: () => privacyService.download(slug, engagement, req.uuid),
    onError: () => toast.error(t("office.privacy.download_failed")),
  });
  const approve = async () => {
    const erasure = req.kind === "erasure";
    const ok = await confirm({
      title: t(`office.privacy.approve_${req.kind}_title`),
      description: t(`office.privacy.approve_${req.kind}_body`),
      confirmLabel: t("office.privacy.approve"),
      variant: erasure ? "destructive" : "default",
    });
    if (ok) decide.mutate({ action: "approve" });
  };
  const reject = async () => {
    const note = await prompt({
      title: t("office.privacy.reject_title"),
      description: t("office.privacy.reject_body"),
      confirmLabel: t("office.privacy.reject"),
    });
    if (note?.trim()) decide.mutate({ action: "reject", note: note.trim() });
  };
  const expired = req.expires_at
    ? new Date(req.expires_at) < new Date()
    : false;
  const downloadable =
    req.kind === "export" && req.status === "completed" && !expired;
  return (
    <li className="bg-surface border-line space-y-2 rounded-xl border p-4">
      <div className="flex flex-wrap items-center gap-2">
        <ShieldCheck aria-hidden className="text-text-3 size-4" />
        <span className="text-sm font-semibold">
          {t(`office.privacy.kind_${req.kind}`)}
        </span>
        <StatusPill tone={statusTone[req.status]}>
          {t(`office.privacy.status_${req.status}`)}
        </StatusPill>
        <time dateTime={req.created_at} className="text-text-3 ms-auto text-xs">
          {datetime(req.created_at, undefined, locale)}
        </time>
      </div>
      {req.reason ? <p className="text-sm">{req.reason}</p> : null}
      {req.decision_note ? (
        <p className="text-text-2 text-sm italic">“{req.decision_note}”</p>
      ) : null}
      {req.kind === "export" && req.status === "completed" && req.expires_at ? (
        <p className="text-text-3 text-xs">
          {expired
            ? t("office.privacy.export_expired")
            : t("office.privacy.export_until", {
                date: datetime(req.expires_at, undefined, locale),
              })}
        </p>
      ) : null}
      {canApprove && (req.status === "pending" || downloadable) ? (
        <div className="flex flex-wrap gap-2">
          {req.status === "pending" ? (
            <>
              <Button
                size="sm"
                className="max-md:min-h-11"
                disabled={decide.isPending}
                onClick={() => void approve()}
              >
                {t("office.privacy.approve")}
              </Button>
              <Button
                size="sm"
                variant="outline"
                className="max-md:min-h-11"
                disabled={decide.isPending}
                onClick={() => void reject()}
              >
                {t("office.privacy.reject")}
              </Button>
            </>
          ) : null}
          {downloadable ? (
            <Button
              size="sm"
              variant="outline"
              className="max-md:min-h-11"
              disabled={download.isPending}
              onClick={() => download.mutate()}
            >
              <Download aria-hidden className="size-4" />
              {t("office.privacy.download")}
            </Button>
          ) : null}
        </div>
      ) : null}
    </li>
  );
}
