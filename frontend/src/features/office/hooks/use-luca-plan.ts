"use client";

import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

import { accountMemoryKeys } from "../services/account-memory.service";
import {
  lucaPlanKeys,
  lucaPlanService,
  type LucaPlanFileKind,
} from "../services/luca-plan.service";

export function useCariSubaccounts(slug: string, engagement: string) {
  return useQuery({
    queryKey: lucaPlanKeys.subaccounts(slug, engagement),
    queryFn: () => lucaPlanService.subaccounts(slug, engagement),
    enabled: !!engagement,
  });
}

export function useLucaPlanFiles(slug: string, engagement: string) {
  return useQuery({
    queryKey: lucaPlanKeys.files(slug, engagement),
    queryFn: () => lucaPlanService.files(slug, engagement),
    enabled: !!engagement,
  });
}

export function useLucaPlanActions(slug: string, engagement: string) {
  const qc = useQueryClient();
  const refresh = () =>
    qc.invalidateQueries({ queryKey: accountMemoryKeys.all(slug, engagement) });
  return {
    generate: useMutation({
      mutationFn: () => lucaPlanService.generate(slug, engagement),
      onSuccess: refresh,
    }),
    download: useMutation({
      mutationFn: (kind: LucaPlanFileKind) =>
        lucaPlanService.download(slug, engagement, kind),
      onSuccess: refresh,
    }),
  };
}
