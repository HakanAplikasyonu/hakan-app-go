"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import {
  petitionKeys,
  petitionService,
  type PetitionRequest,
} from "@/features/petitions/services/petitions.service";

export function usePetitionTemplates(slug: string, enabled = true) {
  return useQuery({
    queryKey: petitionKeys.templates(slug),
    queryFn: () => petitionService.templates(slug),
    staleTime: 5 * 60_000,
    enabled,
  });
}

export function usePetitions(
  slug: string,
  engagement: string,
  page: { limit: number; offset: number },
  enabled = true,
) {
  return useQuery({
    queryKey: petitionKeys.list(slug, engagement, page),
    queryFn: () => petitionService.list(slug, engagement, page),
    placeholderData: keepPreviousData,
    enabled,
  });
}

export function usePreviewPetition(slug: string, engagement: string) {
  return useMutation({
    mutationFn: (body: PetitionRequest) =>
      petitionService.preview(slug, engagement, body),
  });
}

export function useCreatePetition(slug: string, engagement: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (body: PetitionRequest) =>
      petitionService.create(slug, engagement, body),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: petitionKeys.all(slug, engagement) }),
  });
}
