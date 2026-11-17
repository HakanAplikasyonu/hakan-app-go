import { EngagementPayrollPage } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/payroll`: personel, bordro ve SGK (#382). */
export default async function EngagementPayrollRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return <EngagementPayrollPage engagement={engagementUuid} />;
}
