import { EngagementLateDocumentsPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/late-documents`: geçmiş dönem takibi (#780 C). */
export default async function EngagementLateDocumentsRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementLateDocumentsPage engagement={engagementUuid} />;
}
