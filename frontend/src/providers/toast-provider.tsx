"use client";

import { useTheme } from "next-themes";
import { useEffect, type CSSProperties, type ReactNode } from "react";
import { Toaster, toast } from "sonner";

import { ApiError, setGlobalApiErrorHandler } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

/**
 * Zengin renkli bildirimler tasarım token'larıyla (#455): sonner'ın kendi
 * renkleri (ör. hata #e60000 / #fff0f0, 4.34:1) 4.5:1 metin kontrastını
 * karşılamıyordu. tint zemin + aynı ailenin metin token'ı dört temada 4.5:1.
 */
const toastColors = {
  "--success-bg": "var(--ok-tint)",
  "--success-border": "color-mix(in srgb, var(--ok) 35%, transparent)",
  "--success-text": "var(--ok)",
  "--error-bg": "var(--danger-tint)",
  "--error-border": "color-mix(in srgb, var(--danger) 35%, transparent)",
  "--error-text": "var(--danger)",
  "--warning-bg": "var(--warn-tint)",
  "--warning-border": "color-mix(in srgb, var(--warn) 35%, transparent)",
  "--warning-text": "var(--warn)",
  "--info-bg": "var(--info-tint)",
  "--info-border": "color-mix(in srgb, var(--info) 35%, transparent)",
  "--info-text": "var(--info)",
} as CSSProperties;

export function ToastProvider({ children }: { children: ReactNode }) {
  const { theme } = useTheme();
  const { t } = useLocale();

  useEffect(() => {
    setGlobalApiErrorHandler((error: ApiError) => {
      if (error.isUnauthorized) {
        toast.error(t("auth.session_expired"));
        return;
      }
      if (error.isForbidden) {
        toast.error(t("common.error_forbidden"));
        return;
      }
      if (error.isNotFound) {
        toast.error(t("common.error_not_found"));
        return;
      }
      if (error.isValidation) {
        toast.error(error.message || t("common.error_validation"));
        return;
      }
      if (error.isServer) {
        toast.error(
          error.requestId
            ? `${t("common.error_server")} (${error.requestId})`
            : t("common.error_server"),
        );
        return;
      }
      toast.error(error.message || t("common.error_generic"));
    });

    return () => setGlobalApiErrorHandler(null);
  }, [t]);

  return (
    <>
      {children}
      <Toaster
        richColors
        position="top-right"
        theme={theme === "dark" ? "dark" : "light"}
        style={toastColors}
      />
    </>
  );
}

export const appToast = {
  success: (message: string) => toast.success(message),
  error: (message: string) => toast.error(message),
  warning: (message: string) => toast.warning(message),
  info: (message: string) => toast.info(message),
};
