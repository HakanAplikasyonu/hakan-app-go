"use client";

import { useQuery } from "@tanstack/react-query";
import { ArrowRight, FileDown, Landmark, Plus, RefreshCw } from "lucide-react";
import Link from "next/link";
import { useState, type FormEvent } from "react";
import { toast } from "sonner";

import { ErrorState } from "@/components/common/error-state";
import { BosDurum } from "@/components/hakan";
import { StatusPill, formatCellDate } from "@/components/tables";
import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
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
import { Textarea } from "@/components/ui/textarea";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { tenantRequest } from "@/lib/api/tenant-request";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  bankStep,
  lastDays,
  splitIBANs,
  useBankConnections,
  type BankConnection,
  type BankStep,
} from "../hooks/use-bank-connections";
import type { EasytradeConnection } from "../hooks/use-easytrade";

import { AccountingSection } from "./accounting-connections-section";
import { EinvoiceSection } from "./einvoice-connections-section";

type BC = ReturnType<typeof useBankConnections>;

const STEPS: BankStep[] = ["instruction", "submit", "credentials", "active"];

const statusTone: Record<string, "ok" | "warn" | "danger" | "neutral"> = {
  active: "ok",
  rate_limited: "warn",
  auth_failed: "danger",
  disabled: "danger",
  pending: "neutral",
};

function errText(err: unknown, fallback: string) {
  return isApiError(err) ? err.message : fallback;
}

/**
 * `/t/{slug}/engagements/{uuid}/connections` (#518): mükellefe özgü bütün
 * dış bağlantılar tek yerde. Banka web servisi (talimat → şube → servis
 * kullanıcısı → aktif) ve e-fatura entegratörü burada kurulur; Easytrade, muhasebe programı aktarımı
 * ve WhatsApp kendi sekmelerine bağlanır.
 */
export function EngagementConnectionsPage({
  engagement,
}: {
  engagement: string;
}) {
  const { t } = useLocale();
  const { can } = usePermission();

  return (
    <div className="space-y-6">
      <div className="min-w-0">
        <h2 className="font-display truncate text-xl font-bold">
          {t("sources.connections.title")}
        </h2>
        <p className="text-text-2 text-sm">
          {t("sources.connections.description")}
        </p>
      </div>
      {can(Permission.TenantBankRead) ? (
        <BankSection engagement={engagement} />
      ) : null}
      {can(Permission.TenantInvoicesRead) ? (
        <EinvoiceSection engagement={engagement} />
      ) : null}
      {can(Permission.TenantInvoicesRead) ? (
        <AccountingSection engagement={engagement} />
      ) : null}
      <OtherConnections engagement={engagement} />
    </div>
  );
}

function BankSection({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const bc = useBankConnections(engagement);
  const [open, setOpen] = useState(false);
  const canWrite = can(Permission.TenantBankWrite);
  const items = bc.connections.data?.items ?? [];

  return (
    <section aria-labelledby="bank-ws" className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <h3 id="bank-ws" className="font-display text-lg font-semibold">
            {t("sources.bank.title")}
          </h3>
          <p className="text-text-3 text-sm">{t("sources.bank.description")}</p>
        </div>
        {canWrite && items.length > 0 ? (
          <Button variant="outline" onClick={() => setOpen(true)}>
            <Plus aria-hidden className="size-4" />
            {t("sources.bank.add")}
          </Button>
        ) : null}
      </div>

      {bc.connections.isLoading ? (
        <Skeleton className="h-32 w-full" />
      ) : bc.connections.isError ? (
        <ErrorState
          title={t("office.api.load_error")}
          description={bc.connections.error.message}
          onRetry={() => void bc.connections.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : items.length === 0 ? (
        <BosDurum
          baslik={t("sources.bank.empty_title")}
          aciklama={t("sources.bank.empty")}
          eylem={
            canWrite ? (
              <Button onClick={() => setOpen(true)}>
                <Landmark aria-hidden className="size-4" />
                {t("sources.bank.add")}
              </Button>
            ) : null
          }
        />
      ) : (
        <div className="space-y-3">
          {items.map((c) => (
            <BankCard key={c.uuid} conn={c} bc={bc} canWrite={canWrite} />
          ))}
        </div>
      )}
      {canWrite ? (
        <AddBankDialog open={open} onOpenChange={setOpen} bc={bc} />
      ) : null}
    </section>
  );
}

function AddBankDialog({
  open,
  onOpenChange,
  bc,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  bc: BC;
}) {
  const { t } = useLocale();
  const banks = bc.services.data?.items ?? [];
  const [bank, setBank] = useState("");
  const [ibans, setIbans] = useState("");
  const chosen = bank || banks[0]?.bank_id || "";

  const submit = (e: FormEvent) => {
    e.preventDefault();
    bc.create.mutate(
      { bank_id: chosen, ibans: splitIBANs(ibans) },
      {
        onSuccess: () => {
          toast.success(t("sources.bank.created"));
          setIbans("");
          onOpenChange(false);
        },
        onError: (err) =>
          toast.error(errText(err, t("sources.bank.create_failed"))),
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t("sources.bank.add_title")}</DialogTitle>
            <DialogDescription>{t("sources.bank.add_hint")}</DialogDescription>
          </DialogHeader>
          {bc.services.isError ? (
            <p role="alert" className="text-danger text-sm">
              {bc.services.error.message}
            </p>
          ) : banks.length === 0 && !bc.services.isLoading ? (
            <p className="text-text-3 text-sm">{t("sources.bank.no_banks")}</p>
          ) : (
            <>
              <div className="space-y-1.5">
                <Label htmlFor="bank-id">{t("sources.bank.f_bank")}</Label>
                <Select value={chosen} onValueChange={setBank}>
                  <SelectTrigger id="bank-id">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {banks.map((b) => (
                      <SelectItem key={b.bank_id} value={b.bank_id}>
                        {b.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="bank-ibans">{t("sources.bank.f_ibans")}</Label>
                <Textarea
                  id="bank-ibans"
                  rows={3}
                  placeholder="TR00 0000 0000 0000 0000 0000 00"
                  value={ibans}
                  onChange={(e) => setIbans(e.target.value)}
                />
                <p className="text-text-3 text-xs">
                  {t("sources.bank.f_ibans_hint")}
                </p>
              </div>
            </>
          )}
          <DialogFooter>
            <Button
              type="button"
              variant="outline"
              onClick={() => onOpenChange(false)}
            >
              {t("common.cancel")}
            </Button>
            <Button type="submit" disabled={!chosen || bc.create.isPending}>
              {t("sources.bank.create")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function BankCard({
  conn,
  bc,
  canWrite,
}: {
  conn: BankConnection;
  bc: BC;
  canWrite: boolean;
}) {
  const { t, locale } = useLocale();
  const step = bankStep(conn);
  const dt = (v?: string | null) =>
    v ? formatCellDate(v, locale, "datetime") : t("sources.bank.never");

  return (
    <article className="bg-surface border-line space-y-4 rounded-xl border p-4">
      <div className="flex flex-wrap items-center gap-2">
        <h4 className="font-display text-base font-semibold">
          {conn.bank_name || conn.bank_id}
        </h4>
        <StatusPill tone={statusTone[conn.status] ?? "neutral"} dot>
          {t(`sources.bank.state_${conn.setup_state}`)}
        </StatusPill>
        {conn.environment === "test" ? (
          <StatusPill tone="warn">{t("sources.bank.env_test")}</StatusPill>
        ) : null}
        <span className="text-text-3 truncate font-mono text-xs">
          {conn.ibans.join(" · ") || t("sources.bank.no_ibans")}
        </span>
      </div>

      <ol
        aria-label={t("sources.bank.steps")}
        className="grid gap-2 sm:grid-cols-4"
      >
        {STEPS.map((s, i) => {
          const done = STEPS.indexOf(step) > i;
          const current = step === s;
          return (
            <li
              key={s}
              aria-current={current ? "step" : undefined}
              className={cn(
                "rounded-lg border px-3 py-2 text-xs",
                done && "border-ok text-ok",
                current && "border-brand bg-brand-soft font-medium",
                !done && !current && "text-text-3",
              )}
            >
              {i + 1}. {t(`sources.bank.step_${s}`)}
            </li>
          );
        })}
      </ol>

      {conn.error ? (
        <p
          role="alert"
          className="bg-danger-tint text-danger rounded-lg px-3 py-2 text-sm"
        >
          {conn.error.message}
          {conn.error.code ? ` (${conn.error.code})` : ""}
        </p>
      ) : null}

      {canWrite ? <BankStepAction conn={conn} step={step} bc={bc} /> : null}

      <dl className="grid gap-3 sm:grid-cols-3">
        <Fact
          label={t("sources.bank.submitted_at")}
          value={dt(conn.submitted_at)}
        />
        <Fact
          label={t("sources.bank.verified_at")}
          value={dt(conn.verified_at)}
        />
        <Fact
          label={t("sources.bank.last_sync")}
          value={dt(conn.last_sync_at)}
        />
      </dl>
    </article>
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

function BankStepAction({
  conn,
  step,
  bc,
}: {
  conn: BankConnection;
  step: BankStep;
  bc: BC;
}) {
  const { t } = useLocale();
  const download = () =>
    bc.instruction.mutate(conn.uuid, {
      onError: (err) =>
        toast.error(
          String(err).includes("503")
            ? t("sources.bank.egress_missing")
            : t("sources.bank.instruction_failed"),
        ),
    });
  const instructionButton = (
    <Button
      variant={step === "instruction" ? "default" : "outline"}
      onClick={download}
      disabled={bc.instruction.isPending}
    >
      <FileDown aria-hidden className="size-4" />
      {t("sources.bank.instruction")}
    </Button>
  );

  if (step === "instruction" || step === "submit") {
    return (
      <div className="flex flex-wrap items-center gap-2">
        {instructionButton}
        {step === "submit" ? (
          <Button
            onClick={() =>
              bc.submitted.mutate(conn.uuid, {
                onSuccess: () => toast.success(t("sources.bank.submitted")),
                onError: (err) =>
                  toast.error(errText(err, t("sources.bank.action_failed"))),
              })
            }
            disabled={bc.submitted.isPending}
          >
            {t("sources.bank.mark_submitted")}
          </Button>
        ) : null}
        <p className="text-text-3 w-full text-xs">
          {t(`sources.bank.hint_${step}`)}
        </p>
      </div>
    );
  }
  if (step === "credentials") return <CredentialsForm conn={conn} bc={bc} />;

  return (
    <div className="flex flex-wrap items-center gap-2">
      <Button
        variant="outline"
        disabled={bc.sync.isPending || conn.status === "disabled"}
        onClick={() =>
          bc.sync.mutate(
            { uuid: conn.uuid, ...lastDays(7) },
            {
              onSuccess: (r) =>
                toast.success(
                  t("sources.bank.synced", {
                    inserted: r.inserted,
                    merged: r.merged,
                  }),
                ),
              onError: (err) =>
                toast.error(errText(err, t("sources.bank.sync_failed"))),
            },
          )
        }
      >
        <RefreshCw
          aria-hidden
          className={cn("size-4", bc.sync.isPending && "animate-spin")}
        />
        {t("sources.bank.sync_now")}
      </Button>
    </div>
  );
}

function CredentialsForm({ conn, bc }: { conn: BankConnection; bc: BC }) {
  const { t } = useLocale();
  const [values, setValues] = useState<Record<string, string>>({});
  const submit = (e: FormEvent) => {
    e.preventDefault();
    bc.credentials.mutate(
      { uuid: conn.uuid, values },
      {
        onSuccess: (c) => {
          setValues({});
          if (c.setup_state === "active")
            toast.success(t("sources.bank.verified"));
          else toast.error(c.error?.message ?? t("sources.bank.verify_failed"));
        },
        onError: (err) =>
          toast.error(errText(err, t("sources.bank.verify_failed"))),
      },
    );
  };
  return (
    <form
      onSubmit={submit}
      className="grid gap-3 sm:grid-cols-3 sm:items-end"
      autoComplete="off"
    >
      {conn.credential_fields.map((f) => (
        <div key={f.key} className="space-y-1.5">
          <Label htmlFor={`bank-${conn.uuid}-${f.key}`}>{f.label}</Label>
          <Input
            id={`bank-${conn.uuid}-${f.key}`}
            type={f.secret ? "password" : "text"}
            autoComplete={f.secret ? "new-password" : "off"}
            pattern={f.pattern || undefined}
            required
            value={values[f.key] ?? ""}
            onChange={(e) => setValues({ ...values, [f.key]: e.target.value })}
          />
          {f.help ? <p className="text-text-3 text-xs">{f.help}</p> : null}
        </div>
      ))}
      <Button type="submit" disabled={bc.credentials.isPending}>
        {t("sources.bank.verify")}
      </Button>
      <p className="text-text-3 text-xs sm:col-span-3">
        {t("sources.bank.hint_credentials")}
      </p>
    </form>
  );
}

/** Kendi sekmesi olan bağlantılar: durum ve kısa yol. */
function OtherConnections({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const { can } = usePermission();
  const easytrade = useQuery({
    queryKey: [
      "tenant",
      slug.toLowerCase(),
      "easytrade",
      engagement,
      "connections",
    ],
    queryFn: () =>
      tenantRequest<{ items: EasytradeConnection[] }>(
        slug,
        "GET",
        `/v1/tenant/engagements/${encodeURIComponent(engagement)}/easytrade-connections`,
      ),
  });
  const et = easytrade.data?.items[0];
  const cards: {
    key: string;
    href: string;
    status?: string;
    tone?: "ok" | "warn" | "danger" | "neutral";
  }[] = [
    {
      key: "easytrade",
      href: routes.tenant.engagementEasytrade(slug, engagement),
      status: easytrade.isLoading
        ? undefined
        : et
          ? t(`sources.easytrade.status_${et.status}`)
          : t("sources.connections.not_connected"),
      tone: et ? (statusTone[et.status] ?? "neutral") : "neutral",
    },
    ...(can(Permission.TenantJournalsRead)
      ? [
          {
            key: "accounting",
            href: routes.tenant.engagementJournals(slug, engagement),
          },
        ]
      : []),
    { key: "whatsapp", href: routes.tenant.whatsapp(slug) },
  ];
  return (
    <section aria-labelledby="other-conn" className="space-y-3">
      <h3 id="other-conn" className="font-display text-lg font-semibold">
        {t("sources.connections.other_title")}
      </h3>
      <ul className="grid gap-3 sm:grid-cols-3">
        {cards.map((c) => (
          <li key={c.key}>
            <Link
              href={c.href}
              className="bg-surface border-line hover:border-brand focus-visible:ring-brand flex h-full flex-col gap-2 rounded-xl border p-4 transition-colors focus-visible:ring-2 focus-visible:outline-none"
            >
              <span className="flex items-center justify-between gap-2 font-medium">
                {t(`sources.connections.${c.key}_title`)}
                <ArrowRight aria-hidden className="text-text-3 size-4" />
              </span>
              <span className="text-text-3 text-sm">
                {t(`sources.connections.${c.key}_desc`)}
              </span>
              {c.status ? (
                <StatusPill tone={c.tone} dot className="self-start">
                  {c.status}
                </StatusPill>
              ) : null}
            </Link>
          </li>
        ))}
      </ul>
    </section>
  );
}
