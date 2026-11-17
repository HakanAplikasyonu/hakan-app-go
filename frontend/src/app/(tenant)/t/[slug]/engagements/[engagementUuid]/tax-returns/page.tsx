import { EngagementTaxReturnsPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/tax-returns`: KDV1, MUHSGK ve Ba/Bs (#390). */
export default async function EngagementTaxReturnsRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementTaxReturnsPage engagement={engagementUuid} />;
}
