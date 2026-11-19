import type { Metadata } from "next";

import { MyHelpRedirect } from "@/features/help";

export const metadata: Metadata = { title: "Yardım" };

/** `/yardim/benim`: kullanıcıyı rolünün kılavuzuna götürür (#522). */
export default function MyHelpRoute() {
  return <MyHelpRedirect />;
}
