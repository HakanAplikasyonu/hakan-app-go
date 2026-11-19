"use client";

import Link from "next/link";
import { useRouter, useSearchParams } from "next/navigation";
import { useEffect, useRef, useState } from "react";

import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { isApiError } from "@/lib/api";
import { tenantRequest } from "@/lib/api/tenant-request";
import { useLocale } from "@/providers/locale-provider";

import {
  parasutBase,
  takeParasutReturn,
} from "../hooks/use-accounting-connections";

/**
 * Paraşüt onay sayfasından dönüş (`?code=…&state=<bağlantı>`): kodu
 * bağlantıya iletir, sonra mükellefin Bağlantılar sekmesine döner. Hangi
 * ofis ve mükellef olduğu, yönlendirmeden önce tarayıcı oturumuna yazıldı.
 */
export function ParasutCallbackPage() {
  const { t } = useLocale();
  const router = useRouter();
  const params = useSearchParams();
  const [error, setError] = useState<string | null>(null);
  const started = useRef(false);

  useEffect(() => {
    if (started.current) return;
    started.current = true;
    // Work after a microtask: no state update inside the effect body.
    void (async () => {
      await Promise.resolve();
      const code = params.get("code");
      const back = takeParasutReturn(params.get("state"));
      if (!back) {
        setError(t("sources.accounting.callback_lost"));
        return;
      }
      const tab = routes.tenant.engagementConnections(
        back.slug,
        back.engagement,
      );
      if (!code) {
        // Kullanıcı Paraşüt'te vazgeçti ya da Paraşüt hata döndü.
        router.replace(tab);
        return;
      }
      try {
        await tenantRequest(
          back.slug,
          "POST",
          `${parasutBase(back.engagement)}/${encodeURIComponent(back.connection)}/authorize`,
          { body: { code }, silent: true },
        );
        router.replace(tab);
      } catch (err) {
        setError(
          isApiError(err)
            ? err.message
            : t("sources.accounting.callback_failed"),
        );
      }
    })();
  }, [params, router, t]);

  return (
    <main className="mx-auto flex min-h-[60vh] max-w-md flex-col items-center justify-center gap-4 px-4 text-center">
      {error ? (
        <>
          <p role="alert" className="text-danger text-sm">
            {error}
          </p>
          <Button asChild variant="outline">
            <Link href={routes.tenant.picker}>
              {t("sources.accounting.callback_home")}
            </Link>
          </Button>
        </>
      ) : (
        <p className="text-text-2 text-sm" aria-live="polite">
          {t("sources.accounting.callback_working")}
        </p>
      )}
    </main>
  );
}
