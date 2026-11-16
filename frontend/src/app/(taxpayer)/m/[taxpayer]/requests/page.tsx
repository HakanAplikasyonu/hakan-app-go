"use client";

import { use } from "react";

import { TaxpayerRequestsPage, TaxpayerShell } from "@/features/taxpayer";

type PageProps = {
  params: Promise<{ taxpayer: string }>;
};

export default function Page({ params }: PageProps) {
  const { taxpayer } = use(params);
  return (
    <TaxpayerShell taxpayer={taxpayer}>
      {() => <TaxpayerRequestsPage taxpayer={taxpayer} />}
    </TaxpayerShell>
  );
}
