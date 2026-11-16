"use client";

import type { ReactNode } from "react";

import { OfficePalette } from "@/features/office/components/office-palette";
import { OfficeShell } from "@/features/office/shell";
import { TenantProvider } from "@/features/tenant/providers/tenant-provider";

/**
 * `/t/{slug}` shell: org gate + per-org permissions + the office shell
 * (#326: kenar menü / ikon rayı / alt sekme, üst çubukta dönem seçici).
 */
export function TenantShell({
  slug,
  children,
}: {
  slug: string;
  children: ReactNode;
}) {
  return (
    <TenantProvider slug={slug}>
      <OfficeShell slug={slug}>{children}</OfficeShell>
      <OfficePalette />
    </TenantProvider>
  );
}
