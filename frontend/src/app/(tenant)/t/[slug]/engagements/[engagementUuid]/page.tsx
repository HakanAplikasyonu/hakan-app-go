import { redirect } from "next/navigation";

import { routes } from "@/config/routes";

type Props = { params: Promise<{ slug: string; engagementUuid: string }> };

/** `/t/{slug}/engagements/{uuid}`: the engagement opens on its documents. */
export default async function EngagementRoute({ params }: Props) {
  const { slug, engagementUuid } = await params;
  redirect(routes.tenant.engagementDocuments(slug, engagementUuid));
}
