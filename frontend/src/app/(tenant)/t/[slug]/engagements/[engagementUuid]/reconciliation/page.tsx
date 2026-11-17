import { EngagementReconciliationPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/reconciliation`: mutabakat panosu (#389). */
export default async function EngagementReconciliationRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementReconciliationPage engagement={engagementUuid} />;
}
