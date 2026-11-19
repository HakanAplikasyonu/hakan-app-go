import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type ExcelImport = components["schemas"]["ExcelImport"];
export type ExcelImportRow = components["schemas"]["ExcelImportRow"];
export type ExcelImportDataType = components["schemas"]["ExcelImportDataType"];
export type ExcelImportMappingInput =
  components["schemas"]["ExcelImportMappingInput"];
export type ExcelDataTypeKey = ExcelImportDataType["data_type"];
export type ExcelRowResult = ExcelImportRow["result"];

const base = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/excel-imports`;
const one = (engagement: string, uuid: string) =>
  `${base(engagement)}/${encodeURIComponent(uuid)}`;

const root = (slug: string, engagement: string) =>
  ["tenant", slug.toLowerCase(), "excel-imports", engagement] as const;

export const excelImportKeys = {
  all: root,
  list: (slug: string, engagement: string) =>
    [...root(slug, engagement), "list"] as const,
  one: (slug: string, engagement: string, uuid: string) =>
    [...root(slug, engagement), "one", uuid] as const,
  rows: (
    slug: string,
    engagement: string,
    uuid: string,
    result: string,
    offset: number,
  ) =>
    [...root(slug, engagement), "one", uuid, "rows", result, offset] as const,
  dataTypes: (slug: string) =>
    ["tenant", slug.toLowerCase(), "excel-import-data-types"] as const,
};

export type ExcelUploadInput = {
  file: File;
  dataType?: ExcelDataTypeKey;
  period?: string;
};

/** Genel Excel / CSV içe aktarma (#395; ekran #518 B1). */
export const excelImportsService = {
  dataTypes(slug: string) {
    return tenantRequest<{ items: ExcelImportDataType[] }>(
      slug,
      "GET",
      "/v1/tenant/excel-import-data-types",
    );
  },
  list(slug: string, engagement: string) {
    return tenantRequest<{ items: ExcelImport[]; total: number }>(
      slug,
      "GET",
      base(engagement),
      { query: { limit: 50 } },
    );
  },
  get(slug: string, engagement: string, uuid: string) {
    return tenantRequest<ExcelImport>(slug, "GET", one(engagement, uuid));
  },
  /** Multipart yükleme; sınır başlığını tarayıcı koyar. */
  async upload(slug: string, engagement: string, input: ExcelUploadInput) {
    const form = new FormData();
    form.append("file", input.file);
    if (input.dataType) form.append("data_type", input.dataType);
    if (input.period) form.append("period", input.period);
    const response = await fetch(
      `${apiConfig.baseUrl.replace(/\/$/, "")}${base(engagement)}`,
      {
        method: "POST",
        credentials: "include",
        headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
        body: form,
      },
    );
    const payload = await response.json().catch(() => undefined);
    return unwrap<ExcelImport>({ data: payload, response }, { silent: true });
  },
  setMapping(
    slug: string,
    engagement: string,
    uuid: string,
    input: ExcelImportMappingInput,
  ) {
    return tenantRequest<ExcelImport>(
      slug,
      "PUT",
      `${one(engagement, uuid)}/mapping`,
      { body: input, silent: true },
    );
  },
  validate(slug: string, engagement: string, uuid: string) {
    return tenantRequest<ExcelImport>(
      slug,
      "POST",
      `${one(engagement, uuid)}/validate`,
      { silent: true },
    );
  },
  rows(
    slug: string,
    engagement: string,
    uuid: string,
    q: { result?: ExcelRowResult; limit: number; offset: number },
  ) {
    return tenantRequest<{ items: ExcelImportRow[]; total: number }>(
      slug,
      "GET",
      `${one(engagement, uuid)}/rows`,
      { query: q },
    );
  },
  skipRow(
    slug: string,
    engagement: string,
    uuid: string,
    rowNo: number,
    skip: boolean,
  ) {
    return tenantRequest<ExcelImport>(
      slug,
      "PATCH",
      `${one(engagement, uuid)}/rows/${rowNo}`,
      { body: { skip }, silent: true },
    );
  },
  /**
   * Aktarımı ortak modele yazar. Anahtar aktarımın kendi kimliğidir: çift
   * tıklama ya da yeniden deneme ikinci kez yazmaz.
   */
  async commit(slug: string, engagement: string, uuid: string) {
    const response = await fetch(
      `${apiConfig.baseUrl.replace(/\/$/, "")}${one(engagement, uuid)}/commit`,
      {
        method: "POST",
        credentials: "include",
        headers: {
          Accept: "application/json",
          [ORGANIZATION_HEADER]: slug,
          "Idempotency-Key": `excel-import-${uuid}`,
        },
      },
    );
    const payload = await response.json().catch(() => undefined);
    return unwrap<ExcelImport>({ data: payload, response }, { silent: true });
  },
  rollback(slug: string, engagement: string, uuid: string) {
    return tenantRequest<ExcelImport>(
      slug,
      "POST",
      `${one(engagement, uuid)}/rollback`,
      { silent: true },
    );
  },
};
