"use client";

import { use } from "react";

import { TaxpayerReportPage } from "@/features/reports";
import { TaxpayerShell } from "@/features/taxpayer";

type PageProps = {
  params: Promise<{ taxpayer: string }>;
};

export default function Page({ params }: PageProps) {
  const { taxpayer } = use(params);
  return (
    <TaxpayerShell taxpayer={taxpayer}>
      {() => <TaxpayerReportPage taxpayer={taxpayer} />}
    </TaxpayerShell>
  );
}
