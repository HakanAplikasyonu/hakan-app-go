"use client";

import { use } from "react";

import { TaxpayerHome, TaxpayerShell } from "@/features/taxpayer";

type PageProps = {
  params: Promise<{ taxpayer: string }>;
};

export default function TaxpayerHomePage({ params }: PageProps) {
  const { taxpayer } = use(params);
  return (
    <TaxpayerShell taxpayer={taxpayer}>
      {(context) => <TaxpayerHome context={context} />}
    </TaxpayerShell>
  );
}
