"use client";

import { use } from "react";

import { TaxpayerReceiptsPage, TaxpayerShell } from "@/features/taxpayer";

type PageProps = {
  params: Promise<{ taxpayer: string }>;
};

export default function Page({ params }: PageProps) {
  const { taxpayer } = use(params);
  return (
    <TaxpayerShell taxpayer={taxpayer}>
      {() => <TaxpayerReceiptsPage taxpayer={taxpayer} />}
    </TaxpayerShell>
  );
}
