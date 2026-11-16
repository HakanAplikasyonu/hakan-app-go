"use client";

import type { ReactNode } from "react";
import Link from "next/link";

import { AppMark, AppWordmark } from "@/components/brand";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

/**
 * Giriş ve hesap sayfalarının kabuğu (arayüz 9/9, #332; tuval Giris):
 * ≥1024 px'te solda siyah marka paneli (logo, kısa anlatım, büyük işaret),
 * sağda 520 px form paneli. Daha darda yalnız form paneli, logo üstte.
 * Marka paneli her temada siyah: tasarımda sabit.
 */
export function AuthShell({ children }: { children: ReactNode }) {
  const { t } = useLocale();
  return (
    <div className="bg-surface min-h-svh lg:grid lg:grid-cols-[minmax(0,1fr)_520px]">
      <aside className="relative hidden flex-col justify-between overflow-hidden bg-[#0a0a0a] p-12 text-white lg:flex">
        <Link
          href={routes.guest.login}
          className="relative z-10 self-start rounded-md text-white focus-visible:ring-2 focus-visible:ring-white focus-visible:outline-none"
        >
          <AppWordmark />
        </Link>
        <AppMark
          aria-hidden
          className="pointer-events-none absolute -right-16 -bottom-10 h-[394px] w-[360px] opacity-95"
        />
        <div className="relative z-10 max-w-sm space-y-3.5">
          <p className="font-display text-[40px] leading-[1.05] font-bold tracking-tight text-white">
            {t("auth.shell.hero_title")}
          </p>
          <p className="text-[15px] text-[#b4b4be]">
            {t("auth.shell.hero_body")}
          </p>
        </div>
      </aside>
      <main className="relative flex min-h-svh flex-col justify-center px-6 py-16 sm:px-14">
        <div className="absolute top-4 right-4 flex items-center gap-1">
          <LocaleSwitch />
          <ThemeSwitch />
        </div>
        <div className="mx-auto flex w-full max-w-sm flex-col gap-8">
          {/* Erişilebilir ad görünen ürün adından gelir (WCAG 2.5.3). */}
          <Link href={routes.guest.login} className="self-start lg:hidden">
            <AppWordmark />
          </Link>
          {children}
        </div>
      </main>
    </div>
  );
}
