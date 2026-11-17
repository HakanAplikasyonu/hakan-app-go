import { EngagementZReportsPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/z-reports`: Z raporları (#384). */
export default async function EngagementZReportsRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementZReportsPage engagement={engagementUuid} />;
}
