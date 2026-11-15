import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";
import { taxpayerRequest } from "@/features/taxpayer/services/taxpayer.service";

export type TaxpayerRequest = components["schemas"]["TaxpayerRequest"];
export type TaxpayerRequestPage = components["schemas"]["TaxpayerRequestPage"];
export type TaxpayerRequestStatus = TaxpayerRequest["status"];
export type TaxpayerKind = TaxpayerRequest["kind"];
export type CreateTaxpayerRequestBody =
  components["schemas"]["CreateTaxpayerRequestRequest"];
export type TaxpayerRequestAccepted =
  components["schemas"]["TaxpayerRequestAccepted"];
export type TaxpayerConsentPreview =
  components["schemas"]["TaxpayerConsentPreview"];
export type TaxpayerConsentOutcome =
  components["schemas"]["TaxpayerConsentOutcome"];
export type TaxpayerPendingRequest =
  components["schemas"]["TaxpayerPendingRequest"];
export type TaxpayerPendingRequestList =
  components["schemas"]["TaxpayerPendingRequestList"];

export const onboardingKeys = {
  officeRequests: (slug: string, page: { limit: number; offset: number }) =>
    ["tenant", slug.toLowerCase(), "taxpayer-requests", page] as const,
  officeRequestsAll: (slug: string) =>
    ["tenant", slug.toLowerCase(), "taxpayer-requests"] as const,
  consentPreview: (token: string) =>
    ["taxpayer-consent", "preview", token] as const,
  pendingForTaxpayer: (uuid: string) =>
    ["taxpayer", uuid.toLowerCase(), "onboarding-requests"] as const,
};

/**
 * Office side (`/v1/tenant/taxpayer-requests`). Adding a mükellef always
 * answers the same way (D1b): the UI must not branch on anything but the
 * request it sent.
 */
export const officeTaxpayerRequestService = {
  list(slug: string, page: { limit: number; offset: number }) {
    return tenantRequest<TaxpayerRequestPage>(
      slug,
      "GET",
      "/v1/tenant/taxpayer-requests",
      { query: page },
    );
  },
  create(slug: string, body: CreateTaxpayerRequestBody) {
    return tenantRequest<TaxpayerRequestAccepted>(
      slug,
      "POST",
      "/v1/tenant/taxpayer-requests",
      { body },
    );
  },
  cancel(slug: string, uuid: string) {
    return tenantRequest<unknown>(
      slug,
      "DELETE",
      `/v1/tenant/taxpayer-requests/${encodeURIComponent(uuid)}`,
    );
  },
};

/**
 * Consent link calls (`/consent/{token}`): no organization or company
 * header. The token travels in the body, never in an API URL.
 */
export const taxpayerConsentService = {
  preview: (token: string) =>
    taxpayerRequest<TaxpayerConsentPreview>(
      "POST",
      "/v1/taxpayer-consents/preview",
      { body: { token }, silent: true },
    ),
  approve: (token: string) =>
    taxpayerRequest<TaxpayerConsentOutcome>(
      "POST",
      "/v1/taxpayer-consents/approve",
      { body: { token } },
    ),
  reject: (token: string) =>
    taxpayerRequest<unknown>("POST", "/v1/taxpayer-consents/reject", {
      body: { token },
    }),
};

/** Mükellef panel: offices asking to add this company (owners only). */
export const taxpayerOnboardingService = {
  listPending: (taxpayer: string) =>
    taxpayerRequest<TaxpayerPendingRequestList>(
      "GET",
      "/v1/taxpayer/onboarding-requests",
      // Non-owners get 403: the card hides itself, no toast.
      { taxpayer, silent: true },
    ),
  approve: (taxpayer: string, uuid: string) =>
    taxpayerRequest<TaxpayerConsentOutcome>(
      "POST",
      `/v1/taxpayer/onboarding-requests/${encodeURIComponent(uuid)}/approve`,
      { taxpayer },
    ),
  reject: (taxpayer: string, uuid: string) =>
    taxpayerRequest<unknown>(
      "POST",
      `/v1/taxpayer/onboarding-requests/${encodeURIComponent(uuid)}/reject`,
      { taxpayer },
    ),
};
