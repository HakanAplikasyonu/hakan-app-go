"use client";

import type { ReactNode } from "react";

import { RouteGuard } from "@/components/common/route-guard";

/**
 * Mükellef shell — `/m`, `/m/{uuid}`. Needs a session only; access to each
 * company is checked per request (`X-Taxpayer`) by `TaxpayerShell`.
 */
export default function TaxpayerGroupLayout({
  children,
}: {
  children: ReactNode;
}) {
  return <RouteGuard mode="tenant">{children}</RouteGuard>;
}
