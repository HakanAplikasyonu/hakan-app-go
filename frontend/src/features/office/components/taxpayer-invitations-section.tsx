"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Check, Copy, MessageCircle, RotateCw, Send } from "lucide-react";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusPill, type StatusTone } from "@/components/tables/cells";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { Skeleton } from "@/components/ui/skeleton";
import { permissions } from "@/config/permissions";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  taxpayerInvitationKeys,
  taxpayerInvitationService,
  whatsappShareUrl,
  type TaxpayerInvitation,
  type TaxpayerInvitationLink,
  type TaxpayerInvitationRole,
} from "../services/taxpayer-invitations.service";

const STATUS_TONE: Record<TaxpayerInvitation["status"], StatusTone> = {
  pending: "info",
  accepted: "ok",
  revoked: "neutral",
  expired: "warn",
};

/**
 * Davet linkini kopyala ve WhatsApp ile paylaş düğmeleri. E-posta
 * ulaşmasa da müşavir linki mükellefe kendisi iletebilsin (Oğuz'un geri
 * bildirimi).
 */
export function InviteLinkActions({ url }: { url: string }) {
  const { t } = useLocale();
  const [copied, setCopied] = useState(false);

  const copy = async () => {
    try {
      await navigator.clipboard.writeText(url);
      setCopied(true);
      window.setTimeout(() => setCopied(false), 2000);
    } catch {
      toast.error(t("office.app_invite.copy_failed"));
    }
  };

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        type="button"
        size="sm"
        variant="outline"
        onClick={() => void copy()}
        data-testid="invite-copy"
      >
        {copied ? (
          <Check className="size-4" aria-hidden />
        ) : (
          <Copy className="size-4" aria-hidden />
        )}
        {copied ? t("office.app_invite.copied") : t("office.app_invite.copy")}
      </Button>
      <Button asChild size="sm" variant="outline">
        <a
          href={whatsappShareUrl(
            t("office.app_invite.whatsapp_message", { url }),
          )}
          target="_blank"
          rel="noopener noreferrer"
          data-testid="invite-whatsapp"
        >
          <MessageCircle className="size-4" aria-hidden />
          {t("office.app_invite.whatsapp")}
        </a>
      </Button>
    </div>
  );
}

/** Davetin sonucu: e-posta durumu ve link düğmeleri. */
function InviteResult({ result }: { result: TaxpayerInvitationLink }) {
  const { t } = useLocale();
  return (
    <div className="space-y-2" data-testid="invite-link-panel">
      {result.email_sent ? (
        <p
          className="text-ok text-sm font-semibold"
          data-testid="invite-email-sent"
        >
          {t("office.app_invite.email_sent", { email: result.email })}
        </p>
      ) : (
        <Alert variant="destructive" data-testid="invite-email-warning">
          <AlertTitle>{t("office.app_invite.email_failed")}</AlertTitle>
          <AlertDescription>
            {t(
              `office.app_invite.email_reason.${result.email_error ?? "enqueue_failed"}`,
            )}
          </AlertDescription>
        </Alert>
      )}
      <p className="text-text-2 text-xs">{t("office.app_invite.link_hint")}</p>
      <InviteLinkActions url={result.invite_url} />
    </div>
  );
}

/**
 * Mükellef sayfasında "Mükellef uygulaması daveti": mükellefin sahibini ya
 * da çalışanını uygulamaya davet eder. Davet e-postayla gider; yanıt davet
 * linkini döner, e-posta gitmezse ekran açıkça uyarır ve link kopyalanıp
 * WhatsApp ile paylaşılır. Bekleyen davette "Yeniden gönder" yeni link
 * üretir (eski link çalışmaz; sunucu linkin yalnız özetini tutar).
 */
export function TaxpayerInvitationsSection({
  engagement,
}: {
  engagement: string;
}) {
  const { t, locale } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const qc = useQueryClient();
  const canWrite = can(permissions.tenant.engagements.write);
  const [email, setEmail] = useState("");
  const [role, setRole] = useState<TaxpayerInvitationRole>("owner");
  const [last, setLast] = useState<TaxpayerInvitationLink | null>(null);
  const [links, setLinks] = useState<Record<string, TaxpayerInvitationLink>>(
    {},
  );

  const key = taxpayerInvitationKeys.list(slug, engagement);
  const list = useQuery({
    queryKey: key,
    queryFn: () => taxpayerInvitationService.list(slug, engagement),
  });

  const remember = (res: TaxpayerInvitationLink) => {
    setLast(res);
    setLinks((prev) => ({ ...prev, [res.uuid]: res }));
    void qc.invalidateQueries({ queryKey: key });
    if (!res.email_sent) toast.warning(t("office.app_invite.email_failed"));
  };

  const create = useMutation({
    mutationFn: () =>
      taxpayerInvitationService.create(slug, engagement, {
        email: email.trim(),
        role,
      }),
    onSuccess: (res) => {
      setEmail("");
      remember(res);
      if (res.email_sent) toast.success(t("office.app_invite.created"));
    },
    onError: (err) => toast.error(err.message || t("office.app_invite.error")),
  });
  const resend = useMutation({
    mutationFn: (uuid: string) =>
      taxpayerInvitationService.resend(slug, engagement, uuid),
    onSuccess: (res) => {
      remember(res);
      if (res.email_sent) toast.success(t("office.app_invite.resent"));
    },
    onError: (err) => toast.error(err.message || t("office.app_invite.error")),
  });
  const revoke = useMutation({
    mutationFn: (uuid: string) =>
      taxpayerInvitationService.revoke(slug, engagement, uuid),
    onSuccess: (_, uuid) => {
      setLinks((prev) => {
        const next = { ...prev };
        delete next[uuid];
        return next;
      });
      if (last?.uuid === uuid) setLast(null);
      void qc.invalidateQueries({ queryKey: key });
      toast.success(t("office.app_invite.revoked"));
    },
    onError: (err) => toast.error(err.message || t("office.app_invite.error")),
  });

  const submit = (e: FormEvent) => {
    e.preventDefault();
    if (email.trim()) create.mutate();
  };

  const items = (list.data?.items ?? []).filter(
    (x) => x.status === "pending" || x.status === "expired",
  );
  const day = (v: string) =>
    new Date(v).toLocaleDateString(locale === "tr" ? "tr-TR" : "en-GB");

  return (
    <section
      aria-labelledby="app-invite-title"
      className="bg-surface border-line rounded-xl border p-4 md:p-5"
      data-testid="app-invite-section"
    >
      <div className="mb-3 space-y-1">
        <h2 id="app-invite-title" className="font-display text-base font-bold">
          {t("office.app_invite.title")}
        </h2>
        <p className="text-text-2 text-sm">
          {t("office.app_invite.description")}
        </p>
      </div>

      {canWrite ? (
        <form
          onSubmit={submit}
          className="mb-4 flex flex-wrap items-end gap-3"
          data-testid="app-invite-form"
        >
          <div className="min-w-56 flex-1 space-y-1">
            <Label htmlFor="app-invite-email">
              {t("office.app_invite.email")}
            </Label>
            <Input
              id="app-invite-email"
              type="email"
              required
              value={email}
              onChange={(e) => setEmail(e.target.value)}
              placeholder="ornek@sirket.com"
            />
          </div>
          <div className="space-y-1">
            <Label htmlFor="app-invite-role">
              {t("office.app_invite.role")}
            </Label>
            <NativeSelect
              id="app-invite-role"
              value={role}
              onChange={(e) =>
                setRole(e.target.value as TaxpayerInvitationRole)
              }
            >
              <NativeSelectOption value="owner">
                {t("office.app_invite.role_owner")}
              </NativeSelectOption>
              <NativeSelectOption value="employee">
                {t("office.app_invite.role_employee")}
              </NativeSelectOption>
            </NativeSelect>
          </div>
          <Button type="submit" disabled={create.isPending}>
            <Send className="size-4" aria-hidden />
            {t("office.app_invite.send")}
          </Button>
        </form>
      ) : null}

      {last ? (
        <div className="border-line mb-4 rounded-lg border p-3">
          <InviteResult result={last} />
        </div>
      ) : null}

      <h3 className="mb-2 text-sm font-semibold">
        {t("office.app_invite.pending_title")}
      </h3>
      {list.isLoading ? (
        <Skeleton className="h-16 w-full" />
      ) : list.isError ? (
        <ErrorState
          title={t("office.app_invite.error")}
          onRetry={() => void list.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : items.length === 0 ? (
        <p className="text-text-2 text-sm">{t("office.app_invite.empty")}</p>
      ) : (
        <ul className="divide-line divide-y" data-testid="app-invite-list">
          {items.map((x) => {
            const known = links[x.uuid];
            return (
              <li
                key={x.uuid}
                className="flex flex-wrap items-center justify-between gap-2 py-2"
                data-testid="app-invite-row"
              >
                <div className="min-w-0 space-y-0.5">
                  <p className="truncate text-sm font-medium">{x.email}</p>
                  <p className="text-text-3 text-xs">
                    {x.role === "owner"
                      ? t("office.app_invite.role_owner")
                      : t("office.app_invite.role_employee")}
                    {" · "}
                    {t("office.app_invite.expires", {
                      date: day(x.expires_at),
                    })}
                  </p>
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  <StatusPill tone={STATUS_TONE[x.status]}>
                    {t(`office.app_invite.status.${x.status}`)}
                  </StatusPill>
                  {known ? <InviteLinkActions url={known.invite_url} /> : null}
                  {canWrite ? (
                    <>
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={resend.isPending}
                        onClick={() => resend.mutate(x.uuid)}
                        data-testid="invite-resend"
                      >
                        <RotateCw className="size-4" aria-hidden />
                        {t("office.app_invite.resend")}
                      </Button>
                      {x.status === "pending" ? (
                        <Button
                          size="sm"
                          variant="ghost"
                          disabled={revoke.isPending}
                          onClick={() => revoke.mutate(x.uuid)}
                        >
                          {t("office.app_invite.revoke")}
                        </Button>
                      ) : null}
                    </>
                  ) : null}
                </div>
              </li>
            );
          })}
        </ul>
      )}
      {canWrite && items.length > 0 ? (
        <p className="text-text-3 mt-2 text-xs">
          {t("office.app_invite.resend_hint")}
        </p>
      ) : null}
    </section>
  );
}
