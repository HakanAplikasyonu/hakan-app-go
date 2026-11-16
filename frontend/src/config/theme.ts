import { brand } from "@/config/brand";

/** Açık / koyu / sistemi izle. next-themes yönetir (`<html class="dark">`). */
export type ThemeMode = "light" | "dark" | "system";
/** Marka paleti: M3 (ana, kristal mavi) ya da N3 (alternatif, yeşil). */
export type ThemePalette = "m3" | "n3";
/** docs/tasarim/tokens/tokens.css içindeki `data-theme` değerleri. */
export type ThemeName = `${ThemePalette}-${"light" | "dark"}`;

export const themeConfig = {
  /** Arayüz v1: varsayılan "sistemi izle". */
  defaultMode: "system" as ThemeMode,
  defaultPalette: "m3" as ThemePalette,
  storageKey: "app.theme",
  paletteStorageKey: "app.palette",
  brand: {
    primary: brand.colors.primary,
    primaryForeground: brand.colors.primaryForeground,
    navy: brand.colors.navy,
    muted: brand.colors.lightGray,
    accent: brand.colors.lightGray,
    radius: "10px",
  },
} as const;

export const themeNames: readonly ThemeName[] = [
  "m3-light",
  "m3-dark",
  "n3-light",
  "n3-dark",
];

export function isThemePalette(value: unknown): value is ThemePalette {
  return value === "m3" || value === "n3";
}

/** Tema adına göre tarayıcı çubuğu rengi (`--bg`). */
export function themeColor(name: ThemeName): string {
  return name.endsWith("-dark") ? brand.colors.black : brand.colors.mist;
}

/**
 * Boyamadan önce `<html data-theme>` atar (FOUC yok). next-themes kendi
 * betiğiyle `dark` sınıfını koyar; bu betik paleti ekler. Depoya erişilemezse
 * (gizli pencere) varsayılan m3 + sistem kullanılır.
 */
export const themeBootScript = `(function(){try{var d=document.documentElement,s=localStorage,m=s.getItem(${JSON.stringify(
  themeConfig.storageKey,
)})||${JSON.stringify(themeConfig.defaultMode)},p=s.getItem(${JSON.stringify(
  themeConfig.paletteStorageKey,
)});if(p!=="n3")p="m3";if(m!=="light"&&m!=="dark")m=window.matchMedia("(prefers-color-scheme: dark)").matches?"dark":"light";d.setAttribute("data-theme",p+"-"+m);}catch(e){}})();`;
