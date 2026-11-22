"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import type { components } from "@/generated/api";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { tenantRequest } from "@/lib/api/tenant-request";

export type ForeignInvoice = components["schemas"]["ForeignInvoice"];
export type ForeignInvoiceList = components["schemas"]["ForeignInvoiceList"];
export type ForeignInvoiceInput = components["schemas"]["ForeignInvoiceInput"];
export type ForeignInvoicePatch = components["schemas"]["ForeignInvoicePatch"];
export type ForeignServiceKinds = components["schemas"]["ForeignServiceKinds"];
export type ForeignServiceKindSetting =
  components["schemas"]["ForeignServiceKindSetting"];
export type ForeignServiceKindInput =
  components["schemas"]["ForeignServiceKindInput"];
export type ForeignServiceKind = components["schemas"]["ForeignServiceKind"];
export type ForeignInvoiceTreatment =
  components["schemas"]["ForeignInvoiceTreatment"];
export type ForeignInvoiceJournal =
  components["schemas"]["ForeignInvoiceJournal"];
export type ForeignInvoicePeriodJournals =
  components["schemas"]["ForeignInvoicePeriodJournals"];
export type ForeignInvoiceAccounts =
  components["schemas"]["ForeignInvoiceAccounts"];

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/foreign-invoices`;

const rootKey = (slug: string) =>
  ["tenant", slug.toLowerCase(), "foreign-invoices"] as const;

export const foreignKindsKey = (slug: string) =>
  ["tenant", slug.toLowerCase(), "foreign-service-kinds"] as const;

/** Yabancı hizmet faturaları ve KDV2 (#780 A1): bir ayın listesi ve toplamı. */
export function useForeignInvoices(engagement: string, period: string) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: [...rootKey(slug), engagement, period],
    queryFn: () =>
      tenantRequest<ForeignInvoiceList>(slug, "GET", base(engagement), {
        query: { period },
      }),
    enabled: !!engagement && !!period,
  });
}

/** Ekle, düzenle, sil, onayla, onayı geri al; sonra liste yenilenir. */
export function useForeignInvoiceActions(engagement: string) {
  const { slug } = useTenant();
  const qc = useQueryClient();
  const refresh = () => qc.invalidateQueries({ queryKey: rootKey(slug) });
  const one = (uuid: string) =>
    `${base(engagement)}/${encodeURIComponent(uuid)}`;
  const create = useMutation({
    mutationFn: (body: ForeignInvoiceInput) =>
      tenantRequest<ForeignInvoice>(slug, "POST", base(engagement), { body }),
    onSuccess: refresh,
  });
  const update = useMutation({
    mutationFn: ({ uuid, body }: { uuid: string; body: ForeignInvoicePatch }) =>
      tenantRequest<ForeignInvoice>(slug, "PATCH", one(uuid), { body }),
    onSuccess: refresh,
  });
  const remove = useMutation({
    mutationFn: (uuid: string) =>
      tenantRequest<void>(slug, "DELETE", one(uuid)),
    onSuccess: refresh,
  });
  const confirm = useMutation({
    mutationFn: (uuid: string) =>
      tenantRequest<ForeignInvoice>(slug, "POST", `${one(uuid)}/confirm`),
    onSuccess: refresh,
  });
  const unconfirm = useMutation({
    mutationFn: (uuid: string) =>
      tenantRequest<ForeignInvoice>(slug, "POST", `${one(uuid)}/unconfirm`),
    onSuccess: refresh,
  });
  // Fiş taslağı (#780 KDV2 B): tek fatura, dönemin onaylıları, fişi kaldır.
  const createJournal = useMutation({
    mutationFn: (uuid: string) =>
      tenantRequest<ForeignInvoiceJournal>(slug, "POST", `${one(uuid)}/journal`),
    onSuccess: refresh,
  });
  const removeJournal = useMutation({
    mutationFn: (uuid: string) =>
      tenantRequest<void>(slug, "DELETE", `${one(uuid)}/journal`),
    onSuccess: refresh,
  });
  const createPeriodJournals = useMutation({
    mutationFn: (period: string) =>
      tenantRequest<ForeignInvoicePeriodJournals>(
        slug,
        "POST",
        `${base(engagement)}/journal`,
        { query: { period } },
      ),
    onSuccess: refresh,
  });
  return {
    create,
    update,
    remove,
    confirm,
    unconfirm,
    createJournal,
    removeJournal,
    createPeriodJournals,
  };
}

/** Ofis ayarı: bölüm açık mı ve işlem türü varsayılanları. */
export function useForeignServiceKinds(enabled = true) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: foreignKindsKey(slug),
    queryFn: () =>
      tenantRequest<ForeignServiceKinds>(
        slug,
        "GET",
        "/v1/tenant/foreign-service-kinds",
      ),
    enabled,
  });
}

/** Ayarlar → Yabancı hizmet faturaları: anahtar ve tür tablosu. */
export function useSaveForeignServiceKinds() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: {
      enabled: boolean;
      items?: ForeignServiceKindInput[];
      accounts?: ForeignInvoiceAccounts;
    }) =>
      tenantRequest<ForeignServiceKinds>(
        slug,
        "PUT",
        "/v1/tenant/foreign-service-kinds",
        { body },
      ),
    onSuccess: (data) => qc.setQueryData(foreignKindsKey(slug), data),
  });
}

export const foreignInvoicesPath = base;
