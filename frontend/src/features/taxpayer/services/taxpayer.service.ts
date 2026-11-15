import type { components } from "@/generated/api";
import { apiConfig } from "@/config/api";
import { unwrap } from "@/lib/api";

export type TaxpayerContextResponse = components["schemas"]["TaxpayerContext"];
export type TaxpayerInvitationPreview =
  components["schemas"]["TaxpayerInvitationPreview"];
export type AcceptedTaxpayerInvitation =
  components["schemas"]["AcceptedTaxpayerInvitation"];

/** Header the Go API reads on `/v1/taxpayer/*` to pick the company. */
export const TAXPAYER_HEADER = "X-Taxpayer";

export const taxpayerQueryKeys = {
  context: (uuid: string) =>
    ["taxpayer", uuid.toLowerCase(), "context"] as const,
};

async function request<T>(
  method: string,
  path: string,
  options?: { body?: unknown; taxpayer?: string; silent?: boolean },
): Promise<T> {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const response = await fetch(`${base}${path}`, {
    method,
    credentials: "include",
    headers: {
      Accept: "application/json",
      ...(options?.taxpayer ? { [TAXPAYER_HEADER]: options.taxpayer } : {}),
      ...(options?.body ? { "Content-Type": "application/json" } : {}),
    },
    body: options?.body ? JSON.stringify(options.body) : undefined,
  });
  const payload = await response.json().catch(() => undefined);
  return unwrap<T>({ data: payload, response }, { silent: options?.silent });
}

/**
 * The company travels per request (`X-Taxpayer: <uuid>`), never in the
 * session, so two tabs on two companies stay independent. The BFF forwards
 * the header only on `/v1/taxpayer/*`.
 */
export function fetchTaxpayerContext(uuid: string) {
  return request<TaxpayerContextResponse>("GET", "/v1/taxpayer/context", {
    taxpayer: uuid,
    silent: true,
  });
}

export const taxpayerInvitationService = {
  preview: (token: string) =>
    request<TaxpayerInvitationPreview>(
      "POST",
      "/v1/taxpayer-invitations/preview",
      { body: { token }, silent: true },
    ),
  accept: (token: string) =>
    request<AcceptedTaxpayerInvitation>(
      "POST",
      "/v1/taxpayer-invitations/accept",
      { body: { token } },
    ),
};
