"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";

export type EinvoiceIntegrator = components["schemas"]["EinvoiceIntegrator"];
export type EinvoiceConnection = components["schemas"]["EinvoiceConnection"];
export type EinvoiceConnectionTest =
  components["schemas"]["EinvoiceConnectionTest"];

/**
 * Entegratör hatasının ekrandaki türü (sources.einvoice.test_*): bağlantı
 * testinin `kind` değeri de son çekimin go-ubltr hata türü de buraya iner.
 */
export function failureKind(kind?: string | null) {
  switch (kind) {
    case "auth":
    case "permission":
    case "timeout":
    case "unavailable":
      return kind;
    case "transport":
      return "unavailable";
    case "rate_limit":
    case "rate-limit":
      return "rate_limit";
    default:
      return "unknown";
  }
}

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/einvoice-connections`;

/** Formdan gelen değerler: boşlukları kırpılmış, boş olanlar atılmış. */
export function cleanValues(values: Record<string, string>) {
  const out: Record<string, string> = {};
  for (const [k, v] of Object.entries(values)) {
    const s = v.trim();
    if (s) out[k] = s;
  }
  return out;
}

/**
 * Bağlantı bekliyor mu: kimlik girildi, ilk çekim sırada ya da sürüyor.
 * Bu sürede liste kendini yeniler; sonuç (aktif / reddedildi) işçiden gelir.
 */
export function isWaiting(conn: Pick<EinvoiceConnection, "state">) {
  return conn.state === "waiting";
}

/** Mükellefin e-fatura entegratör bağlantıları (#518, A1). */
export function useEinvoiceConnections(engagement: string) {
  const { slug } = useTenant();
  const qc = useQueryClient();
  const key = [
    "tenant",
    slug.toLowerCase(),
    "einvoice-connections",
    engagement,
  ];

  const integrators = useQuery({
    queryKey: ["tenant", slug.toLowerCase(), "einvoice-integrators"],
    queryFn: () =>
      tenantRequest<{ items: EinvoiceIntegrator[] }>(
        slug,
        "GET",
        "/v1/tenant/einvoice-integrators",
      ),
    staleTime: 5 * 60_000,
  });
  const connections = useQuery({
    queryKey: key,
    queryFn: () =>
      tenantRequest<{ items: EinvoiceConnection[] }>(
        slug,
        "GET",
        base(engagement),
      ),
    refetchInterval: (q) =>
      q.state.data?.items.some(isWaiting) ? 10_000 : false,
  });

  const invalidate = () => qc.invalidateQueries({ queryKey: key });
  const one = (uuid: string) =>
    `${base(engagement)}/${encodeURIComponent(uuid)}`;

  const create = useMutation({
    mutationFn: (body: { integrator_id: string; environment: string }) =>
      tenantRequest<EinvoiceConnection>(slug, "POST", base(engagement), {
        body,
        silent: true,
      }),
    onSuccess: invalidate,
  });
  const credentials = useMutation({
    mutationFn: (v: { uuid: string; values: Record<string, string> }) =>
      tenantRequest<EinvoiceConnection>(
        slug,
        "PUT",
        `${one(v.uuid)}/credentials`,
        { body: { values: cleanValues(v.values) }, silent: true },
      ),
    onSuccess: invalidate,
  });
  const sync = useMutation({
    mutationFn: (uuid: string) =>
      tenantRequest<EinvoiceConnection>(slug, "POST", `${one(uuid)}/sync`, {
        silent: true,
      }),
    onSuccess: invalidate,
  });

  const test = useMutation({
    mutationFn: (v: { uuid: string; values?: Record<string, string> }) =>
      tenantRequest<EinvoiceConnectionTest>(
        slug,
        "POST",
        `${one(v.uuid)}/test`,
        {
          body: { values: cleanValues(v.values ?? {}) },
          silent: true,
        },
      ),
  });

  return { integrators, connections, create, credentials, sync, test };
}
