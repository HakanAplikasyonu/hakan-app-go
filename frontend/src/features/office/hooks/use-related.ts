"use client";

import { useQuery } from "@tanstack/react-query";

import {
  relatedKeys,
  relatedService,
  type RelatedSourceKind,
} from "../services/related.service";

export function useRelatedRecords(
  slug: string,
  engagement: string,
  kind: RelatedSourceKind,
  uuid: string,
) {
  return useQuery({
    queryKey: relatedKeys.one(slug, engagement, kind, uuid),
    queryFn: () => relatedService.list(slug, engagement, kind, uuid),
    enabled: !!engagement && !!uuid,
  });
}
