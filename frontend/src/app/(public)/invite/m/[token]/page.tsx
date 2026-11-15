import type { Metadata } from "next";

import { TaxpayerInviteAcceptPage } from "@/features/taxpayer";

// The token is in this URL: never leak it to other origins via Referer.
export const metadata: Metadata = { referrer: "no-referrer" };

type PageProps = {
  params: Promise<{ token: string }>;
};

export default async function TaxpayerInvitePage({ params }: PageProps) {
  const { token } = await params;
  return <TaxpayerInviteAcceptPage token={token} />;
}
