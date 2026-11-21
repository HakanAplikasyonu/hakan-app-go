import { EngagementCommunicationsPage } from "@/features/communication";

type Props = { params: Promise<{ engagementUuid: string }> };

export default async function EngagementCommunicationsRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementCommunicationsPage engagementUuid={engagementUuid} />;
}
