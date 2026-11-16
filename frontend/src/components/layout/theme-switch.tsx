"use client";

import { Moon, Sun } from "lucide-react";
import { useTheme } from "next-themes";

import { Button } from "@/components/ui/button";
import { useLocale } from "@/providers/locale-provider";

/**
 * Üst çubukta hızlı açık/koyu geçişi. Paleti ve "sistemi izle"yi Ayarlar →
 * Görünüm yönetir (GorunumAyari); bu düğme yalnızca kipi değiştirir.
 * theme-color meta etiketi ThemeProvider'da güncellenir.
 */
export function ThemeSwitch() {
  const { setTheme, resolvedTheme } = useTheme();
  const { t } = useLocale();

  return (
    <Button
      type="button"
      variant="ghost"
      size="icon"
      className="relative scale-95 rounded-full"
      aria-label={t("common.theme")}
      onClick={() => setTheme(resolvedTheme === "dark" ? "light" : "dark")}
    >
      <Sun className="size-[1.2rem] scale-100 rotate-0 transition-all dark:scale-0 dark:-rotate-90" />
      <Moon className="absolute size-[1.2rem] scale-0 rotate-90 transition-all dark:scale-100 dark:rotate-0" />
      <span className="sr-only">{t("common.theme")}</span>
    </Button>
  );
}
