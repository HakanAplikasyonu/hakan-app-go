"use client";

import { useMemo, type ReactNode } from "react";

import { SidebarInset, SidebarProvider } from "@/components/ui/sidebar";
import { shellNav } from "@/config/nav";
import { CopilotProvider, useOptionalCopilot } from "@/features/copilot";
import { ImpersonationBanner } from "@/features/auth/components/impersonation-banner";
import { NotificationLiveBridge } from "@/features/notifications/components/notification-live-bridge";
import {
  CommandPalette,
  CommandPaletteProvider,
} from "@/features/search-engine";
import { useIsXl } from "@/hooks/use-mobile";
import { useWebPushSync } from "@/hooks/use-web-push-sync";

import { OfficeMenu } from "./office-menu";
import { OfficeTabbar } from "./office-tabbar";
import { OfficeTopbar } from "./office-topbar";
import { useShellCounts } from "./use-shell-counts";

const noop = () => {};

/**
 * Ofis kabuğu (OfisKabugu, #326). Kırılımlar (docs/tasarim/README.md):
 * - ≥1280 px: 240 px tam kenar menü;
 * - 768–1279 px: 72 px ikon rayı;
 * - <768 px: alt sekme çubuğu, menü "Daha" ile çekmecede.
 * Menü genişliği kırılımdan gelir; kullanıcı daraltmaz.
 */
export function OfficeShell({
  slug,
  children,
}: {
  slug: string;
  children: ReactNode;
}) {
  useWebPushSync(true);
  const nav = useMemo(() => shellNav({ variant: "tenant", slug }), [slug]);
  const xl = useIsXl();
  const counts = useShellCounts();

  return (
    <CommandPaletteProvider>
      <CopilotProvider>
        <SidebarProvider
          open={xl}
          onOpenChange={noop}
          style={
            {
              "--sidebar-width": "240px",
              "--sidebar-width-icon": "72px",
            } as React.CSSProperties
          }
        >
          <OfficeMenu
            catalog={nav.catalog}
            homeHref={nav.homeHref}
            counts={counts}
          />
          <SidebarInset className="@container/content min-w-0">
            <OfficeTopbar catalog={nav.catalog} />
            <main
              id="main"
              className="flex min-w-0 flex-1 flex-col gap-4 px-4 pt-4 pb-24 md:px-6 md:pb-8"
            >
              <ImpersonationBanner />
              {children}
            </main>
          </SidebarInset>
          <OfficeTabbar counts={counts} />
          <NotificationLiveBridge />
          <OfficeCommandPalette catalogs={nav.paletteCatalogs} />
        </SidebarProvider>
      </CopilotProvider>
    </CommandPaletteProvider>
  );
}

/** ⌘K: pages, records, and "Co-pilot'a sor" for a typed command (#392). */
function OfficeCommandPalette({
  catalogs,
}: {
  catalogs: ReturnType<typeof shellNav>["paletteCatalogs"];
}) {
  const copilot = useOptionalCopilot();
  return <CommandPalette catalogs={catalogs} onAsk={copilot?.ask} />;
}
