import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type PayrollEmployee = components["schemas"]["PayrollEmployee"];
export type PayrollEmployeePage = components["schemas"]["PayrollEmployeePage"];
export type PayrollEmployeeInput =
  components["schemas"]["PayrollEmployeeInput"];
export type PayrollRun = components["schemas"]["PayrollRun"];
export type PayrollRunPage = components["schemas"]["PayrollRunPage"];
export type PayrollRunDetail = components["schemas"]["PayrollRunDetail"];
export type PayrollLine = components["schemas"]["PayrollLine"];
export type PayrollImportResult = components["schemas"]["PayrollImportResult"];
export type PayrollBankMatchResult =
  components["schemas"]["PayrollBankMatchResult"];
export type SgkPremium = components["schemas"]["SgkPremium"];
export type SgkPremiumPage = components["schemas"]["SgkPremiumPage"];
export type PayrollDocument = components["schemas"]["PayrollDocument"];
export type PayrollDocumentPage = components["schemas"]["PayrollDocumentPage"];
export type JournalDraft = components["schemas"]["JournalDraft"];

const root = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}/payroll`;

export const payrollKeys = {
  all: (slug: string, engagement: string) =>
    [
      "tenant",
      slug.toLowerCase(),
      "engagements",
      engagement.toLowerCase(),
      "payroll",
    ] as const,
};

async function upload<T>(
  slug: string,
  path: string,
  form: FormData,
  query = "",
): Promise<T> {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const response = await fetch(`${base}${path}${query}`, {
    method: "POST",
    credentials: "include",
    headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
    body: form,
  });
  const payload = await response.json().catch(() => undefined);
  return unwrap<T>({ data: payload, response }, { silent: true });
}

async function download(slug: string, path: string, name: string) {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const response = await fetch(`${base}${path}`, {
    credentials: "include",
    headers: { [ORGANIZATION_HEADER]: slug },
  });
  if (!response.ok) throw new Error(`download ${response.status}`);
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
}

/** Personel, bordro ve SGK uçları (#382). */
export const payrollService = {
  employees(
    slug: string,
    engagement: string,
    q: { status?: string; q?: string; limit: number; offset: number },
  ) {
    return tenantRequest<PayrollEmployeePage>(
      slug,
      "GET",
      `${root(engagement)}/employees`,
      { query: q },
    );
  },
  createEmployee(slug: string, engagement: string, body: PayrollEmployeeInput) {
    return tenantRequest<PayrollEmployee>(
      slug,
      "POST",
      `${root(engagement)}/employees`,
      { body, silent: true },
    );
  },
  runs(slug: string, engagement: string, year?: string) {
    return tenantRequest<PayrollRunPage>(
      slug,
      "GET",
      `${root(engagement)}/runs`,
      { query: { year, limit: 200 } },
    );
  },
  run(slug: string, engagement: string, run: string) {
    return tenantRequest<PayrollRunDetail>(
      slug,
      "GET",
      `${root(engagement)}/runs/${encodeURIComponent(run)}`,
    );
  },
  importFile(
    slug: string,
    engagement: string,
    file: File,
    opts: { preview: boolean; period?: string },
  ) {
    const form = new FormData();
    form.append("file", file);
    if (opts.period) form.append("period", opts.period);
    return upload<PayrollImportResult>(
      slug,
      `${root(engagement)}/imports`,
      form,
      opts.preview ? "?preview=true" : "",
    );
  },
  journalDraft(slug: string, engagement: string, run: string) {
    return tenantRequest<JournalDraft>(
      slug,
      "POST",
      `${root(engagement)}/runs/${encodeURIComponent(run)}/journal-draft`,
      { silent: true },
    );
  },
  bankMatch(slug: string, engagement: string, period: string) {
    return tenantRequest<PayrollBankMatchResult>(
      slug,
      "POST",
      `${root(engagement)}/bank-match`,
      { body: { period } },
    );
  },
  exportRun(slug: string, engagement: string, run: PayrollRun) {
    return download(
      slug,
      `${root(engagement)}/runs/${encodeURIComponent(run.uuid)}/export`,
      `bordro-${run.period}.xlsx`,
    );
  },
  premiums(slug: string, engagement: string, year?: string) {
    return tenantRequest<SgkPremiumPage>(
      slug,
      "GET",
      `${root(engagement)}/sgk-premiums`,
      { query: { year, limit: 200 } },
    );
  },
  markPremiumPaid(slug: string, engagement: string, p: SgkPremium) {
    return tenantRequest<SgkPremium>(
      slug,
      "PATCH",
      `${root(engagement)}/sgk-premiums/${encodeURIComponent(p.uuid)}`,
      { body: { paid_amount: p.payable_amount } },
    );
  },
  documents(
    slug: string,
    engagement: string,
    q: { period?: string; limit: number; offset: number },
  ) {
    return tenantRequest<PayrollDocumentPage>(
      slug,
      "GET",
      `${root(engagement)}/documents`,
      { query: q },
    );
  },
  uploadDocument(
    slug: string,
    engagement: string,
    file: File,
    meta: { period: string; title?: string; visible: boolean },
  ) {
    const form = new FormData();
    form.append("file", file);
    form.append("period", meta.period);
    if (meta.title) form.append("title", meta.title);
    form.append("visible_to_taxpayer", meta.visible ? "true" : "false");
    return upload<PayrollDocument>(slug, `${root(engagement)}/documents`, form);
  },
  downloadDocument(slug: string, engagement: string, d: PayrollDocument) {
    return download(
      slug,
      `${root(engagement)}/documents/${encodeURIComponent(d.uuid)}/file`,
      d.file_name,
    );
  },
  deleteDocument(slug: string, engagement: string, d: PayrollDocument) {
    return tenantRequest<void>(
      slug,
      "DELETE",
      `${root(engagement)}/documents/${encodeURIComponent(d.uuid)}`,
    );
  },
};
