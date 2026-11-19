import { EngagementExcelImportsPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/excel-imports`: Excel içe aktarma (#518). */
export default async function EngagementExcelImportsRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementExcelImportsPage engagement={engagementUuid} />;
}
