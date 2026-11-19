"use client";

import { useQuery } from "@tanstack/react-query";
import { ExternalLink, RefreshCw } from "lucide-react";
import { useState, type FormEvent, type ReactNode } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { StatusPill, formatCellDate } from "@/components/tables";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Permission } from "@/config/permissions";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  accountingStep,
  rememberParasutReturn,
  reportTotals,
  useKolayBi,
  useParasut,
  type AccountingStep,
  type Company,
  type SyncReport,
} from "../hooks/use-accounting-connections";
import { lastDays } from "../hooks/use-bank-connections";

const stepTone: Record<AccountingStep, "ok" | "warn" | "danger" | "neutral"> = {
  authorize: "neutral",
  company: "warn",
  active: "ok",
  failed: "danger",
};

function errText(err: unknown, fallback: string) {
  return isApiError(err) ? err.message : fallback;
}

/** Ön muhasebe programları (#518, A3–A4): Paraşüt ve KolayBi. */
export function AccountingSection({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const canWrite = can(Permission.TenantInvoicesWrite);
  return (
    <section aria-labelledby="accounting-conn" className="space-y-3">
      <div className="min-w-0">
        <h3 id="accounting-conn" className="font-display text-lg font-semibold">
          {t("sources.accounting.title")}
        </h3>
        <p className="text-text-3 text-sm">
          {t("sources.accounting.description")}
        </p>
      </div>
      <div className="grid gap-3 lg:grid-cols-2">
        <ParasutCard engagement={engagement} canWrite={canWrite} />
        <KolayBiCard engagement={engagement} canWrite={canWrite} />
      </div>
    </section>
  );
}

function Shell({
  name,
  step,
  children,
}: {
  name: string;
  step?: AccountingStep;
  children: ReactNode;
}) {
  const { t } = useLocale();
  return (
    <article className="bg-surface border-line flex flex-col gap-4 rounded-xl border p-4">
      <div className="flex flex-wrap items-center gap-2">
        <h4 className="font-display text-base font-semibold">{name}</h4>
        <StatusPill tone={step ? stepTone[step] : "neutral"} dot>
          {step
            ? t(`sources.accounting.step_${step}`)
            : t("sources.connections.not_connected")}
        </StatusPill>
      </div>
      {children}
    </article>
  );
}

function Facts({
  company,
  lastSync,
  report,
}: {
  company?: string;
  lastSync?: string;
  report?: SyncReport | null;
}) {
  const { t, locale } = useLocale();
  const totals = reportTotals(report);
  return (
    <dl className="grid gap-3 sm:grid-cols-3">
      <Fact label={t("sources.accounting.company")} value={company || "—"} />
      <Fact
        label={t("sources.accounting.last_sync")}
        value={
          lastSync
            ? formatCellDate(lastSync, locale, "datetime")
            : t("sources.accounting.never")
        }
      />
      <Fact
        label={t("sources.accounting.last_report")}
        value={totals ? t("sources.accounting.report", totals) : "—"}
      />
    </dl>
  );
}

function Fact({ label, value }: { label: string; value: string }) {
  return (
    <div className="min-w-0">
      <dt className="text-text-3 text-xs">{label}</dt>
      <dd className="truncate text-sm font-medium tabular-nums">{value}</dd>
    </div>
  );
}

function Problem({ text }: { text?: string }) {
  if (!text) return null;
  return (
    <p
      role="alert"
      className="bg-danger-tint text-danger rounded-lg px-3 py-2 text-sm"
    >
      {text}
    </p>
  );
}

function syncToast(t: ReturnType<typeof useLocale>["t"]) {
  return {
    onSuccess: (r: SyncReport) => {
      const totals = reportTotals(r);
      if (totals) toast.success(t("sources.accounting.synced", totals));
    },
    onError: (err: unknown) =>
      toast.error(
        isApiError(err) && err.status === 429
          ? t("sources.accounting.throttled")
          : errText(err, t("sources.accounting.sync_failed")),
      ),
  };
}

function SyncButton({
  pending,
  onClick,
}: {
  pending: boolean;
  onClick: () => void;
}) {
  const { t } = useLocale();
  return (
    <Button variant="outline" disabled={pending} onClick={onClick}>
      <RefreshCw
        aria-hidden
        className={cn("size-4", pending && "animate-spin")}
      />
      {t("sources.accounting.sync_now")}
    </Button>
  );
}

function CompanyPicker({
  id,
  companies,
  pending,
  onPick,
}: {
  id: string;
  companies: Company[];
  pending: boolean;
  onPick: (companyId: string) => void;
}) {
  const { t } = useLocale();
  const [picked, setPicked] = useState("");
  const chosen = picked || companies[0]?.id || "";
  if (companies.length === 0) {
    return (
      <p className="text-text-3 text-sm">
        {t("sources.accounting.no_company")}
      </p>
    );
  }
  return (
    <form
      className="flex flex-wrap items-end gap-2"
      onSubmit={(e) => {
        e.preventDefault();
        onPick(chosen);
      }}
    >
      <div className="min-w-48 flex-1 space-y-1.5">
        <Label htmlFor={id}>{t("sources.accounting.f_company")}</Label>
        <Select value={chosen} onValueChange={setPicked}>
          <SelectTrigger id={id}>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            {companies.map((c) => (
              <SelectItem key={c.id} value={c.id}>
                {c.name}
                {c.tax_number ? ` · ${c.tax_number}` : ""}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>
      <Button type="submit" disabled={!chosen || pending}>
        {t("sources.accounting.pick_company")}
      </Button>
    </form>
  );
}

function ParasutCard({
  engagement,
  canWrite,
}: {
  engagement: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const p = useParasut(engagement);
  // Oldest first: the newest connection is the one being set up.
  const conn = p.connections.data?.items.at(-1);
  const step = conn ? accountingStep(conn) : undefined;
  const companies = useQuery({
    queryKey: ["parasut-companies", p.slug, engagement, conn?.uuid],
    queryFn: () => p.companies(conn!.uuid),
    enabled: canWrite && step === "company" && !!conn,
  });

  const goAuthorize = (uuid: string, url?: string) => {
    if (!url) {
      toast.error(t("sources.accounting.not_configured"));
      return;
    }
    rememberParasutReturn({ slug: p.slug, engagement, connection: uuid });
    window.location.assign(url);
  };
  const start = () =>
    p.create.mutate(undefined, {
      onSuccess: (c) => goAuthorize(c.uuid, c.authorize_url),
      onError: (err) =>
        toast.error(
          isApiError(err) && err.status === 503
            ? t("sources.accounting.not_configured")
            : errText(err, t("sources.accounting.create_failed")),
        ),
    });

  if (p.connections.isLoading) return <Skeleton className="h-40 w-full" />;
  if (p.connections.isError) {
    return (
      <ErrorState
        title={t("office.api.load_error")}
        description={p.connections.error.message}
        onRetry={() => void p.connections.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  return (
    <Shell name="Paraşüt" step={step}>
      <p className="text-text-3 text-sm">
        {t("sources.accounting.parasut_desc")}
      </p>
      <Problem text={conn?.error} />
      {canWrite && !conn ? (
        <Button
          onClick={start}
          disabled={p.create.isPending}
          className="self-start"
        >
          <ExternalLink aria-hidden className="size-4" />
          {t("sources.accounting.parasut_connect")}
        </Button>
      ) : null}
      {canWrite && conn && (step === "authorize" || step === "failed") ? (
        <div className="space-y-1">
          <Button
            onClick={() => goAuthorize(conn.uuid, conn.authorize_url)}
            className="self-start"
          >
            <ExternalLink aria-hidden className="size-4" />
            {t("sources.accounting.parasut_authorize")}
          </Button>
          <p className="text-text-3 text-xs">
            {t("sources.accounting.parasut_hint")}
          </p>
        </div>
      ) : null}
      {canWrite && conn && step === "company" ? (
        companies.isLoading ? (
          <Skeleton className="h-10 w-full" />
        ) : companies.isError ? (
          <Problem
            text={errText(
              companies.error,
              t("sources.accounting.companies_failed"),
            )}
          />
        ) : (
          <CompanyPicker
            id={`parasut-co-${conn.uuid}`}
            companies={companies.data?.items ?? []}
            pending={p.company.isPending}
            onPick={(company_id) =>
              p.company.mutate(
                { uuid: conn.uuid, company_id },
                {
                  onSuccess: () => toast.success(t("sources.accounting.bound")),
                  onError: (err) =>
                    toast.error(
                      errText(err, t("sources.accounting.bind_failed")),
                    ),
                },
              )
            }
          />
        )
      ) : null}
      {canWrite && conn && step === "active" ? (
        <SyncButton
          pending={p.sync.isPending}
          onClick={() =>
            p.sync.mutate({ uuid: conn.uuid, ...lastDays(30) }, syncToast(t))
          }
        />
      ) : null}
      {conn ? (
        <Facts
          company={conn.company_name}
          lastSync={conn.last_sync_at}
          report={conn.last_report}
        />
      ) : null}
    </Shell>
  );
}

function KolayBiCard({
  engagement,
  canWrite,
}: {
  engagement: string;
  canWrite: boolean;
}) {
  const { t } = useLocale();
  const k = useKolayBi(engagement);
  const conn = k.connections.data?.items.at(-1);
  const step = conn ? accountingStep(conn) : undefined;
  const [apiKey, setApiKey] = useState("");
  // The company list comes only with the connect answer (KolayBi has no
  // list route); a reload before choosing asks to connect again.
  const [companies, setCompanies] = useState<Company[]>([]);

  const connect = (e: FormEvent) => {
    e.preventDefault();
    k.connect.mutate(
      { api_key: apiKey.trim() },
      {
        onSuccess: (r) => {
          setApiKey("");
          setCompanies(r.companies);
          toast.success(
            r.connection.company_id
              ? t("sources.accounting.bound")
              : t("sources.accounting.kolaybi_pick"),
          );
        },
        onError: (err) =>
          toast.error(
            isApiError(err) && err.status === 502
              ? t("sources.accounting.kolaybi_refused")
              : isApiError(err) && err.status === 503
                ? t("sources.accounting.not_configured")
                : errText(err, t("sources.accounting.create_failed")),
          ),
      },
    );
  };

  if (k.connections.isLoading) return <Skeleton className="h-40 w-full" />;
  if (k.connections.isError) {
    return (
      <ErrorState
        title={t("office.api.load_error")}
        description={k.connections.error.message}
        onRetry={() => void k.connections.refetch()}
        retryLabel={t("common.retry")}
      />
    );
  }

  return (
    <Shell name="KolayBi" step={step}>
      <p className="text-text-3 text-sm">
        {t("sources.accounting.kolaybi_desc")}
      </p>
      <Problem text={conn?.error} />
      {canWrite &&
      (!conn ||
        step === "failed" ||
        (step === "company" && companies.length === 0)) ? (
        <form
          onSubmit={connect}
          className="flex flex-wrap items-end gap-2"
          autoComplete="off"
        >
          <div className="min-w-48 flex-1 space-y-1.5">
            <Label htmlFor={`kolaybi-key-${engagement}`}>
              {t("sources.accounting.f_api_key")}
            </Label>
            <Input
              id={`kolaybi-key-${engagement}`}
              type="password"
              autoComplete="new-password"
              required
              value={apiKey}
              onChange={(e) => setApiKey(e.target.value)}
            />
          </div>
          <Button
            type="submit"
            disabled={!apiKey.trim() || k.connect.isPending}
          >
            {conn
              ? t("sources.accounting.kolaybi_reconnect")
              : t("sources.accounting.kolaybi_connect")}
          </Button>
          <p className="text-text-3 w-full text-xs">
            {t("sources.accounting.kolaybi_hint")}
          </p>
        </form>
      ) : null}
      {canWrite && conn && step === "company" && companies.length > 0 ? (
        <CompanyPicker
          id={`kolaybi-co-${conn.uuid}`}
          companies={companies}
          pending={k.company.isPending}
          onPick={(company_id) =>
            k.company.mutate(
              { uuid: conn.uuid, company_id },
              {
                onSuccess: () => toast.success(t("sources.accounting.bound")),
                onError: (err) =>
                  toast.error(
                    errText(err, t("sources.accounting.bind_failed")),
                  ),
              },
            )
          }
        />
      ) : null}
      {canWrite && conn && step === "active" ? (
        <SyncButton
          pending={k.sync.isPending}
          onClick={() =>
            k.sync.mutate({ uuid: conn.uuid, ...lastDays(30) }, syncToast(t))
          }
        />
      ) : null}
      {conn ? (
        <Facts
          company={conn.company_name}
          lastSync={conn.last_sync_at}
          report={conn.last_report}
        />
      ) : null}
    </Shell>
  );
}
