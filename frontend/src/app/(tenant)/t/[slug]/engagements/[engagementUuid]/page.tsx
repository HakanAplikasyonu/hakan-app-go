import { EngagementOverviewPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}`: mükellef özeti ve dönem şeridi (#329). */
export default async function EngagementRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementOverviewPage engagement={engagementUuid} />;
}
