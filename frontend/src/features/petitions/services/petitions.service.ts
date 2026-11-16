import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { parseApiError } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";
import { withStepUpRetry } from "@/features/step-up-engine/lib/step-up-interceptor";

export type PetitionTemplate = components["schemas"]["PetitionTemplate"];
export type PetitionField = components["schemas"]["PetitionField"];
export type PetitionTemplateList =
  components["schemas"]["PetitionTemplateList"];
export type PetitionRequest = components["schemas"]["PetitionRequest"];
export type RenderedPetition = components["schemas"]["RenderedPetition"];
export type Petition = components["schemas"]["Petition"];
export type PetitionList = components["schemas"]["PetitionList"];
export type PetitionCategory = PetitionTemplate["category"];

export const PETITION_CATEGORIES: PetitionCategory[] = [
  "vergi_dairesi",
  "ticaret_sicil",
  "genel",
];

function petitionsPath(engagement: string) {
  return `/v1/tenant/engagements/${encodeURIComponent(engagement)}/petitions`;
}

export const petitionKeys = {
  templates: (slug: string) =>
    ["tenant", slug.toLowerCase(), "petition-templates"] as const,
  all: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "petitions",
    ] as const,
  list: (
    slug: string,
    engagement: string,
    page: { limit: number; offset: number },
  ) => [...petitionKeys.all(slug, engagement), page] as const,
};

/**
 * Template-based petitions (issue #56). The API fills the mükellef, office
 * and signer lines; the UI only sends the template's own fields.
 */
export const petitionService = {
  templates(slug: string) {
    return tenantRequest<PetitionTemplateList>(
      slug,
      "GET",
      "/v1/tenant/petition-templates",
    );
  },
  list(
    slug: string,
    engagement: string,
    page: { limit: number; offset: number },
  ) {
    return tenantRequest<PetitionList>(slug, "GET", petitionsPath(engagement), {
      query: page,
    });
  },
  /** Renders without storing anything; errors are shown inline. */
  preview(slug: string, engagement: string, body: PetitionRequest) {
    return tenantRequest<RenderedPetition>(
      slug,
      "POST",
      `${petitionsPath(engagement)}/preview`,
      { body, silent: true },
    );
  },
  create(slug: string, engagement: string, body: PetitionRequest) {
    return tenantRequest<Petition>(slug, "POST", petitionsPath(engagement), {
      body,
      silent: true,
    });
  },
  /**
   * The PDF needs the `X-Organization` header, so a plain link cannot load
   * it: fetch it as a Blob instead.
   */
  pdf(slug: string, engagement: string, uuid: string): Promise<Blob> {
    return withStepUpRetry(async () => {
      const base = apiConfig.baseUrl.replace(/\/$/, "");
      const url = `${base}${petitionsPath(engagement)}/${encodeURIComponent(uuid)}/pdf`;
      const response = await fetch(url, {
        credentials: "include",
        headers: {
          Accept: "application/pdf",
          [ORGANIZATION_HEADER]: slug,
        },
      });
      if (!response.ok) {
        const payload = await response.json().catch(() => undefined);
        throw parseApiError(response.status, payload);
      }
      return response.blob();
    });
  },
};
