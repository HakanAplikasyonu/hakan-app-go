"use client";

import Link from "next/link";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { routes } from "@/config/routes";
import { OfficeRequestsCard } from "@/features/taxpayer-onboarding/components/office-requests-card";
import { HistorySharesCard } from "@/features/taxpayer/components/history-shares-card";
import type { TaxpayerContextResponse } from "@/features/taxpayer/services/taxpayer.service";
import { useLocale } from "@/providers/locale-provider";

/**
 * `/m/{uuid}` home. Owners also see offices asking to add the company
 * (issue #7) and the pre-engagement history consents for the current office
 * (issue #8); both cards hide themselves for everyone else.
 */
export function TaxpayerHome({
  context,
}: {
  context: TaxpayerContextResponse;
}) {
  const { t } = useLocale();
  const role =
    context.role === "owner"
      ? t("taxpayer.role_owner")
      : t("taxpayer.role_employee");
  return (
    <div className="space-y-6">
      <OfficeRequestsCard taxpayer={context.taxpayer.uuid} />
      <Card>
        <CardHeader>
          <CardTitle>{context.taxpayer.title}</CardTitle>
          <CardDescription>
            {t("taxpayer.home_description", { role })}
            {" · "}
            <span className="font-mono">
              {context.taxpayer.tax_id_type.toUpperCase()}{" "}
              {context.taxpayer.tax_id}
            </span>
          </CardDescription>
        </CardHeader>
        <CardContent className="grid gap-3 sm:grid-cols-3">
          {[
            {
              href: routes.taxpayer.invoices(context.taxpayer.uuid),
              title: t("taxpayer.nav_invoices"),
              text: t("taxpayer.home_invoices"),
            },
            {
              href: routes.taxpayer.receipts(context.taxpayer.uuid),
              title: t("taxpayer.nav_receipts"),
              text: t("taxpayer.home_receipts"),
            },
            {
              href: routes.taxpayer.uploads(context.taxpayer.uuid),
              title: t("taxpayer.nav_uploads"),
              text: t("taxpayer.home_uploads"),
            },
          ].map((item) => (
            <Link
              key={item.href}
              href={item.href}
              className="hover:bg-muted/50 rounded-md border p-3 transition-colors"
            >
              <div className="font-medium">{item.title}</div>
              <div className="text-muted-foreground text-sm">{item.text}</div>
            </Link>
          ))}
        </CardContent>
      </Card>
      <HistorySharesCard taxpayer={context.taxpayer.uuid} />
    </div>
  );
}
