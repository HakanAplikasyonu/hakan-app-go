"use client";

import { use } from "react";

import { TaxpayerArchivePage, TaxpayerShell } from "@/features/taxpayer";

type PageProps = {
  params: Promise<{ taxpayer: string }>;
};

/** `/m/{uuid}/archive`: müşavirin paylaştığı arşiv belgeleri, salt okuma (#682). */
export default function Page({ params }: PageProps) {
  const { taxpayer } = use(params);
  return (
    <TaxpayerShell taxpayer={taxpayer}>
      {() => <TaxpayerArchivePage taxpayer={taxpayer} />}
    </TaxpayerShell>
  );
}
