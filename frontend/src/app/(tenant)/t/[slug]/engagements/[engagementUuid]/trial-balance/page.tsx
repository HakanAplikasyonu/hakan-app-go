import { EngagementTrialBalancePage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/trial-balance`: mükellef mizanı (#669 E). */
export default async function EngagementTrialBalanceRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementTrialBalancePage engagement={engagementUuid} />;
}
