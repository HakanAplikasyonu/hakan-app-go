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

/** Fetch for taxpayer-side endpoints; `taxpayer` adds the `X-Taxpayer` header. */
export async function taxpayerRequest<T>(
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
  return taxpayerRequest<TaxpayerContextResponse>(
    "GET",
    "/v1/taxpayer/context",
    {
      taxpayer: uuid,
      silent: true,
    },
  );
}

export const taxpayerInvitationService = {
  preview: (token: string) =>
    taxpayerRequest<TaxpayerInvitationPreview>(
      "POST",
      "/v1/taxpayer-invitations/preview",
      { body: { token }, silent: true },
    ),
  accept: (token: string) =>
    taxpayerRequest<AcceptedTaxpayerInvitation>(
      "POST",
      "/v1/taxpayer-invitations/accept",
      { body: { token } },
    ),
};

// --- Mükellef web paneli (issue #39) ----------------------------------------

export type TaxpayerInvoice = components["schemas"]["TaxpayerInvoice"];
export type TaxpayerInvoicePage = components["schemas"]["TaxpayerInvoicePage"];
export type TaxpayerInvoiceDetail =
  components["schemas"]["TaxpayerInvoiceDetail"];
export type TaxpayerReceipt = components["schemas"]["TaxpayerReceipt"];
export type TaxpayerReceiptPage = components["schemas"]["TaxpayerReceiptPage"];
export type TaxpayerUploadJob = components["schemas"]["UBLUploadJob"];
export type TaxpayerUploadPage = {
  items: TaxpayerUploadJob[];
  limit: number;
  offset: number;
};

export const taxpayerPanelKeys = {
  invoices: (uuid: string, filters: Record<string, string | number>) =>
    ["taxpayer", uuid.toLowerCase(), "invoices", filters] as const,
  invoice: (uuid: string, invoice: string) =>
    ["taxpayer", uuid.toLowerCase(), "invoice", invoice] as const,
  receipts: (uuid: string, offset: number) =>
    ["taxpayer", uuid.toLowerCase(), "receipts", offset] as const,
  uploads: (uuid: string) =>
    ["taxpayer", uuid.toLowerCase(), "uploads"] as const,
};

function query(params: Record<string, string | number | undefined>) {
  const q = new URLSearchParams();
  for (const [k, v] of Object.entries(params)) {
    if (v !== undefined && v !== "") q.set(k, String(v));
  }
  const s = q.toString();
  return s ? `?${s}` : "";
}

export const taxpayerPanelService = {
  invoices: (
    taxpayer: string,
    params: {
      period?: string;
      direction?: string;
      limit?: number;
      offset?: number;
    },
  ) =>
    taxpayerRequest<TaxpayerInvoicePage>(
      "GET",
      `/v1/taxpayer/invoices${query(params)}`,
      { taxpayer },
    ),
  invoice: (taxpayer: string, invoice: string) =>
    taxpayerRequest<TaxpayerInvoiceDetail>(
      "GET",
      `/v1/taxpayer/invoices/${encodeURIComponent(invoice)}`,
      { taxpayer },
    ),
  receipts: (taxpayer: string, params: { limit?: number; offset?: number }) =>
    taxpayerRequest<TaxpayerReceiptPage>(
      "GET",
      `/v1/taxpayer/receipts${query(params)}`,
      { taxpayer },
    ),
  uploads: (taxpayer: string) =>
    taxpayerRequest<TaxpayerUploadPage>(
      "GET",
      `/v1/taxpayer/uploads${query({ limit: 50 })}`,
      { taxpayer },
    ),
  /** Multipart upload; the browser sets the boundary header itself. */
  async upload(taxpayer: string, file: File): Promise<TaxpayerUploadJob> {
    const base = apiConfig.baseUrl.replace(/\/$/, "");
    const form = new FormData();
    form.append("file", file);
    const response = await fetch(`${base}/v1/taxpayer/uploads`, {
      method: "POST",
      credentials: "include",
      headers: { Accept: "application/json", [TAXPAYER_HEADER]: taxpayer },
      body: form,
    });
    const payload = await response.json().catch(() => undefined);
    return unwrap<TaxpayerUploadJob>({ data: payload, response });
  },
};

// --- Müşavirin istekleri ve belge yükleme (arayüz 8/9, #331) -----------------

export type TaxpayerDocumentTask = components["schemas"]["DocumentTask"];
export type TaxpayerDocumentTaskPage = {
  items: TaxpayerDocumentTask[];
  total: number;
  limit: number;
  offset: number;
};
export type TaxpayerDocumentTaskStatus =
  "open" | "submitted" | "matched" | "cancelled" | "all";

export const taxpayerTaskKeys = {
  all: (uuid: string) => ["taxpayer", uuid.toLowerCase(), "tasks"] as const,
  list: (uuid: string, status: TaxpayerDocumentTaskStatus) =>
    ["taxpayer", uuid.toLowerCase(), "tasks", status] as const,
};

async function postMultipart<T>(
  taxpayer: string,
  path: string,
  field: string,
  file: File,
  headers?: Record<string, string>,
): Promise<T> {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const form = new FormData();
  form.append(field, file);
  const response = await fetch(`${base}${path}`, {
    method: "POST",
    credentials: "include",
    headers: {
      Accept: "application/json",
      [TAXPAYER_HEADER]: taxpayer,
      ...headers,
    },
    body: form,
  });
  const payload = await response.json().catch(() => undefined);
  return unwrap<T>({ data: payload, response });
}

export const taxpayerTaskService = {
  list: (taxpayer: string, status: TaxpayerDocumentTaskStatus) =>
    taxpayerRequest<TaxpayerDocumentTaskPage>(
      "GET",
      `/v1/taxpayer/document-tasks${query({ status, limit: 100 })}`,
      { taxpayer },
    ),
  /** Bir isteğe belge ekler (PDF ya da görsel). */
  upload: (taxpayer: string, task: string, file: File) =>
    postMultipart<unknown>(
      taxpayer,
      `/v1/taxpayer/document-tasks/${encodeURIComponent(task)}/uploads`,
      "file",
      file,
    ),
  /** Fiş / fatura fotoğrafı: müşavirin okuma kuyruğuna gider. */
  receipt: (taxpayer: string, file: File) =>
    postMultipart<TaxpayerReceipt>(
      taxpayer,
      "/v1/taxpayer/receipts",
      "image",
      file,
      { "Idempotency-Key": crypto.randomUUID() },
    ),
};

export type TaxpayerSupport = components["schemas"]["TaxpayerSupport"];

/** Destek kanalı (#387): şirketin ofisi ve ofisin ClickFix widget kimliği. */
export const taxpayerSupportKeys = {
  support: (uuid: string) =>
    ["taxpayer", uuid.toLowerCase(), "support"] as const,
};

export function fetchTaxpayerSupport(uuid: string) {
  return taxpayerRequest<TaxpayerSupport>("GET", "/v1/taxpayer/support", {
    taxpayer: uuid,
    silent: true,
  });
}
