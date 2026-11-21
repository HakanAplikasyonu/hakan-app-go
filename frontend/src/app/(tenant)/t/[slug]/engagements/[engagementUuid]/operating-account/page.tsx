import { EngagementOperatingAccountPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/operating-account`: işletme hesabı (#780). */
export default async function EngagementOperatingAccountRoute({
  params,
}: Props) {
  const { engagementUuid } = await params;
  return <EngagementOperatingAccountPage engagement={engagementUuid} />;
}
