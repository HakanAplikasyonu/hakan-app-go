import type { Metadata } from "next";

import { HelpPage } from "@/features/help";

export const metadata: Metadata = { title: "Yardım" };

/** Usage guide index (#274), public. */
export default function HelpIndexRoute() {
  return <HelpPage />;
}
