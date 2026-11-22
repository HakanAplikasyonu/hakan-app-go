import { EngagementForeignInvoicesPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/foreign-invoices`: yabancı faturalar / KDV2 (#780 A1). */
export default async function EngagementForeignInvoicesRoute({
  params,
}: Props) {
  const { engagementUuid } = await params;
  return <EngagementForeignInvoicesPage engagement={engagementUuid} />;
}
