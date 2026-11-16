"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { useAppMutation } from "@/lib/query/mutation";

import {
  officeSetupKeys,
  officeSetupService,
  taxpayerImportService,
  type OfficeSetupInput,
  type TaxpayerImport,
} from "../services/office-setup.service";

/**
 * Wizard state. `silent`: the dashboard card hides itself on any error
 * (no toast on a page other features own).
 */
export function useOfficeSetup(
  slug: string,
  options?: { enabled?: boolean; silent?: boolean },
) {
  const silent = options?.silent ?? false;
  return useQuery({
    queryKey: officeSetupKeys.state(slug),
    queryFn: () => officeSetupService.get(slug, { silent }),
    enabled: options?.enabled ?? true,
    retry: silent ? false : undefined,
    meta: silent ? { silent: true } : undefined,
  });
}

export function useSaveOfficeSetup(slug: string) {
  const queryClient = useQueryClient();
  return useAppMutation({
    mutationFn: (body: OfficeSetupInput) => officeSetupService.put(slug, body),
    onSuccess: (state) =>
      queryClient.setQueryData(officeSetupKeys.state(slug), state),
  });
}

/** One taxpayer import with its rows (and its outcome once sent). */
export function useTaxpayerImport(slug: string, uuid: string | undefined) {
  return useQuery({
    queryKey: officeSetupKeys.taxpayerImport(slug, uuid ?? ""),
    queryFn: () => taxpayerImportService.get(slug, uuid!),
    enabled: Boolean(uuid),
    // The step renders a missing import itself.
    meta: { silent: true },
  });
}

/** Puts a fresh upload / send answer in the cache the steps read. */
export function useStoreTaxpayerImport(slug: string) {
  const queryClient = useQueryClient();
  return (batch: TaxpayerImport) =>
    queryClient.setQueryData(
      officeSetupKeys.taxpayerImport(slug, batch.uuid),
      batch,
    );
}
