import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";

export type PublicLetter = components["schemas"]["PublicReconciliationLetter"];
export type PublicResponseInput =
  components["schemas"]["ReconciliationLetterResponseInput"];

async function post<T>(path: string, body: unknown): Promise<T> {
  const root = apiConfig.baseUrl.replace(/\/$/, "");
  const response = await fetch(`${root}${path}`, {
    method: "POST",
    credentials: "omit",
    headers: { Accept: "application/json", "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const payload = await response.json().catch(() => undefined);
  return unwrap<T>({ data: payload, response }, { silent: true });
}

/**
 * Mutabakat mektubu yanıt bağlantısı (#389, `/mutabakat/{token}`): oturum
 * gerekmez. Jeton API adresinde değil, gövdede gider.
 */
export const publicLetterService = {
  preview: (token: string) =>
    post<PublicLetter>("/v1/public/reconciliation-letters/preview", { token }),
  respond: (token: string, input: PublicResponseInput) =>
    post<PublicLetter>("/v1/public/reconciliation-letters/respond", {
      token,
      ...input,
    }),
};
