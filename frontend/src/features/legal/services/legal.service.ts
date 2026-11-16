import type { components } from "@/generated/api";
import { apiConfig } from "@/config/api";
import type { AppLocale } from "@/config/i18n";
import { unwrap } from "@/lib/api";
import { ApiError } from "@/lib/api/errors";
import { tenantRequest } from "@/lib/api/tenant-request";

export type LegalDocument = components["schemas"]["LegalDocument"];
export type LegalKind = LegalDocument["kind"];
export type LegalLanguage = LegalDocument["language"];
export type MyLegalAcceptance = components["schemas"]["MyLegalAcceptance"];
export type LegalAcceptanceContext = MyLegalAcceptance["context"];
export type OfficeDPAStatus = components["schemas"]["OfficeDPAStatus"];
/** Kinds a user can acknowledge for themselves (`POST /v1/me/legal-acceptances`). */
export type SelfAcceptableKind = "privacy_notice" | "retention_policy";

export const legalKeys = {
  all: ["legal"] as const,
  documents: (lang: AppLocale) => ["legal", "documents", lang] as const,
  document: (kind: LegalKind, lang: string, version?: number) =>
    ["legal", "document", kind, lang, version ?? "current"] as const,
  myAcceptances: () => ["legal", "me", "acceptances"] as const,
  officeDpa: (slug: string, lang: AppLocale) =>
    ["tenant", slug.toLowerCase(), "legal", "dpa", lang] as const,
  officeDpaAll: (slug: string) =>
    ["tenant", slug.toLowerCase(), "legal", "dpa"] as const,
};

/** Code the API answers (409) when the text changed after it was shown. */
export const LEGAL_VERSION_OUTDATED = "LEGAL_VERSION_OUTDATED";

/**
 * Raised instead of the ApiError for a stale text version. It is not an
 * ApiError on purpose: the global mutation toast skips it and the flow
 * shows its own message (refetch the text, ask again).
 */
export class LegalVersionOutdatedError extends Error {
  constructor(message?: string) {
    super(message ?? LEGAL_VERSION_OUTDATED);
    this.name = "LegalVersionOutdatedError";
  }
}

export function isLegalVersionOutdated(
  error: unknown,
): error is LegalVersionOutdatedError {
  return error instanceof LegalVersionOutdatedError;
}

/**
 * Runs a request that carries a legal text version and maps the 409
 * LEGAL_VERSION_OUTDATED answer to LegalVersionOutdatedError. Pass a
 * `silent` request: any other ApiError is rethrown and toasted by the
 * mutation cache.
 */
export async function withLegalVersionCheck<T>(
  run: () => Promise<T>,
): Promise<T> {
  try {
    return await run();
  } catch (error) {
    if (
      error instanceof ApiError &&
      error.status === 409 &&
      error.code === LEGAL_VERSION_OUTDATED
    ) {
      throw new LegalVersionOutdatedError(error.message);
    }
    throw error;
  }
}

function search(params: Record<string, string | number | undefined>) {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") q.set(k, String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : "";
}

/** Plain BFF fetch: no organization / company header, works signed out. */
async function legalRequest<T>(
  method: string,
  path: string,
  options?: { body?: unknown; silent?: boolean },
): Promise<T> {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const response = await fetch(`${base}${path}`, {
    method,
    credentials: "include",
    headers: {
      Accept: "application/json",
      ...(options?.body ? { "Content-Type": "application/json" } : {}),
    },
    body: options?.body ? JSON.stringify(options.body) : undefined,
  });
  const payload = await response.json().catch(() => undefined);
  return unwrap<T>({ data: payload, response }, { silent: options?.silent });
}

/** Public KVKK texts (`/v1/legal/documents`): no session needed. */
export const legalDocumentService = {
  list: (lang: AppLocale) =>
    legalRequest<{ items: LegalDocument[] }>(
      "GET",
      `/v1/legal/documents${search({ lang })}`,
      { silent: true },
    ),
  get: (kind: LegalKind, opts: { lang?: string; version?: number } = {}) =>
    legalRequest<LegalDocument>(
      "GET",
      `/v1/legal/documents/${encodeURIComponent(kind)}${search({
        lang: opts.lang,
        version: opts.version,
      })}`,
      // Every caller renders the failure itself (dialog, page, card).
      { silent: true },
    ),
};

/** The signed-in user's own acceptances (KVKK md. 11). */
export const myLegalService = {
  acceptances: () =>
    legalRequest<{ items: MyLegalAcceptance[] }>(
      "GET",
      "/v1/me/legal-acceptances",
      { silent: true },
    ),
  accept: (body: {
    kind: SelfAcceptableKind;
    version: number;
    language?: LegalLanguage;
  }) =>
    withLegalVersionCheck(() =>
      legalRequest<unknown>("POST", "/v1/me/legal-acceptances", {
        body,
        silent: true,
      }),
    ),
};

/** Data processing agreement of the office (`/v1/tenant/legal/dpa`). */
export const officeDpaService = {
  get: (slug: string, lang: AppLocale) =>
    tenantRequest<OfficeDPAStatus>(slug, "GET", "/v1/tenant/legal/dpa", {
      query: { lang },
      silent: true,
    }),
  accept: (slug: string, body: { version: number; language?: LegalLanguage }) =>
    withLegalVersionCheck(() =>
      tenantRequest<OfficeDPAStatus>(
        slug,
        "POST",
        "/v1/tenant/legal/dpa/accept",
        { body, silent: true },
      ),
    ),
};
