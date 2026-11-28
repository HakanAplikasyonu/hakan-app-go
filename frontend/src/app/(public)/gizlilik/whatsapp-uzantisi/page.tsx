import type { Metadata } from "next";

import { WhatsAppExtensionPrivacyPage } from "@/features/legal";

export const metadata: Metadata = {
  title: "WhatsApp uzantısı gizlilik bildirimi",
};

/**
 * Public privacy notice of the WhatsApp Web extension (#780). Linked from the
 * Chrome Web Store listing, so it must open without a session (proxy.ts
 * matcher does not cover /gizlilik).
 */
export default function WhatsAppExtensionPrivacyRoute() {
  return <WhatsAppExtensionPrivacyPage />;
}
