import { EngagementConnectionsPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/connections`: mükellefin dış bağlantıları (#518). */
export default async function EngagementConnectionsRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementConnectionsPage engagement={engagementUuid} />;
}
