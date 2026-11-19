import { EngagementPrivacyPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/privacy`: KVKK ilgili kişi talepleri (#518). */
export default async function EngagementPrivacyRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementPrivacyPage engagement={engagementUuid} />;
}
