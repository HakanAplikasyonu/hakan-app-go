"use client";

import type { ReactNode } from "react";
import Link from "next/link";

import { AppWordmark } from "@/components/brand";
import { LocaleSwitch } from "@/components/layout/locale-switch";
import { ThemeSwitch } from "@/components/layout/theme-switch";
import { routes } from "@/config/routes";

export function AuthShell({ children }: { children: ReactNode }) {
  return (
    <div className="bg-muted relative flex min-h-svh flex-col items-center justify-center p-6 md:p-10">
      <div className="absolute top-4 right-4 flex items-center gap-1">
        <LocaleSwitch />
        <ThemeSwitch />
      </div>
      <div className="flex w-full max-w-sm flex-col gap-6">
        {/* Erişilebilir ad görünen ürün adından gelir (WCAG 2.5.3). */}
        <Link href={routes.guest.login} className="self-center">
          <AppWordmark />
        </Link>
        {children}
      </div>
    </div>
  );
}
