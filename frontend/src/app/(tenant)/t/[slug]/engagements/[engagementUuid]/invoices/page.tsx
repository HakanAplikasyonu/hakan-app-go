import { InvoicesPage } from "@/features/office";

/**
 * `/t/{slug}/engagements/{uuid}/invoices`: Faturalar sekmesi (#468). Genel
 * fatura listesi mükellef bağlamında, mükellef sabit.
 */
export default function EngagementInvoicesRoute() {
  return <InvoicesPage />;
}
