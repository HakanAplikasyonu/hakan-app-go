"use client";

import {
  keepPreviousData,
  useMutation,
  useQuery,
  useQueryClient,
} from "@tanstack/react-query";

import {
  whatsappKeys,
  whatsappService,
  type WhatsAppTarget,
} from "@/features/office/services/whatsapp.service";

export function useWhatsAppCaptures(
  slug: string,
  params: { target?: WhatsAppTarget; limit: number; offset: number },
) {
  return useQuery({
    queryKey: whatsappKeys.captures(slug, params),
    queryFn: () => whatsappService.captures(slug, params),
    placeholderData: keepPreviousData,
    // Uzantıdan gelen kayıtlar sayfa açıkken de görünsün.
    refetchInterval: 60_000,
  });
}

export function useWhatsAppLinks(
  slug: string,
  params: { limit: number; offset: number },
) {
  return useQuery({
    queryKey: whatsappKeys.links(slug, params),
    queryFn: () => whatsappService.links(slug, params),
    placeholderData: keepPreviousData,
  });
}

export function useCreateWhatsAppPairing(slug: string) {
  return useMutation({
    mutationFn: () => whatsappService.createPairing(slug),
  });
}

export function useDeleteWhatsAppLink(slug: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (uuid: string) => whatsappService.deleteLink(slug, uuid),
    onSuccess: () => qc.invalidateQueries({ queryKey: whatsappKeys.all(slug) }),
  });
}
