"use client";

import { useStore } from "zustand";

import { useTenant } from "@/features/tenant/providers/tenant-provider";

import { officeStore, type OfficeStore } from "../store/office-store";

/** Aktif ofisin (URL'deki slug) ofis paneli verisi. */
export function useOffice<T>(selector: (s: OfficeStore) => T): T {
  const { slug } = useTenant();
  return useStore(officeStore(slug), selector);
}

export function useOfficeActions() {
  const { slug } = useTenant();
  return officeStore(slug).getState();
}
