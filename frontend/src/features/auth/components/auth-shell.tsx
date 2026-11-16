"use client";

import type { ReactNode } from "react";
import Link from "next/link";

import { AppWordmark } from "@/components/brand";
import { MarkaLogosu } from "@/components/hakan";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

/**
 * Giriş kabuğu (#332, Giris.dc.html): ≥1024 bölünmüş; solda siyah marka
 * paneli (ürün cümlesi ve büyük M3 işareti), sağda form. Daha dar
 * ekranlarda yalnızca form ve logo.
 */
export function AuthShell({ children }: { children: ReactNode }) {
  const { t } = useLocale();
  return (
    <div className="bg-background flex min-h-svh">
      <aside className="giris-panel relative hidden flex-1 flex-col justify-between overflow-hidden bg-[#0A0A0A] p-12 text-white lg:flex">
        <MarkaLogosu boyut={26} />
        <span
          aria-hidden="true"
          className="pointer-events-none absolute -right-16 -bottom-10 opacity-95"
        >
          <MarkaLogosu varyant="isaret" boyut={394} />
        </span>
        <div className="relative flex max-w-[340px] flex-col gap-3.5">
          <h2 className="font-display text-[40px] leading-[1.05] font-bold text-white">
            {t("auth.shell_headline")}
          </h2>
          <p className="text-[15px] text-[#B4B4BE]">{t("auth.shell_body")}</p>
        </div>
      </aside>
      <main className="bg-surface relative flex w-full flex-col items-center justify-center p-6 md:p-10 lg:w-[520px] lg:flex-none lg:px-14">
        <div className="absolute top-4 right-4 flex items-center gap-1">
          <LocaleSwitch />
          <ThemeSwitch />
        </div>
        <div className="flex w-full max-w-sm flex-col gap-6">
          {/* Erişilebilir ad görünen ürün adından gelir (WCAG 2.5.3). */}
          <Link href={routes.guest.login} className="self-center lg:hidden">
            <AppWordmark />
          </Link>
          {children}
        </div>
      </main>
    </div>
  );
}
