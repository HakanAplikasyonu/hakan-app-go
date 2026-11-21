"use client";

import { useQuery } from "@tanstack/react-query";
import { Mail, MessageSquareText, Printer } from "lucide-react";
import { useMemo, useState } from "react";

import { ErrorState } from "@/components/common/error-state";
import { Loading } from "@/components/common/loading";
import { EntityPage } from "@/components/entity";
import { StatusPill, type StatusTone } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { FilterChip } from "@/components/ui/filter-chip";
import { permissions } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  communicationKeys,
  communicationService,
  type CommunicationItem,
} from "../services/communication.service";

type Filter = "all" | "sms" | "email";

/** Ekrandaki durum: SMS iletildi mi, e-posta açıldı / indirildi mi. */
type Outcome =
  | "delivered"
  | "sent"
  | "opened"
  | "downloaded"
  | "unopened"
  | "failed"
  | "uncertain"
  | "dry_run"
  | "pending";

const outcomeTone: Record<Outcome, StatusTone> = {
  delivered: "ok",
  opened: "ok",
  downloaded: "ok",
  sent: "info",
  unopened: "warn",
  failed: "danger",
  uncertain: "warn",
  dry_run: "neutral",
  pending: "info",
};

export function outcomeOf(m: CommunicationItem): Outcome {
  if (m.status === "sent") {
    if (m.channel === "sms") return m.delivered_at ? "delivered" : "sent";
    if (m.downloaded_at) return "downloaded";
    return m.opened_at ? "opened" : "unopened";
  }
  return m.status;
}

/** "E-posta açılmadı, SMS gitti" yedek gönderimi (kural kodunda iz). */
export function isMailFallback(m: CommunicationItem): boolean {
  return m.channel === "sms" && m.source_rule.includes("eposta_acilmadi");
}

/**
 * Mükellef → İletişim geçmişi (#780 D): ofisin bu mükellefe gönderdiği SMS
 * ve e-postalar, yeniden eskiye. "Bana söylemediniz" diyen mükellefe
 * gösterilecek kanıt: saniyesiyle gönderim zamanı, numara / adres, metin,
 * NetGSM iş no, iletildi / açıldı / indirildi zamanı. Yazdır düğmesi kanıtı
 * kâğıda ya da PDF'e döker.
 */
export function EngagementCommunicationsPage({
  engagementUuid,
}: {
  engagementUuid: string;
}) {
  const { t, locale } = useLocale();
  const tenant = useTenant();
  const [filter, setFilter] = useState<Filter>("all");
  const query = useQuery({
    queryKey: communicationKeys.history(tenant.slug, engagementUuid, "all"),
    queryFn: () => communicationService.history(tenant.slug, engagementUuid),
  });
  const all = useMemo(() => query.data?.items ?? [], [query.data]);
  const items = useMemo(
    () => (filter === "all" ? all : all.filter((m) => m.channel === filter)),
    [all, filter],
  );
  const counts = useMemo(
    () => ({
      all: all.length,
      sms: all.filter((m) => m.channel === "sms").length,
      email: all.filter((m) => m.channel === "email").length,
    }),
    [all],
  );
  const stamp = useMemo(
    () =>
      new Intl.DateTimeFormat(locale === "en" ? "en-GB" : "tr-TR", {
        day: "2-digit",
        month: "long",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
        timeZone: "Europe/Istanbul",
      }),
    [locale],
  );
  const when = (v: string | null | undefined) =>
    v ? stamp.format(new Date(v)) : null;

  const filters: { key: Filter; label: string }[] = [
    { key: "all", label: t("office.communications.filter_all") },
    { key: "sms", label: t("office.communications.filter_sms") },
    { key: "email", label: t("office.communications.filter_email") },
  ];

  return (
    <EntityPage
      title={t("office.communications.title")}
      description={t("office.communications.description")}
      permission={permissions.tenant.engagements.read}
      headingLevel={2}
      forbiddenFallback={
        <ErrorState
          title={t("common.error_forbidden")}
          description={t("errors.forbidden_body")}
        />
      }
      breadcrumbs={[
        {
          label: tenant.organization.name,
          href: routes.tenant.home(tenant.slug),
        },
        { label: t("office.communications.title") },
      ]}
      actions={
        all.length > 0 ? (
          <Button
            variant="outline"
            size="sm"
            onClick={() => window.print()}
            data-testid="communications-print"
          >
            <Printer className="size-4" aria-hidden />
            {t("office.communications.print")}
          </Button>
        ) : null
      }
    >
      <div className="space-y-4" data-testid="communications-page">
        <div
          role="group"
          aria-label={t("office.communications.filter_label")}
          className="flex flex-wrap gap-2 print:hidden"
        >
          {filters.map((f) => (
            <FilterChip
              key={f.key}
              pressed={filter === f.key}
              onPressedChange={() => setFilter(f.key)}
              data-testid={`communications-filter-${f.key}`}
            >
              {f.label}
              <span className="ms-1.5 tabular-nums">{counts[f.key]}</span>
            </FilterChip>
          ))}
        </div>

        {query.isLoading ? <Loading label={t("common.loading")} /> : null}
        {query.isError ? (
          <ErrorState
            title={t("common.error_generic")}
            onRetry={() => void query.refetch()}
            retryLabel={t("common.retry")}
          />
        ) : null}
        {query.isSuccess && items.length === 0 ? (
          <p
            className="text-text-2 border-line rounded-md border border-dashed p-6 text-sm"
            data-testid="communications-empty"
          >
            {all.length === 0
              ? t("office.communications.empty")
              : t("office.communications.empty_filter")}
          </p>
        ) : null}

        <ul className="space-y-3" data-testid="communications-list">
          {items.map((m) => {
            const outcome = outcomeOf(m);
            const Icon = m.channel === "sms" ? MessageSquareText : Mail;
            const facts: { label: string; value: string | null }[] = [
              {
                label: t("office.communications.fact_created"),
                value: when(m.created_at),
              },
              {
                label: t("office.communications.fact_sent"),
                value: when(m.sent_at),
              },
              ...(m.channel === "sms"
                ? [
                    {
                      label: t("office.communications.fact_delivered"),
                      value:
                        when(m.delivered_at) ??
                        (m.status === "sent"
                          ? t("office.communications.not_yet")
                          : null),
                    },
                    {
                      label: t("office.communications.fact_job"),
                      value: m.provider_job_id || null,
                    },
                    {
                      label: t("office.communications.fact_header"),
                      value: m.msgheader || null,
                    },
                  ]
                : [
                    {
                      label: t("office.communications.fact_opened"),
                      value:
                        when(m.opened_at) ??
                        (m.status === "sent"
                          ? t("office.communications.not_opened")
                          : null),
                    },
                    {
                      label: t("office.communications.fact_downloaded"),
                      value: when(m.downloaded_at),
                    },
                  ]),
              {
                label: t("office.communications.fact_sender"),
                value: m.sent_by_name || t("office.communications.automatic"),
              },
              {
                label: t("office.communications.fact_rule"),
                value: m.source_rule || null,
              },
            ];
            return (
              <li
                key={m.uuid}
                className="border-line bg-surface break-inside-avoid space-y-3 rounded-lg border p-4"
                data-testid="communication-item"
                data-channel={m.channel}
                data-outcome={outcome}
              >
                <div className="flex flex-wrap items-center gap-2">
                  <span className="bg-surface-3 text-text-2 inline-flex items-center gap-1.5 rounded-md px-2 py-1 text-xs font-medium">
                    <Icon className="size-3.5" aria-hidden />
                    {t(`office.communications.channel_${m.channel}`)}
                  </span>
                  <StatusPill tone={outcomeTone[outcome]}>
                    {t(`office.communications.outcome_${outcome}`)}
                  </StatusPill>
                  <span className="figure font-medium break-all">{m.to}</span>
                  <span className="text-text-3 ms-auto text-sm">
                    {when(m.sent_at ?? m.created_at)}
                  </span>
                </div>
                {isMailFallback(m) ? (
                  <p
                    className="bg-warn-tint text-warn rounded-md px-3 py-2 text-sm"
                    data-testid="communication-fallback"
                  >
                    {t("office.communications.fallback_note")}
                  </p>
                ) : null}
                {m.subject ? (
                  <p className="text-sm font-semibold">{m.subject}</p>
                ) : null}
                <p className="text-text-2 text-sm whitespace-pre-wrap">
                  {m.body.trim()}
                </p>
                {m.error_message ? (
                  <p className="text-text-3 text-xs">{m.error_message}</p>
                ) : null}
                <dl className="grid gap-x-6 gap-y-1 text-sm sm:grid-cols-2">
                  {facts
                    .filter((f) => f.value)
                    .map((f) => (
                      <div key={f.label} className="flex gap-2">
                        <dt className="text-text-3 shrink-0">{f.label}</dt>
                        <dd className="figure break-all">{f.value}</dd>
                      </div>
                    ))}
                </dl>
              </li>
            );
          })}
        </ul>
      </div>
    </EntityPage>
  );
}
