import type { ReactNode } from "react";

import { EngagementLayout } from "@/features/office";

type Props = {
  children: ReactNode;
  params: Promise<{ engagementUuid: string }>;
};

/**
 * Mükellef detayının ortak düzeni (#468): başlık, sekme çubuğu ve dönem
 * şeridi her sekmede ve sekme içi detaylarda görünür kalır.
 */
export default async function EngagementLayoutRoute({
  children,
  params,
}: Props) {
  const { engagementUuid } = await params;
  return (
    <EngagementLayout engagement={engagementUuid}>{children}</EngagementLayout>
  );
}
