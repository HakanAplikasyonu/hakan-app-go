"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";

export type ParasutConnection = components["schemas"]["ParasutConnection"];
export type ParasutCompany = components["schemas"]["ParasutCompany"];
export type KolayBiConnection = components["schemas"]["KolayBiConnection"];
export type SyncReport = components["schemas"]["ParasutReport"];
export type Company = { id: string; name: string; tax_number?: string };

/** Ön muhasebe programının bağlantı adımı. */
export type AccountingStep = "authorize" | "company" | "active" | "failed";

/**
 * Paraşüt: taslak / yetkisi düşmüş bağlantı onay ister; onaylanmış ama
 * şirket seçilmemiş olan şirket ister. KolayBi'de onay adımı yoktur (API
 * anahtarı bağlarken doğrulanır).
 */
export function accountingStep(conn: {
  setup_state: string;
  status: string;
  company_id?: string;
}): AccountingStep {
  if (conn.status === "auth_failed") return "failed";
  if (conn.setup_state === "draft") return "authorize";
  if (!conn.company_id) return "company";
  return conn.status === "active" ? "active" : "failed";
}

/** Raporun okunur özeti: yazılan fatura ve masraf, olası mükerrer. */
export function reportTotals(r?: SyncReport | null) {
  if (!r) return null;
  const excluded = Object.values(r.excluded ?? {}).reduce((a, b) => a + b, 0);
  return {
    invoices: r.invoices_created,
    expenses: r.expenses_created + r.expenses_updated,
    duplicates: r.possible_duplicates,
    excluded,
  };
}

// Paraşüt onay sayfası tek, sabit bir adrese döner (PARASUT_REDIRECT_URL);
// dönüşte hangi ofis ve mükellef olduğunu tarayıcı oturumu hatırlar.
const RETURN_KEY = "hakan.parasut.return";

export type ParasutReturn = {
  slug: string;
  engagement: string;
  connection: string;
};

export function rememberParasutReturn(v: ParasutReturn) {
  try {
    sessionStorage.setItem(RETURN_KEY, JSON.stringify(v));
  } catch {
    // Özel pencere: dönüş sayfası ne yapacağını bilemez, kullanıcı sekmeye döner.
  }
}

/** Kaydı okur ve siler; `state` başka bağlantıya aitse null. */
export function takeParasutReturn(state: string | null): ParasutReturn | null {
  try {
    const raw = sessionStorage.getItem(RETURN_KEY);
    sessionStorage.removeItem(RETURN_KEY);
    if (!raw) return null;
    const v = JSON.parse(raw) as ParasutReturn;
    if (!v.slug || !v.engagement || !v.connection) return null;
    if (state && state !== v.connection) return null;
    return v;
  } catch {
    return null;
  }
}

const enc = encodeURIComponent;
export const parasutBase = (engagement: string) =>
  `/v1/tenant/engagements/${enc(engagement)}/parasut-connections`;
const kolaybiBase = (engagement: string) =>
  `/v1/tenant/engagements/${enc(engagement)}/kolaybi-connections`;

/** Mükellefin Paraşüt bağlantıları (#518, A3). */
export function useParasut(engagement: string) {
  const { slug } = useTenant();
  const qc = useQueryClient();
  const key = ["tenant", slug.toLowerCase(), "parasut-connections", engagement];
  const one = (uuid: string) => `${parasutBase(engagement)}/${enc(uuid)}`;
  const invalidate = () => qc.invalidateQueries({ queryKey: key });

  const connections = useQuery({
    queryKey: key,
    queryFn: () =>
      tenantRequest<{ items: ParasutConnection[]; configured: boolean }>(
        slug,
        "GET",
        parasutBase(engagement),
      ),
  });
  const create = useMutation({
    mutationFn: () =>
      tenantRequest<ParasutConnection>(slug, "POST", parasutBase(engagement), {
        silent: true,
      }),
    onSuccess: invalidate,
  });
  const companies = (uuid: string) =>
    tenantRequest<{ items: ParasutCompany[] }>(
      slug,
      "GET",
      `${one(uuid)}/companies`,
      { silent: true },
    );
  const company = useMutation({
    mutationFn: (v: { uuid: string; company_id: string }) =>
      tenantRequest<ParasutConnection>(slug, "PUT", `${one(v.uuid)}/company`, {
        body: { company_id: v.company_id },
        silent: true,
      }),
    onSuccess: invalidate,
  });
  const sync = useMutation({
    mutationFn: (v: { uuid: string; from: string; to: string }) =>
      tenantRequest<SyncReport>(slug, "POST", `${one(v.uuid)}/sync`, {
        body: { from: v.from, to: v.to },
        silent: true,
      }),
    onSuccess: invalidate,
  });
  return { slug, connections, create, companies, company, sync };
}

/** Mükellefin KolayBi bağlantıları (#518, A4). */
export function useKolayBi(engagement: string) {
  const { slug } = useTenant();
  const qc = useQueryClient();
  const key = ["tenant", slug.toLowerCase(), "kolaybi-connections", engagement];
  const one = (uuid: string) => `${kolaybiBase(engagement)}/${enc(uuid)}`;
  const invalidate = () => qc.invalidateQueries({ queryKey: key });

  const connections = useQuery({
    queryKey: key,
    queryFn: () =>
      tenantRequest<{ items: KolayBiConnection[]; configured: boolean }>(
        slug,
        "GET",
        kolaybiBase(engagement),
      ),
  });
  const connect = useMutation({
    mutationFn: (body: { api_key: string }) =>
      tenantRequest<{ connection: KolayBiConnection; companies: Company[] }>(
        slug,
        "POST",
        kolaybiBase(engagement),
        { body, silent: true },
      ),
    onSuccess: invalidate,
  });
  const company = useMutation({
    mutationFn: (v: { uuid: string; company_id: string }) =>
      tenantRequest<KolayBiConnection>(slug, "PUT", `${one(v.uuid)}/company`, {
        body: { company_id: v.company_id },
        silent: true,
      }),
    onSuccess: invalidate,
  });
  const sync = useMutation({
    mutationFn: (v: { uuid: string; from: string; to: string }) =>
      tenantRequest<SyncReport>(slug, "POST", `${one(v.uuid)}/sync`, {
        body: { from: v.from, to: v.to },
        silent: true,
      }),
    onSuccess: invalidate,
  });
  return { connections, connect, company, sync };
}
