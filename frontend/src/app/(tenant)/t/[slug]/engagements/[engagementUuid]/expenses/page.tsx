import { EngagementExpensesPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/expenses`: masraflar (#384). */
export default async function EngagementExpensesRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementExpensesPage engagement={engagementUuid} />;
}
