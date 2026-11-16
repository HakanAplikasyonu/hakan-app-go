import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type OfficeSetupStepState = components["schemas"]["OfficeSetupStep"];
export type OfficeSetupStepStatus = OfficeSetupStepState["status"];
/** GET / PUT answer: `steps` always holds the six steps. */
export type OfficeSetupState =
  components["schemas"]["EnvelopeOfficeSetupState"]["data"];
/** PUT body: steps left out are pending; the server sets the timestamps. */
export type OfficeSetupInput = components["schemas"]["OfficeSetupState"];
export type AccountingProgram = NonNullable<
  OfficeSetupInput["accounting_program"]
>;

export type TaxpayerImport = components["schemas"]["TaxpayerImport"];
export type TaxpayerImportRow = components["schemas"]["TaxpayerImportRow"];
export type TaxpayerImportOutcome =
  components["schemas"]["TaxpayerImportOutcome"];

export const officeSetupKeys = {
  state: (slug: string) => ["tenant", slug.toLowerCase(), "setup"] as const,
  taxpayerImport: (slug: string, uuid: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "taxpayer-imports",
      uuid.toLowerCase(),
    ] as const,
};

const IMPORTS = "/v1/tenant/taxpayer-imports";

/** Office setup wizard state (issue #273), kept apart from tenant settings. */
export const officeSetupService = {
  get(slug: string, options?: { silent?: boolean }) {
    return tenantRequest<OfficeSetupState>(slug, "GET", "/v1/tenant/setup", {
      silent: options?.silent,
    });
  },
  put(slug: string, body: OfficeSetupInput) {
    return tenantRequest<OfficeSetupState>(slug, "PUT", "/v1/tenant/setup", {
      body,
    });
  },
};

/** Bulk taxpayer list: upload, check, then invite every valid row once. */
export const taxpayerImportService = {
  /**
   * Multipart upload; the browser sets the boundary header itself. Silent:
   * the wizard shows a refused file (422) next to the file input.
   */
  async create(slug: string, file: File) {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const form = new FormData();
    form.append("file", file);
    const response = await fetch(`${base}${IMPORTS}`, {
      method: "POST",
      credentials: "include",
      headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
      body: form,
    });
    const payload = await response.json().catch(() => undefined);
    return unwrap<TaxpayerImport>(
      { data: payload, response },
      { silent: true },
    );
  },
  get(slug: string, uuid: string) {
    return tenantRequest<TaxpayerImport>(
      slug,
      "GET",
      `${IMPORTS}/${encodeURIComponent(uuid)}`,
      { silent: true },
    );
  },
  /** `409 IMPORT_ALREADY_SENT` when the import was sent before. */
  send(slug: string, uuid: string) {
    return tenantRequest<TaxpayerImport>(
      slug,
      "POST",
      `${IMPORTS}/${encodeURIComponent(uuid)}/send`,
      { silent: true },
    );
  },
};
