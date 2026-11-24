"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";

export type EasytradeConnection = components["schemas"]["EasytradeConnection"];
export type EasytradeSyncRun = components["schemas"]["EasytradeSyncRun"];
export type EasytradeItemStats = components["schemas"]["EasytradeItemStats"];
export type EasytradeFinancialSummary =
  components["schemas"]["EnvelopeEasytradeFinancialSummary"]["data"];
export type EasytradeFinancialTransaction =
  components["schemas"]["EasytradeFinancialTransaction"];
export type EasytradeZReportList =
  components["schemas"]["EnvelopeEasytradeZReportList"]["data"];
export type EasytradeZReport = components["schemas"]["EasytradeZReport"];
export type EasytradeConnectRequest =
  components["schemas"]["EasytradeConnectRequest"];
export type EasytradeConnectionTest =
  components["schemas"]["EasytradeConnectionTest"];

/** Easytrade'in firma ID'si yalnızca rakamdır (6 haneli Müşteri No değil). */
export const EASYTRADE_ID_RE = /^[0-9]{1,18}$/;

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/easytrade-connections`;

/** Easytrade bağlantıları, senkron geçmişi ve dönem görünümleri (#385). */
export function useEasytrade(engagement: string, period: string) {
  const { slug } = useTenant();
  const qc = useQueryClient();
  const key = ["tenant", slug.toLowerCase(), "easytrade", engagement];

  const connections = useQuery({
    queryKey: [...key, "connections"],
    queryFn: () =>
      tenantRequest<{ items: EasytradeConnection[] }>(
        slug,
        "GET",
        base(engagement),
      ),
    // Zamanlanmış senkron arka planda ilerler: ekran dakikada bir tazelenir.
    refetchInterval: 60_000,
  });
  const conn = connections.data?.items[0];
  const one = conn ? `${base(engagement)}/${conn.uuid}` : "";

  const runs = useQuery({
    queryKey: [...key, "runs", conn?.uuid],
    enabled: !!conn,
    queryFn: () =>
      tenantRequest<{ items: EasytradeSyncRun[] }>(
        slug,
        "GET",
        `${one}/sync-runs`,
      ),
  });
  const financial = useQuery({
    queryKey: [...key, "financial", conn?.uuid, period],
    enabled: !!conn,
    queryFn: () =>
      tenantRequest<EasytradeFinancialSummary>(
        slug,
        "GET",
        `${one}/financial-transactions`,
        { query: { period } },
      ),
  });
  const zReports = useQuery({
    queryKey: [...key, "z", conn?.uuid, period],
    enabled: !!conn,
    queryFn: () =>
      tenantRequest<EasytradeZReportList>(slug, "GET", `${one}/z-reports`, {
        query: { period },
      }),
  });

  const invalidate = () => qc.invalidateQueries({ queryKey: key });
  const connect = useMutation({
    mutationFn: (body: EasytradeConnectRequest) =>
      tenantRequest<EasytradeConnection>(slug, "POST", base(engagement), {
        body,
        silent: true,
      }),
    onSuccess: invalidate,
  });
  // Bağlantıyı test et: hiçbir şey kaydedilmez.
  const testConnection = useMutation({
    mutationFn: (body: EasytradeConnectRequest) =>
      tenantRequest<EasytradeConnectionTest>(
        slug,
        "POST",
        `${base(engagement)}/test`,
        { body, silent: true },
      ),
  });
  const schedule = useMutation({
    mutationFn: (minutes: number | null) =>
      tenantRequest<EasytradeConnection>(slug, "PATCH", one, {
        body: { sync_interval_minutes: minutes },
        silent: true,
      }),
    onSuccess: invalidate,
  });
  const syncNow = useMutation({
    mutationFn: (range: { from: string; to: string }) =>
      tenantRequest(slug, "POST", `${one}/sync`, { body: range, silent: true }),
    onSuccess: invalidate,
  });

  return {
    connections,
    conn,
    runs,
    financial,
    zReports,
    connect,
    testConnection,
    schedule,
    syncNow,
  };
}

/** Dönemin ilk günü ile son günü (bugünü geçmez): elle senkron aralığı. */
export function periodRange(period: string, today = new Date()) {
  const [y, m] = period.split("-").map(Number);
  const pad = (n: number) => String(n).padStart(2, "0");
  const last = new Date(Date.UTC(y, m, 0)).getUTCDate();
  const iso = `${today.getFullYear()}-${pad(today.getMonth() + 1)}-${pad(today.getDate())}`;
  const end = `${y}-${pad(m)}-${pad(last)}`;
  return { from: `${y}-${pad(m)}-01`, to: end < iso ? end : iso };
}
