"use client";

import { useSearchParams } from "next/navigation";
import { createContext, useCallback, useContext, type ReactNode } from "react";

import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";

import { carryListParams } from "./lib/engagement-nav";

export { carryListParams };

/**
 * Mükellef bağlamı (#468). Mükellef detayının düzeni (`engagements/[uuid]/
 * layout.tsx`) altındaki ekranlar bu bağlamı görür: ofis düzeyi listeler
 * (faturalar, uyarılar, …) mükellefi sabitler, seçiciyi gizler ve
 * bağlantılarını mükellef rotasından kurar. Bağlam yoksa ekran genel
 * sayfadadır ve bugünkü gibi davranır.
 */
const EngagementScopeContext = createContext<string | null>(null);

export function EngagementScopeProvider({
  engagement,
  children,
}: {
  engagement: string;
  children: ReactNode;
}) {
  return (
    <EngagementScopeContext.Provider value={engagement}>
      {children}
    </EngagementScopeContext.Provider>
  );
}

/** Mükellef detayının içindeyse mükellefin (engagement) uuid'i, değilse null. */
export function useEngagementScope(): string | null {
  return useContext(EngagementScopeContext);
}

/**
 * Fatura ayrıntısının bağlantısı: mükellef bağlamında (ve fatura o mükellefin
 * ise) sekme içindeki rota, liste durumu taşınarak; değilse genel rota.
 */
export function useInvoiceHref({
  carry = true,
}: {
  /**
   * Fatura listesinden açılınca liste durumu taşınır; başka bir ekrandan
   * (uyarı, inceleme, yükleme) açılınca o ekranın parametreleri taşınmaz.
   */
  carry?: boolean;
} = {}) {
  const scope = useEngagementScope();
  const { slug } = useTenant();
  const params = useSearchParams();
  return useCallback(
    (id: string, engagement?: string | null) =>
      scope && (!engagement || engagement === scope)
        ? `${routes.tenant.engagementInvoice(slug, scope, id)}${carry ? carryListParams(params) : ""}`
        : routes.tenant.invoice(slug, id, engagement ?? undefined),
    [carry, scope, slug, params],
  );
}
