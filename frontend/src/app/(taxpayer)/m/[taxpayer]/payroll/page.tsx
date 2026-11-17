"use client";

import { use } from "react";

import { TaxpayerPayrollPage, TaxpayerShell } from "@/features/taxpayer";

type PageProps = {
  params: Promise<{ taxpayer: string }>;
};

/** `/m/{uuid}/payroll`: bordro PDF'leri, salt okuma (#382). */
export default function Page({ params }: PageProps) {
  const { taxpayer } = use(params);
  return (
    <TaxpayerShell taxpayer={taxpayer}>
      {() => <TaxpayerPayrollPage taxpayer={taxpayer} />}
    </TaxpayerShell>
  );
}
