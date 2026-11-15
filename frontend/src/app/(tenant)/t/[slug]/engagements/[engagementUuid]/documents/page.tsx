import { EngagementDocumentsPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

export default async function EngagementDocumentsRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementDocumentsPage engagementUuid={engagementUuid} />;
}
