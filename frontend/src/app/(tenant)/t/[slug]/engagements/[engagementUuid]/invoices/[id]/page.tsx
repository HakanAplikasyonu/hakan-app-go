import { InvoiceDetailPage } from "@/features/office";

type Props = { params: Promise<{ id: string }> };

/** `/t/{slug}/engagements/{uuid}/invoices/{id}`: sekme içinde fatura (#468). */
export default async function EngagementInvoiceRoute({ params }: Props) {
  const { id } = await params;
  return <InvoiceDetailPage id={id} />;
}
