import { EngagementReportPage } from "@/features/reports";

type Props = { params: Promise<{ engagementUuid: string }> };

export default async function EngagementReportRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementReportPage engagement={engagementUuid} />;
}
