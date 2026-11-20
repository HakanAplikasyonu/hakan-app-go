import { EngagementArchivePage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/archive`: mükellefin belge arşivi (#669 I). */
export default async function EngagementArchiveRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementArchivePage engagement={engagementUuid} />;
}
