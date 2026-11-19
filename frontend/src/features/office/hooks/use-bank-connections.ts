"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";

import { downloadFile } from "../services/download";

export type BankService = components["schemas"]["BankService"];
export type BankConnection = components["schemas"]["BankConnection"];
export type BankCredentialField = components["schemas"]["BankCredentialField"];
export type BankSyncResult =
  components["schemas"]["EnvelopeBankSyncResult"]["data"];

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/bank-connections`;

/**
 * Bağlantının sıradaki adımı (20 §5.2): talimat → şubeye teslim → servis
 * kullanıcısı → aktif. `failed` bağlantı kimlik bilgisini yeniden ister.
 */
export type BankStep = "instruction" | "submit" | "credentials" | "active";

export function bankStep(conn: Pick<BankConnection, "setup_state">): BankStep {
  switch (conn.setup_state) {
    case "draft":
      return "instruction";
    case "form_ready":
      return "submit";
    case "active":
      return "active";
    default:
      // submitted_to_bank, credentials_entered, verifying, failed
      return "credentials";
  }
}

/** Boşluk ve satır sonlarıyla ayrılmış IBAN'lar (boşluklu yazım korunmaz). */
export function splitIBANs(text: string): string[] {
  const out: string[] = [];
  for (const line of text.split(/[\n,;]+/)) {
    const v = line.replace(/\s+/g, "").toUpperCase();
    if (v && !out.includes(v)) out.push(v);
  }
  return out;
}

/** Mükellefin banka web servisi bağlantıları (#518, A2). */
export function useBankConnections(engagement: string) {
  const { slug } = useTenant();
  const qc = useQueryClient();
  const key = ["tenant", slug.toLowerCase(), "bank-connections", engagement];

  const services = useQuery({
    queryKey: ["tenant", slug.toLowerCase(), "bank-services"],
    queryFn: () =>
      tenantRequest<{ items: BankService[] }>(
        slug,
        "GET",
        "/v1/tenant/bank-services",
      ),
    staleTime: 5 * 60_000,
  });
  const connections = useQuery({
    queryKey: key,
    queryFn: () =>
      tenantRequest<{ items: BankConnection[] }>(slug, "GET", base(engagement)),
  });

  const invalidate = () => qc.invalidateQueries({ queryKey: key });
  const one = (uuid: string) =>
    `${base(engagement)}/${encodeURIComponent(uuid)}`;

  const create = useMutation({
    mutationFn: (body: { bank_id: string; ibans: string[] }) =>
      tenantRequest<BankConnection>(slug, "POST", base(engagement), {
        body,
        silent: true,
      }),
    onSuccess: invalidate,
  });
  const instruction = useMutation({
    mutationFn: (uuid: string) =>
      downloadFile(slug, `${one(uuid)}/instruction`, {}, "banka-talimati.pdf"),
    // `draft` → `form_ready`.
    onSuccess: invalidate,
  });
  const submitted = useMutation({
    mutationFn: (uuid: string) =>
      tenantRequest<BankConnection>(slug, "POST", `${one(uuid)}/submitted`, {
        silent: true,
      }),
    onSuccess: invalidate,
  });
  const credentials = useMutation({
    mutationFn: (v: { uuid: string; values: Record<string, string> }) =>
      tenantRequest<BankConnection>(slug, "PUT", `${one(v.uuid)}/credentials`, {
        body: { values: v.values },
        silent: true,
      }),
    onSuccess: invalidate,
  });
  const sync = useMutation({
    mutationFn: (v: { uuid: string; from: string; to: string }) =>
      tenantRequest<BankSyncResult>(slug, "POST", `${one(v.uuid)}/sync`, {
        body: { from: v.from, to: v.to },
        silent: true,
      }),
    onSuccess: invalidate,
  });

  return {
    services,
    connections,
    create,
    instruction,
    submitted,
    credentials,
    sync,
  };
}
