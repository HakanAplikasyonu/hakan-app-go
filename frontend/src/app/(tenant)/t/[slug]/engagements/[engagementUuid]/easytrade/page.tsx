import { EngagementEasytradePage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/easytrade`: Easytrade senkronu ve kaynakları (#385). */
export default async function EngagementEasytradeRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementEasytradePage engagement={engagementUuid} />;
}
