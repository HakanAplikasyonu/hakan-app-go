"use client";

import { useRouter } from "next/navigation";
import { useEffect } from "react";

import { Skeleton } from "@/components/ui/skeleton";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

import { helpHref, helpTarget } from "../lib/context";
import { helpRoles } from "../lib/guides";

/**
 * `/yardim/benim` (#522): uygulama içindeki "Yardım" bağlantılarının hedefi.
 * Oturum bilgisi gelince kullanıcı rolünün kılavuzuna, rolüne yazılmış
 * kılavuz yoksa (ya da giriş yapılmamışsa) yardım merkezine yönlenir.
 */
export function MyHelpRedirect() {
  const { t } = useLocale();
  const router = useRouter();
  const { user, bootstrapped } = useAuth();
  useEffect(() => {
    if (!bootstrapped) return;
    router.replace(helpHref(helpTarget(helpRoles(user))));
  }, [bootstrapped, router, user]);
  return (
    <div
      className="mx-auto w-full max-w-4xl space-y-4 px-4 py-8 sm:px-6"
      aria-busy
    >
      <span className="sr-only">{t("common.loading")}</span>
      <Skeleton className="h-10 w-64" />
      <Skeleton className="h-64 w-full" />
    </div>
  );
}
