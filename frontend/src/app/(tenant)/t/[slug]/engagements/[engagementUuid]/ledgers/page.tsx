import { EngagementLedgersPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/ledgers`: mükellef defterleri (#780). */
export default async function EngagementLedgersRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementLedgersPage engagement={engagementUuid} />;
}
