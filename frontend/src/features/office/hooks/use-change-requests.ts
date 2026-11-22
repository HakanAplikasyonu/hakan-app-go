"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";

export type ChangeRequest = components["schemas"]["ChangeRequest"];

/**
 * Talebin nasıl uygulanacağı (Oğuz'un demo geri bildirimi): "apply" eski
 * davranış (kayıt kendi kilitli dönemine yazılır), "correction" geçmiş
 * döneme düzeltme olarak, "target_period" seçilen aya.
 */
export type ApplyMode = "apply" | "correction" | "target_period";

export type DecideChoice =
  | { action: "reject" }
  | { action: "apply"; mode?: ApplyMode; target_period?: string };

export type DecideInput = DecideChoice & { uuid: string };

/** Değişen bir alan: eski ve yeni değer (diff.fields). */
export type FieldChange = { field: string; old: string; new: string };

function show(v: unknown): string {
  if (v === null || v === undefined || v === "") return "—";
  if (typeof v === "object") return JSON.stringify(v);
  return String(v);
}

/**
 * `diff` özetini okur: `{kind, source: {system, capture_method}, fields:
 * {ad: {old, new}}}`. Bilinmeyen biçimde boş döner, ekran çökmez.
 */
export function readDiff(diff: Record<string, unknown> | undefined) {
  const src = (diff?.source ?? {}) as Record<string, unknown>;
  const raw = (diff?.fields ?? {}) as Record<string, unknown>;
  const fields: FieldChange[] = [];
  for (const [field, v] of Object.entries(raw)) {
    const pair = (v ?? {}) as Record<string, unknown>;
    fields.push({ field, old: show(pair.old), new: show(pair.new) });
  }
  fields.sort((a, b) => a.field.localeCompare(b.field));
  // Gelen faturanın özeti (#710); eski taleplerde yoksa alanlardan.
  const sum = (diff?.summary ?? {}) as Record<string, unknown>;
  const pick = (k: string, field: string) => {
    if (typeof sum[k] === "string" && sum[k]) return sum[k] as string;
    const pair = (raw[field] ?? {}) as Record<string, unknown>;
    return typeof pair.new === "string" ? pair.new : "";
  };
  return {
    source: typeof src.system === "string" ? src.system : "",
    method: typeof src.capture_method === "string" ? src.capture_method : "",
    fields,
    summary: {
      number: pick("invoice_number", "invoice_number"),
      direction: pick("direction", "direction"),
      counterparty: pick("counterparty", "customer_name"),
      payable: pick("payable_amount", "payable_amount"),
      currency: pick("currency", "currency") || "TRY",
    },
  };
}

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/change-requests`;

/** Kilitli dönemlere gelen açık değişiklik talepleri (#518, B2). */
export function useChangeRequests(engagement: string, enabled = true) {
  const { slug } = useTenant();
  const qc = useQueryClient();
  const key = ["tenant", slug.toLowerCase(), "change-requests", engagement];
  const list = useQuery({
    queryKey: key,
    queryFn: () =>
      tenantRequest<{ items: ChangeRequest[]; total: number }>(
        slug,
        "GET",
        base(engagement),
        { query: { status: "open", limit: 50 } },
      ),
    enabled,
  });
  const done = () => {
    void qc.invalidateQueries({ queryKey: key });
    // Dönem şeridindeki "değişiklik var" sayısı da değişir.
    void qc.invalidateQueries({
      queryKey: ["tenant", slug.toLowerCase(), "periods"],
    });
  };
  const decide = useMutation({
    mutationFn: (v: DecideInput) =>
      tenantRequest<ChangeRequest>(
        slug,
        "POST",
        `${base(engagement)}/${encodeURIComponent(v.uuid)}/${v.action}`,
        {
          silent: true,
          body:
            v.action === "apply" && v.mode && v.mode !== "apply"
              ? {
                  mode: v.mode,
                  ...(v.mode === "target_period"
                    ? { target_period: v.target_period }
                    : {}),
                }
              : undefined,
        },
      ),
    onSuccess: done,
  });
  return { list, decide };
}

/**
 * Karar verilmiş talepler (Oğuz'un demo geri bildirimi): hangi seçenekle
 * (uygula, düzeltme, seçilen ay, reddet) ve hangi aya işlendiği görünür.
 */
export function useDecidedChangeRequests(engagement: string, enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [
      "tenant",
      slug.toLowerCase(),
      "change-requests",
      engagement,
      "decided",
    ],
    queryFn: async () => {
      const res = await tenantRequest<{
        items: ChangeRequest[];
        total: number;
      }>(slug, "GET", base(engagement), {
        query: { status: "all", limit: 50 },
      });
      return res.items.filter((c) => c.status !== "open");
    },
    enabled,
  });
}
