/**
 * Uygulama kimliği (D19, docs/brand). "Hakan" geçici ad; ürün adı ayrıca
 * karara bağlanacak. JS tarafı için hex karşılıklar (theme-color, ikonlar);
 * CSS tokenları theme.css'te.
 */
export const brand = {
  name: "Hakan",
  productName: "Hakan",
  tagline: "Mali müşavir ve mükellef için kontrol edilmiş muhasebe verisi",
  /** Custom URL scheme of the companion mobile app (QR sign-in fallback). */
  mobileScheme: "app",
  colors: {
    /** slate-900 — shadcn slate primary */
    primary: "#0F172A",
    primaryForeground: "#F8FAFC",
    /** Logo karosu (docs/brand/logo-mark.svg) */
    ink: "#0F172B",
    /** Marka mavisi (blue-700) */
    brand: "#1447E6",
    /** Logo onay çizgisi */
    check: "#5B8CFF",
    status: {
      approved: "#009689",
      review: "#FFB900",
      warning: "#F54900",
      blocking: "#E7000B",
    },
    /** slate-100 */
    mist: "#F1F5F9",
    white: "#FFFFFF",
    /** Compat aliases used by theme-switch / legacy tokens */
    navy: "#020617",
    lightGray: "#F1F5F9",
  },
} as const;

export type BrandTone = "auto" | "light" | "dark";
export type BrandVariant = "logo" | "wordmark" | "mark" | "icon";
