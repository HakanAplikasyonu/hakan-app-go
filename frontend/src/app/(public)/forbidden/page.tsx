"use client";

import Link from "next/link";

import { YetkiYok } from "@/components/hakan";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useLocale } from "@/providers/locale-provider";

export default function Page() {
  const { t } = useLocale();
  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <YetkiYok
        className="w-full max-w-md"
        aciklama={t("errors.forbidden_body")}
        eylem={
          <Button asChild variant="outline">
            <Link href={routes.guest.login}>{t("errors.back_to_login")}</Link>
          </Button>
        }
      />
    </div>
  );
}
