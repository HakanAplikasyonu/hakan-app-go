"use client";

import { useQuery, useQueryClient } from "@tanstack/react-query";

import { smsKeys, smsService } from "@/features/tenant/services/sms.service";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useAppMutation } from "@/lib/query/mutation";

import {
  communicationKeys,
  communicationService,
  type BulkSendInput,
  type TaxpayerGroupInput,
} from "../services/communication.service";

export function useTaxpayerGroups() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: communicationKeys.groups(slug),
    queryFn: () => communicationService.groups(slug),
  });
}

export function useTaxpayerGroup(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: communicationKeys.group(slug, uuid ?? ""),
    queryFn: () => communicationService.group(slug, uuid ?? ""),
    enabled: Boolean(uuid),
  });
}

export function useSaveTaxpayerGroup() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return useAppMutation({
    mutationFn: ({
      uuid,
      body,
    }: {
      uuid?: string;
      body: TaxpayerGroupInput;
    }) =>
      uuid
        ? communicationService.updateGroup(slug, uuid, body)
        : communicationService.createGroup(slug, body),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: communicationKeys.groups(slug) }),
  });
}

export function useDeleteTaxpayerGroup() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return useAppMutation({
    mutationFn: (uuid: string) => communicationService.deleteGroup(slug, uuid),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: communicationKeys.groups(slug) }),
  });
}

export function useCommunicationFilters() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: communicationKeys.filters(slug),
    queryFn: () => communicationService.filters(slug),
    staleTime: 60_000,
  });
}

export function useRecentBulkSends() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: communicationKeys.bulkSends(slug),
    queryFn: () => communicationService.recent(slug),
  });
}

/** Toplu gönderimin ilerlemesi: sürerken her 1,5 saniyede bir okunur. */
export function useBulkSend(uuid: string | null) {
  const { slug } = useTenant();
  return useQuery({
    queryKey: communicationKeys.bulkSend(slug, uuid ?? ""),
    queryFn: () => communicationService.bulkSend(slug, uuid ?? ""),
    enabled: Boolean(uuid),
    refetchInterval: (q) => (q.state.data?.status === "running" ? 1500 : false),
  });
}

export function usePreviewBulkSend() {
  const { slug } = useTenant();
  return useAppMutation({
    mutationFn: (body: BulkSendInput) =>
      communicationService.preview(slug, body),
  });
}

export function useStartBulkSend() {
  const { slug } = useTenant();
  const qc = useQueryClient();
  return useAppMutation({
    mutationFn: (body: BulkSendInput) => communicationService.start(slug, body),
    onSuccess: () =>
      qc.invalidateQueries({ queryKey: communicationKeys.bulkSends(slug) }),
  });
}

/** Ofisin SMS ayarı: kodlama (sayaç) ve SMS'in açık olup olmadığı. */
export function useSmsSettings() {
  const { slug } = useTenant();
  return useQuery({
    queryKey: smsKeys.settings(slug),
    queryFn: () => smsService.settings(slug),
    staleTime: 60_000,
  });
}
