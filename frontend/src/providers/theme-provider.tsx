"use client";

import { ThemeProvider as NextThemesProvider, useTheme } from "next-themes";
import {
  createContext,
  useCallback,
  useContext,
  useEffect,
  useMemo,
  useSyncExternalStore,
  type ReactNode,
} from "react";

import {
  isThemePalette,
  themeColor,
  themeConfig,
  type ThemeName,
  type ThemePalette,
} from "@/config/theme";

// next-themes injects an inline <script> to prevent theme FOUC. React 19 warns
// about script tags inside Client Components; the script still runs correctly
// during SSR. Filter this known false-positive in development only.
if (typeof window !== "undefined" && process.env.NODE_ENV === "development") {
  const orig = console.error;
  console.error = (...args: unknown[]) => {
    const message = args
      .map((arg) => {
        if (typeof arg === "string") return arg;
        if (arg instanceof Error) return arg.message;
        return "";
      })
      .join(" ");
    if (message.includes("Encountered a script tag")) return;
    orig.apply(console, args);
  };
}

const PALETTE_EVENT = "app:palette-change";

function readPalette(): ThemePalette {
  try {
    const value = window.localStorage.getItem(themeConfig.paletteStorageKey);
    return isThemePalette(value) ? value : themeConfig.defaultPalette;
  } catch {
    return themeConfig.defaultPalette;
  }
}

function subscribePalette(onChange: () => void) {
  window.addEventListener(PALETTE_EVENT, onChange);
  window.addEventListener("storage", onChange);
  return () => {
    window.removeEventListener(PALETTE_EVENT, onChange);
    window.removeEventListener("storage", onChange);
  };
}

type PaletteContextValue = {
  palette: ThemePalette;
  setPalette: (palette: ThemePalette) => void;
};

const PaletteContext = createContext<PaletteContextValue | null>(null);

/**
 * Tema = palet (m3 | n3) × kip (açık | koyu | sistemi izle). Kip next-themes
 * ile `dark` sınıfını yönetir (Tailwind `dark:`); palet ayrı saklanır. İkisi
 * birlikte `<html data-theme="m3-light|m3-dark|n3-light|n3-dark">` olur
 * (styles/tokens.css). İlk boyamayı layout'taki `themeBootScript` yapar.
 */
export function ThemeProvider({ children }: { children: ReactNode }) {
  const palette = useSyncExternalStore(
    subscribePalette,
    readPalette,
    () => themeConfig.defaultPalette,
  );
  const setPalette = useCallback((next: ThemePalette) => {
    try {
      window.localStorage.setItem(themeConfig.paletteStorageKey, next);
    } catch {
      // Depo kapalıysa (gizli pencere) seçim bu sekmede kalmaz.
    }
    window.dispatchEvent(new Event(PALETTE_EVENT));
  }, []);
  const value = useMemo(() => ({ palette, setPalette }), [palette, setPalette]);

  return (
    <NextThemesProvider
      attribute="class"
      defaultTheme={themeConfig.defaultMode}
      enableSystem
      storageKey={themeConfig.storageKey}
      disableTransitionOnChange
    >
      <PaletteContext.Provider value={value}>
        <ThemeAttributeSync palette={palette} />
        {children}
      </PaletteContext.Provider>
    </NextThemesProvider>
  );
}

function ThemeAttributeSync({ palette }: { palette: ThemePalette }) {
  const { resolvedTheme } = useTheme();
  useEffect(() => {
    if (!resolvedTheme) return;
    const name: ThemeName = `${palette}-${resolvedTheme === "dark" ? "dark" : "light"}`;
    document.documentElement.setAttribute("data-theme", name);
    document
      .querySelector("meta[name='theme-color']")
      ?.setAttribute("content", themeColor(name));
  }, [palette, resolvedTheme]);
  return null;
}

/** Ayarlar → Görünüm için palet seçimi. */
export function usePalette(): PaletteContextValue {
  const ctx = useContext(PaletteContext);
  if (!ctx) throw new Error("usePalette, ThemeProvider içinde kullanılmalı");
  return ctx;
}
