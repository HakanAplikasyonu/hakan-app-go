"use client";

import Link from "next/link";
import { useRouter } from "next/navigation";
import { useEffect } from "react";
import { ChevronRight } from "lucide-react";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

/** `/m` — lists the caller's companies (several mükellef memberships). */
export function TaxpayerPicker() {
  const { t } = useLocale();
  const router = useRouter();
  const { user } = useAuth();
  const companies = user?.taxpayers ?? [];
  const only = companies.length === 1 ? companies[0] : undefined;
  useEffect(() => {
    if (only) router.replace(routes.taxpayer.home(only.uuid));
  }, [only, router]);
  if (only) return null;

  return (
    <div className="flex min-h-svh items-center justify-center p-6">
      <Card className="w-full max-w-lg">
        <CardHeader>
          <CardTitle>
            {companies.length
              ? t("taxpayer.picker_title")
              : t("taxpayer.picker_empty_title")}
          </CardTitle>
          <CardDescription>
            {companies.length
              ? t("taxpayer.picker_description")
              : t("taxpayer.picker_empty_description")}
          </CardDescription>
        </CardHeader>
        {companies.length ? (
          <CardContent>
            <ul className="divide-border divide-y rounded-md border">
              {companies.map((c) => (
                <li key={c.uuid}>
                  <Link
                    href={routes.taxpayer.home(c.uuid)}
                    className="hover:bg-muted flex items-center justify-between gap-3 px-4 py-3"
                  >
                    <span className="flex flex-col">
                      <span className="font-medium">{c.title}</span>
                      <span className="text-muted-foreground text-xs">
                        {c.role === "owner"
                          ? t("taxpayer.role_owner")
                          : t("taxpayer.role_employee")}
                      </span>
                    </span>
                    <ChevronRight className="text-muted-foreground size-4" />
                  </Link>
                </li>
              ))}
            </ul>
          </CardContent>
        ) : null}
      </Card>
    </div>
  );
}
