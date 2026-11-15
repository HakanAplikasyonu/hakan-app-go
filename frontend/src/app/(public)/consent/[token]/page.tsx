import type { Metadata } from "next";

import { TaxpayerConsentPage } from "@/features/taxpayer-onboarding";

// The token is in this URL: never leak it to other origins via Referer.
export const metadata: Metadata = { referrer: "no-referrer" };

type PageProps = {
  params: Promise<{ token: string }>;
};

export default async function TaxpayerConsentRoute({ params }: PageProps) {
  const { token } = await params;
  return <TaxpayerConsentPage token={token} />;
}
