"use client";

import { use } from "react";

import { routes } from "@/config/routes";
import { AccountProfilePage } from "@/features/account/components/account-profile-page";
import { TaxpayerShell } from "@/features/taxpayer";
import { useLocale } from "@/providers/locale-provider";

type PageProps = {
  params: Promise<{ taxpayer: string }>;
};

/**
 * `/m/{uuid}/profil`: profil mükellef kabuğunun içinde (#890 C); özet
 * kartında şirket ve şirketteki rol (#890 D).
 */
export default function Page({ params }: PageProps) {
  const { taxpayer } = use(params);
  const { t } = useLocale();
  return (
    <TaxpayerShell taxpayer={taxpayer}>
      {(ctx) => (
        <AccountProfilePage
          home={routes.taxpayer.home(taxpayer)}
          scope={{
            kind: "company",
            name: ctx.taxpayer.title,
            role:
              ctx.role === "owner"
                ? t("taxpayer.role_owner")
                : t("taxpayer.role_employee"),
          }}
        />
      )}
    </TaxpayerShell>
  );
}
