"use client";

import { ArrowRight, FileText, PlugZap, Plus, RefreshCw } from "lucide-react";
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
import { Progress } from "@/components/ui/progress";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Skeleton } from "@/components/ui/skeleton";
import { Permission } from "@/config/permissions";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { isApiError } from "@/lib/api";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";
import { usePermission } from "@/providers/permission-provider";

import {
  failureKind,
  useEinvoiceConnections,
  type EinvoiceConnection,
  type EinvoiceConnectionTest,
} from "../hooks/use-einvoice-connections";

type EC = ReturnType<typeof useEinvoiceConnections>;

const stateTone: Record<string, "ok" | "warn" | "danger" | "neutral"> = {
  active: "ok",
  waiting: "warn",
  rate_limited: "warn",
  auth_failed: "danger",
  disabled: "danger",
  draft: "neutral",
};

function errText(err: unknown, fallback: string) {
  return isApiError(err) ? err.message : fallback;
}

type TestState = { result: EinvoiceConnectionTest } | { error: unknown } | null;

/** "Bağlantıyı test et" sonucu: başarı ya da Türkçe neden + teknik kod. */
function TestOutcome({ state }: { state: TestState }) {
  const { t } = useLocale();
  if (!state) return null;
  if ("error" in state) {
    const err = state.error;
    const msg =
      isApiError(err) && err.status === 409
        ? t("sources.einvoice.need_user")
        : isApiError(err) && err.status === 503
          ? t("sources.einvoice.vault_missing")
          : errText(err, t("sources.einvoice.test_failed"));
    return (
      <p
        role="alert"
        className="bg-danger-tint text-danger rounded-lg px-3 py-2 text-sm"
      >
        {msg}
      </p>
    );
  }
  const r = state.result;
  if (r.ok) {
    return (
      <p
        role="status"
        data-testid="einvoice-test-result"
        className="bg-ok-tint text-ok rounded-lg px-3 py-2 text-sm"
      >
        {t("sources.einvoice.test_ok")}
      </p>
    );
  }
  return (
    <div
      role="alert"
      data-testid="einvoice-test-result"
      className="bg-danger-tint text-danger space-y-0.5 rounded-lg px-3 py-2 text-sm"
    >
      <p>{t(`sources.einvoice.test_${failureKind(r.kind)}`)}</p>
      {r.code ? (
        <p className="text-xs opacity-80">
          {t("sources.einvoice.tech_code", { code: r.code })}
        </p>
      ) : null}
    </div>
  );
}

/**
 * Bağlantılar sekmesinde e-fatura entegratörü (#518, A1): ofis entegratörü
 * seçer, web servis kullanıcısını girer; ilk çekim sabit IP'li işçide
 * çalışır ve bağlantıyı aktif ya da "reddedildi" yapar.
 */
export function EinvoiceSection({ engagement }: { engagement: string }) {
  const { t } = useLocale();
  const { can } = usePermission();
  const ec = useEinvoiceConnections(engagement);
  const [open, setOpen] = useState(false);
  const canWrite = can(Permission.TenantInvoicesWrite);
  const items = ec.connections.data?.items ?? [];

  return (
    <section aria-labelledby="einvoice-conn" className="space-y-3">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="min-w-0">
          <h3 id="einvoice-conn" className="font-display text-lg font-semibold">
            {t("sources.einvoice.title")}
          </h3>
          <p className="text-text-3 text-sm">
            {t("sources.einvoice.description")}
          </p>
        </div>
        {canWrite && items.length > 0 ? (
          <Button variant="outline" onClick={() => setOpen(true)}>
            <Plus aria-hidden className="size-4" />
            {t("sources.einvoice.add")}
          </Button>
        ) : null}
      </div>

      {ec.connections.isLoading ? (
        <Skeleton className="h-28 w-full" />
      ) : ec.connections.isError ? (
        <ErrorState
          title={t("office.api.load_error")}
          description={ec.connections.error.message}
          onRetry={() => void ec.connections.refetch()}
          retryLabel={t("common.retry")}
        />
      ) : items.length === 0 ? (
        <BosDurum
          baslik={t("sources.einvoice.empty_title")}
          aciklama={t("sources.einvoice.empty")}
          eylem={
            canWrite ? (
              <Button onClick={() => setOpen(true)}>
                <FileText aria-hidden className="size-4" />
                {t("sources.einvoice.add")}
              </Button>
            ) : null
          }
        />
      ) : (
        <div className="space-y-3">
          {items.map((c) => (
            <EinvoiceCard
              key={c.uuid}
              conn={c}
              ec={ec}
              canWrite={canWrite}
              engagement={engagement}
            />
          ))}
        </div>
      )}
      {canWrite ? (
        <AddDialog
          open={open}
          onOpenChange={setOpen}
          ec={ec}
          taken={items.map((c) => c.integrator_id)}
        />
      ) : null}
    </section>
  );
}

function AddDialog({
  open,
  onOpenChange,
  ec,
  taken,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  ec: EC;
  taken: string[];
}) {
  const { t } = useLocale();
  const list = (ec.integrators.data?.items ?? []).filter(
    (i) => !taken.includes(i.id),
  );
  const [picked, setPicked] = useState("");
  const [env, setEnv] = useState("production");
  const chosen = list.some((i) => i.id === picked)
    ? picked
    : (list[0]?.id ?? "");

  const submit = (e: FormEvent) => {
    e.preventDefault();
    ec.create.mutate(
      { integrator_id: chosen, environment: env },
      {
        onSuccess: () => {
          toast.success(t("sources.einvoice.created"));
          onOpenChange(false);
        },
        onError: (err) =>
          toast.error(errText(err, t("sources.einvoice.create_failed"))),
      },
    );
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <form onSubmit={submit} className="space-y-4">
          <DialogHeader>
            <DialogTitle>{t("sources.einvoice.add_title")}</DialogTitle>
            <DialogDescription>
              {t("sources.einvoice.add_hint")}
            </DialogDescription>
          </DialogHeader>
          {ec.integrators.isError ? (
            <p role="alert" className="text-danger text-sm">
              {ec.integrators.error.message}
            </p>
          ) : list.length === 0 && !ec.integrators.isLoading ? (
            <p className="text-text-3 text-sm">
              {t("sources.einvoice.none_left")}
            </p>
          ) : (
            <>
              <div className="space-y-1.5">
                <Label htmlFor="einvoice-id">
                  {t("sources.einvoice.f_integrator")}
                </Label>
                <Select value={chosen} onValueChange={setPicked}>
                  <SelectTrigger id="einvoice-id">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {list.map((i) => (
                      <SelectItem key={i.id} value={i.id}>
                        {i.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="einvoice-env">
                  {t("sources.einvoice.f_environment")}
                </Label>
                <Select value={env} onValueChange={setEnv}>
                  <SelectTrigger id="einvoice-env">
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value="production">
                      {t("sources.einvoice.env_production")}
                    </SelectItem>
                    <SelectItem value="test">
                      {t("sources.einvoice.env_test")}
                    </SelectItem>
                  </SelectContent>
                </Select>
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
            <Button type="submit" disabled={!chosen || ec.create.isPending}>
              {t("sources.einvoice.create")}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

/**
 * İlk yükleme ilerlemesi (#780): önce bu ay ve geçen ay gelir (liste +
 * fatura), sonra geçmiş aylar yeniden eskiye listelenir. Son iki ay hazır
 * olunca fatura listesine geçiş düğmesi çıkar.
 */
function FirstLoadProgress({
  progress,
  engagement,
}: {
  progress: NonNullable<EinvoiceConnection["progress"]>;
  engagement: string;
}) {
  const { t } = useLocale();
  const { slug } = useTenant();
  const p = progress;
  const pct =
    p.history_months_total > 0
      ? Math.round((p.history_months_done / p.history_months_total) * 100)
      : 100;
  const status =
    p.phase === "queued"
      ? t("sources.einvoice.progress_queued")
      : p.phase === "recent"
        ? t("sources.einvoice.progress_recent", {
            n: p.this_month_documents,
          })
        : p.phase === "history"
          ? t("sources.einvoice.progress_history", {
              n: p.this_month_documents,
              done: p.history_months_done,
              total: p.history_months_total,
            })
          : t("sources.einvoice.progress_done", {
              n: p.this_month_documents,
              total: p.history_months_total,
            });
  return (
    <div
      data-testid="einvoice-progress"
      data-phase={p.phase}
      className="bg-surface-2 space-y-2 rounded-lg px-3 py-3"
    >
      <p role="status" aria-live="polite" className="text-sm font-medium">
        {status}
      </p>
      {p.phase !== "queued" ? (
        <Progress
          value={pct}
          aria-label={t("sources.einvoice.progress_label")}
          className="h-1.5"
        />
      ) : null}
      {p.recent_ready ? (
        <div className="flex flex-wrap items-center justify-between gap-2">
          <p className="text-text-3 text-xs">
            {t("sources.einvoice.progress_ready", {
              n: p.this_month_invoices,
            })}
          </p>
          <Button asChild size="sm">
            <Link
              href={routes.tenant.engagementInvoices(slug, engagement)}
              data-testid="einvoice-go-invoices"
            >
              {t("sources.einvoice.go_invoices")}
              <ArrowRight aria-hidden className="size-4" />
            </Link>
          </Button>
        </div>
      ) : null}
    </div>
  );
}

function EinvoiceCard({
  conn,
  ec,
  canWrite,
  engagement,
}: {
  conn: EinvoiceConnection;
  ec: EC;
  canWrite: boolean;
  engagement: string;
}) {
  const { t, locale } = useLocale();
  const [editing, setEditing] = useState(false);
  const [tested, setTested] = useState<TestState>(null);
  const runTest = () => {
    setTested(null);
    ec.test.mutate(
      { uuid: conn.uuid },
      {
        onSuccess: (result) => setTested({ result }),
        onError: (error) => setTested({ error }),
      },
    );
  };
  const dt = (v?: string | null) =>
    v ? formatCellDate(v, locale, "datetime") : t("sources.einvoice.never");
  const askCredentials =
    canWrite &&
    !conn.ip_filtered &&
    (conn.state === "draft" || conn.state === "auth_failed" || editing);
  const run = conn.last_run;

  return (
    <article className="bg-surface border-line space-y-4 rounded-xl border p-4">
      <div className="flex flex-wrap items-center gap-2">
        <h4 className="font-display text-base font-semibold">
          {conn.integrator_name}
        </h4>
        <StatusPill tone={stateTone[conn.state] ?? "neutral"} dot>
          {t(`sources.einvoice.state_${conn.state}`)}
        </StatusPill>
        {conn.environment === "test" ? (
          <StatusPill tone="warn">{t("sources.einvoice.env_test")}</StatusPill>
        ) : null}
      </div>

      {conn.error ? (
        <p
          role="alert"
          className="bg-danger-tint text-danger rounded-lg px-3 py-2 text-sm"
        >
          {conn.state === "auth_failed"
            ? t("sources.einvoice.auth_failed")
            : failureKind(conn.error.kind) === "unknown"
              ? t("sources.einvoice.last_error")
              : t(`sources.einvoice.test_${failureKind(conn.error.kind)}`)}
          {conn.error.kind || conn.error.code ? (
            <span className="mt-0.5 block text-xs opacity-80">
              {t("sources.einvoice.tech_code", {
                code: [conn.error.kind, conn.error.code]
                  .filter(Boolean)
                  .join(":"),
              })}
            </span>
          ) : null}
        </p>
      ) : null}

      {conn.progress ? (
        <FirstLoadProgress progress={conn.progress} engagement={engagement} />
      ) : null}

      {conn.state === "waiting" ? (
        <p className="text-text-3 text-sm">
          {conn.ip_filtered
            ? t("sources.einvoice.hint_ip_filtered")
            : t("sources.einvoice.hint_waiting")}
        </p>
      ) : null}

      {askCredentials ? (
        <CredentialsForm conn={conn} ec={ec} onDone={() => setEditing(false)} />
      ) : canWrite && conn.state !== "draft" ? (
        <div className="flex flex-wrap items-center gap-2">
          <Button
            variant="outline"
            disabled={
              ec.sync.isPending ||
              conn.state === "disabled" ||
              conn.state === "waiting"
            }
            onClick={() =>
              ec.sync.mutate(conn.uuid, {
                onSuccess: () => toast.success(t("sources.einvoice.queued")),
                onError: (err) =>
                  toast.error(
                    isApiError(err) && err.code === "QUEUE_NOT_CONFIGURED"
                      ? t("sources.einvoice.unavailable")
                      : isApiError(err) && err.status === 409
                        ? t("sources.einvoice.need_user")
                        : errText(err, t("sources.einvoice.sync_failed")),
                  ),
              })
            }
          >
            <RefreshCw
              aria-hidden
              className={cn("size-4", ec.sync.isPending && "animate-spin")}
            />
            {t("sources.einvoice.sync_now")}
          </Button>
          <Button
            variant="outline"
            disabled={ec.test.isPending}
            onClick={runTest}
          >
            <PlugZap aria-hidden className="size-4" />
            {ec.test.isPending
              ? t("sources.einvoice.testing")
              : t("sources.einvoice.test")}
          </Button>
          {!conn.ip_filtered ? (
            <Button variant="ghost" onClick={() => setEditing(true)}>
              {t("sources.einvoice.change_user")}
            </Button>
          ) : null}
        </div>
      ) : null}

      {!askCredentials ? <TestOutcome state={tested} /> : null}

      <dl className="grid gap-3 sm:grid-cols-3">
        <Fact
          label={t("sources.einvoice.last_success")}
          value={dt(conn.last_success_at)}
        />
        <Fact
          label={t("sources.einvoice.last_run")}
          value={
            run
              ? `${dt(run.finished_at ?? run.started_at)} · ${t(`sources.einvoice.run_${run.status === "running" ? "running" : run.status === "failed" ? "failed" : "done"}`)}`
              : t("sources.einvoice.never")
          }
        />
        <Fact
          label={t("sources.einvoice.last_new")}
          value={
            run
              ? t("sources.einvoice.counts", {
                  new: run.counters.new ?? 0,
                  listed: run.counters.listed ?? 0,
                })
              : "—"
          }
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

function CredentialsForm({
  conn,
  ec,
  onDone,
}: {
  conn: EinvoiceConnection;
  ec: EC;
  onDone: () => void;
}) {
  const { t } = useLocale();
  const [values, setValues] = useState<Record<string, string>>({});
  const [tested, setTested] = useState<TestState>(null);
  const runTest = () => {
    setTested(null);
    ec.test.mutate(
      { uuid: conn.uuid, values },
      {
        onSuccess: (result) => setTested({ result }),
        onError: (error) => setTested({ error }),
      },
    );
  };
  const submit = (e: FormEvent) => {
    e.preventDefault();
    ec.credentials.mutate(
      { uuid: conn.uuid, values },
      {
        onSuccess: () => {
          setValues({});
          onDone();
          toast.success(t("sources.einvoice.saved"));
        },
        onError: (err) =>
          toast.error(
            isApiError(err) && err.status === 503
              ? t("sources.einvoice.vault_missing")
              : errText(err, t("sources.einvoice.save_failed")),
          ),
      },
    );
  };
  return (
    <form
      onSubmit={submit}
      className="grid gap-3 sm:grid-cols-3 sm:items-end"
      autoComplete="off"
    >
      {conn.fields.map((f) => (
        <div key={f.key} className="space-y-1.5">
          <Label htmlFor={`einv-${conn.uuid}-${f.key}`}>{f.label}</Label>
          <Input
            id={`einv-${conn.uuid}-${f.key}`}
            type={f.secret ? "password" : "text"}
            autoComplete={f.secret ? "new-password" : "off"}
            required={f.required}
            value={values[f.key] ?? ""}
            onChange={(e) => setValues({ ...values, [f.key]: e.target.value })}
          />
          {f.help ? <p className="text-text-3 text-xs">{f.help}</p> : null}
        </div>
      ))}
      <div className="flex flex-wrap gap-2">
        <Button type="submit" disabled={ec.credentials.isPending}>
          {t("sources.einvoice.save")}
        </Button>
        <Button
          type="button"
          variant="outline"
          disabled={ec.test.isPending}
          onClick={runTest}
        >
          <PlugZap aria-hidden className="size-4" />
          {ec.test.isPending
            ? t("sources.einvoice.testing")
            : t("sources.einvoice.test")}
        </Button>
      </div>
      <div className="sm:col-span-3">
        <TestOutcome state={tested} />
      </div>
      <p className="text-text-3 text-xs sm:col-span-3">
        {t("sources.einvoice.hint_credentials")}
      </p>
    </form>
  );
}
