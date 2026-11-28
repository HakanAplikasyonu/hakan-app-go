"use client";

import { use } from "react";

import { routes } from "@/config/routes";
import { AccountProfilePage } from "@/features/account/components/account-profile-page";
import { TaxpayerShell } from "@/features/taxpayer";

type PageProps = {
  params: Promise<{ taxpayer: string }>;
};

/** `/m/{uuid}/profil`: profil mükellef kabuğunun içinde (#890 C). */
export default function Page({ params }: PageProps) {
  const { taxpayer } = use(params);
  return (
    <TaxpayerShell taxpayer={taxpayer}>
      {() => <AccountProfilePage home={routes.taxpayer.home(taxpayer)} />}
    </TaxpayerShell>
  );
}
