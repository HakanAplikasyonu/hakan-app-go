import type { Metadata } from "next";

import { LetterResponsePage } from "@/features/reconciliation";

// The token is in this URL: never leak it to other origins via Referer.
export const metadata: Metadata = {
  referrer: "no-referrer",
  robots: { index: false, follow: false },
};

type PageProps = { params: Promise<{ token: string }> };

/** `/mutabakat/{token}`: karşı firmanın mutabakat yanıtı (#389). */
export default async function LetterResponseRoute({ params }: PageProps) {
  const { token } = await params;
  return <LetterResponsePage token={token} />;
}
