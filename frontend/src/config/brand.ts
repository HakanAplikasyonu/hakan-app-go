/**
 * Uygulama kimliği (arayüz v1, M3 "Taç kristal" logo; docs/brand). "Hakan"
 * geçici ad; işaret ad değişse de kalır. JS tarafı için hex karşılıklar
 * (theme-color, ikonlar); CSS token'ları styles/tokens.css'te.
 */
export const brand = {
  name: "Hakan",
  productName: "Hakan",
  tagline: "Mali müşavir ve mükellef için kontrol edilmiş muhasebe verisi",
  /** Custom URL scheme of the companion mobile app (QR sign-in fallback). */
  mobileScheme: "app",
  colors: {
    /** M3 kristalleri: açık (mükellef), orta (beyanname), koyu (mali müşavir). */
    crystal: { light: "#CFD8FF", mid: "#2448FF", dark: "#102290" },
    /** N3 alternatif tema kristalleri. */
    crystalN3: { light: "#D4D3D8", mid: "#01FF11", dark: "#0A0A0A" },
    /** Metin / siyah zemin (m3-light `--text`, koyu temada `--bg`). */
    primary: "#0A0A0A",
    primaryForeground: "#F3F3F5",
    /** Uygulama ikonu karosu (mükellef ve web: siyah). */
    ink: "#0A0A0A",
    /** Marka mavisi (`--brand`, M3 orta kristal). */
    brand: "#2448FF",
    /** Eski D19 onay çizgisi rengi; artık orta kristal. */
    check: "#2448FF",
    status: {
      approved: "#0E7A4E",
      review: "#9A5800",
      warning: "#9A5800",
      blocking: "#C0261C",
    },
    /** Açık tema zemini (`--bg`). */
    mist: "#F3F3F5",
    black: "#0A0A0A",
    white: "#FFFFFF",
    /** Compat aliases used by theme-switch / legacy tokens */
    navy: "#0A0A0A",
    lightGray: "#F3F3F5",
  },
} as const;

export type BrandTone = "auto" | "light" | "dark";
export type BrandVariant = "logo" | "wordmark" | "mark" | "icon";
