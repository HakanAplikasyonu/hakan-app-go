"use client";

import { useQuery } from "@tanstack/react-query";
import { ChevronDown } from "lucide-react";
import Link from "next/link";
import { useRouter } from "next/navigation";
import type { ReactNode } from "react";

import { MarkaLogosu } from "@/components/hakan";
import { UserMenu } from "@/components/layout/user-menu";
import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuLabel,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { routes } from "@/config/routes";
import {
  fetchTaxpayerContext,
  taxpayerQueryKeys,
  type TaxpayerContextResponse,
} from "@/features/taxpayer/services/taxpayer.service";
import { ApiError } from "@/lib/api/errors";
import { TaxpayerNav } from "@/features/taxpayer/components/taxpayer-nav";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/**
 * `/m/{uuid}` shell for mükellef users (issue #4). The company is checked
 * per request with `X-Taxpayer`; office membership plays no part here.
 */
export function TaxpayerShell({
  taxpayer,
  children,
}: {
  taxpayer: string;
  children: (ctx: TaxpayerContextResponse) => ReactNode;
}) {
  const { t } = useLocale();
  const context = useQuery({
    queryKey: taxpayerQueryKeys.context(taxpayer),
    queryFn: () => fetchTaxpayerContext(taxpayer),
    retry: false,
    meta: { silent: true },
  });

  if (context.isLoading) {
    return (
      <div className="text-muted-foreground flex min-h-svh items-center justify-center text-sm">
        {t("common.loading")}
      </div>
    );
  }

  if (context.isError || !context.data) {
    const denied =
      context.error instanceof ApiError &&
      (context.error.isForbidden || context.error.isValidation);
    return (
      <div className="flex min-h-svh items-center justify-center p-6">
        <Card className="w-full max-w-md">
          <CardHeader>
            <CardTitle>
              {denied
                ? t("taxpayer.gate_not_member_title")
                : t("taxpayer.gate_error_title")}
            </CardTitle>
            <CardDescription>
              {denied
                ? t("taxpayer.gate_not_member_description")
                : t("taxpayer.gate_error_description")}
            </CardDescription>
          </CardHeader>
          <CardContent className="flex gap-2">
            {denied ? (
              <Button asChild variant="outline">
                <Link href={routes.taxpayer.picker}>
                  {t("taxpayer.gate_choose_other")}
                </Link>
              </Button>
            ) : (
              <Button onClick={() => void context.refetch()}>
                {t("taxpayer.gate_retry")}
              </Button>
            )}
          </CardContent>
        </Card>
      </div>
    );
  }

  return (
    <div className="bg-background min-h-svh">
      <a
        href="#taxpayer-main"
        className="bg-brand text-on-brand sr-only rounded-md px-3 py-2 text-sm focus:not-sr-only focus:fixed focus:top-2 focus:left-2 focus:z-50"
      >
        {t("taxpayer.skip_to_content")}
      </a>
      {/* Mükellef web portalı başlığı (arayüz 8/9, #331; tuval MukellefWeb). */}
      <header className="bg-surface border-line border-b">
        <div className="mx-auto flex h-[60px] max-w-[1200px] items-center gap-6 px-4 md:px-8">
          <Link
            href={routes.taxpayer.home(context.data.taxpayer.uuid)}
            className="flex min-h-11 shrink-0 items-center rounded-md"
            aria-label={t("taxpayer.brand_home")}
          >
            <MarkaLogosu varyant="tam" boyut={24} />
          </Link>
          <div className="hidden h-full min-w-0 flex-1 md:flex">
            <TaxpayerNav taxpayer={context.data.taxpayer.uuid} />
          </div>
          <div className="ml-auto flex shrink-0 items-center gap-2 md:ml-0">
            <CompanySwitcher
              current={context.data.taxpayer.uuid}
              title={context.data.taxpayer.title}
            />
            <UserMenu />
          </div>
        </div>
        <div className="border-line h-12 border-t px-2 md:hidden">
          <TaxpayerNav taxpayer={context.data.taxpayer.uuid} />
        </div>
      </header>
      <main
        id="taxpayer-main"
        className="mx-auto max-w-[1200px] px-4 py-6 md:px-8 md:py-8"
      >
        {children(context.data)}
      </main>
    </div>
  );
}

function companyInitials(title: string) {
  const parts = title
    .replace(/[^\p{L}\p{N}\s]/gu, " ")
    .trim()
    .split(/\s+/)
    .filter(Boolean);
  return (
    (parts[0]?.[0] ?? "") + (parts.length > 1 ? (parts[1]?.[0] ?? "") : "")
  ).toLocaleUpperCase("tr-TR");
}

/**
 * Şirket seçici: /me'deki mükellef üyelikleri. Tek şirketi olan kullanıcı
 * yalnız şirketin adını görür (değiştirecek bir şey yok).
 */
export function CompanySwitcher({
  current,
  title,
}: {
  current: string;
  title?: string;
}) {
  const { t } = useLocale();
  const router = useRouter();
  const { user } = useAuth();
  const companies = user?.taxpayers ?? [];
  const name = title ?? companies.find((c) => c.uuid === current)?.title ?? "";
  const face = (
    <>
      <span
        aria-hidden
        className="bg-brand text-on-brand flex size-7 shrink-0 items-center justify-center rounded-md text-[11px] font-bold"
      >
        {companyInitials(name) || "•"}
      </span>
      <span className="hidden max-w-48 truncate text-sm font-medium sm:inline">
        {name}
      </span>
    </>
  );
  if (companies.length < 2) {
    return (
      <span className="flex items-center gap-2 px-1" title={name}>
        <span className="sr-only">{t("taxpayer.switcher_label")}: </span>
        {face}
      </span>
    );
  }
  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <button
          type="button"
          className="border-line hover:bg-surface-2 focus-visible:ring-ring flex min-h-11 items-center gap-2 rounded-lg border px-2 focus-visible:ring-2 focus-visible:outline-none md:min-h-10"
          aria-label={`${t("taxpayer.switcher_label")}: ${name}`}
        >
          {face}
          <ChevronDown aria-hidden className="text-text-3 size-4" />
        </button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-64">
        <DropdownMenuLabel className="text-text-3 text-xs font-normal">
          {t("taxpayer.switcher_label")}
        </DropdownMenuLabel>
        <DropdownMenuRadioGroup value={current}>
          {companies.map((c) => (
            <DropdownMenuRadioItem
              key={c.uuid}
              value={c.uuid}
              className="min-h-10"
              onSelect={() => {
                if (c.uuid !== current)
                  router.push(routes.taxpayer.home(c.uuid));
              }}
            >
              {c.title}
            </DropdownMenuRadioItem>
          ))}
        </DropdownMenuRadioGroup>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
