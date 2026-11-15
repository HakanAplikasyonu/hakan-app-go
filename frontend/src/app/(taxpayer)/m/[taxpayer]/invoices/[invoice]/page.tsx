"use client";

import { use } from "react";

import { TaxpayerInvoiceDetailPage, TaxpayerShell } from "@/features/taxpayer";

type PageProps = {
  params: Promise<{ taxpayer: string; invoice: string }>;
};

export default function Page({ params }: PageProps) {
  const { taxpayer, invoice } = use(params);
  return (
    <TaxpayerShell taxpayer={taxpayer}>
      {() => <TaxpayerInvoiceDetailPage taxpayer={taxpayer} invoice={invoice} />}
    </TaxpayerShell>
  );
}
