import { EngagementPetitionsPage } from "@/features/petitions";

type Props = { params: Promise<{ engagementUuid: string }> };

export default async function EngagementPetitionsRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementPetitionsPage engagementUuid={engagementUuid} />;
}
