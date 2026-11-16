import { EngagementBankPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/bank`: banka hareketleri (#329). */
export default async function EngagementBankRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementBankPage engagement={engagementUuid} />;
}
