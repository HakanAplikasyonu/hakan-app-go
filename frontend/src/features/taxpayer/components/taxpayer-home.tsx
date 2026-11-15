"use client";

import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import type { TaxpayerContextResponse } from "@/features/taxpayer/services/taxpayer.service";
import { useLocale } from "@/providers/locale-provider";

/** `/m/{uuid}` home: empty for now (issue #4 is the shell only). */
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
      <CardContent className="text-muted-foreground text-sm">
        {t("taxpayer.home_empty")}
      </CardContent>
    </Card>
  );
}
