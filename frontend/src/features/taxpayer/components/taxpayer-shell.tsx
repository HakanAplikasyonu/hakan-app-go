"use client";

import { useQuery } from "@tanstack/react-query";
import Link from "next/link";
import { useRouter } from "next/navigation";
import type { ReactNode } from "react";

import { Button } from "@/components/ui/button";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
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
      <header className="border-b">
        <div className="mx-auto flex max-w-5xl items-center justify-between gap-4 px-4 py-3">
          <span className="font-semibold">{context.data.taxpayer.title}</span>
          <CompanySwitcher current={context.data.taxpayer.uuid} />
        </div>
        <div className="mx-auto max-w-5xl px-4 pb-2">
          <TaxpayerNav taxpayer={context.data.taxpayer.uuid} />
        </div>
      </header>
      <main className="mx-auto max-w-5xl px-4 py-6">
        {children(context.data)}
      </main>
    </div>
  );
}

/** Company selector: the caller's taxpayer memberships from /me. */
export function CompanySwitcher({ current }: { current: string }) {
  const { t } = useLocale();
  const router = useRouter();
  const { user } = useAuth();
  const companies = user?.taxpayers ?? [];
  if (companies.length < 2) return null;
  return (
    <div className="flex items-center gap-2">
      <span className="text-muted-foreground text-sm">
        {t("taxpayer.switcher_label")}
      </span>
      <Select
        value={current}
        onValueChange={(value) => {
          if (value && value !== current) {
            router.push(routes.taxpayer.home(String(value)));
          }
        }}
      >
        <SelectTrigger
          className="w-56"
          aria-label={t("taxpayer.switcher_label")}
        >
          <SelectValue />
        </SelectTrigger>
        <SelectContent>
          {companies.map((c) => (
            <SelectItem key={c.uuid} value={c.uuid}>
              {c.title}
            </SelectItem>
          ))}
        </SelectContent>
      </Select>
    </div>
  );
}
