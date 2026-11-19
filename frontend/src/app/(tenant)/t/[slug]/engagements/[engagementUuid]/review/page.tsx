import { EngagementReviewSettings, ReviewWorkspace } from "@/features/office";

type Props = { params: Promise<{ engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}/review`: İnceleme sekmesi (#468). */
export default async function EngagementReviewRoute({ params }: Props) {
  const { engagementUuid } = await params;
  return (
    <div className="space-y-5">
      <EngagementReviewSettings engagement={engagementUuid} />
      <ReviewWorkspace />
    </div>
  );
}
