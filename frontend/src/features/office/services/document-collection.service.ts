import type { components } from "@/generated/api";
import { tenantRequest } from "@/lib/api/tenant-request";

export type MissingDocumentItem = components["schemas"]["MissingDocumentItem"];
export type MissingDocuments = components["schemas"]["MissingDocuments"];
export type MissingDocumentsRequestResult =
  components["schemas"]["MissingDocumentsRequestResult"];

export type MissingRequestInput = {
  period: string;
  /** Boşsa dönemin bütün eksikleri istenir. */
  items?: { kind: MissingDocumentItem["kind"]; name?: string }[];
  sms?: boolean;
  due_days?: number;
};

/**
 * Ay başı belge toplama (#780): bir mükellefin dönem eksikleri ve eksikleri
 * topluca isteme. Aynı belge için açık istek varsa ikincisi açılmaz.
 */
export const documentCollectionService = {
  missing(slug: string, engagement: string, period: string) {
    return tenantRequest<MissingDocuments>(
      slug,
      "GET",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/document-tasks/missing`,
      { query: { period }, silent: true },
    );
  },
  request(slug: string, engagement: string, body: MissingRequestInput) {
    return tenantRequest<MissingDocumentsRequestResult>(
      slug,
      "POST",
      `/v1/tenant/engagements/${encodeURIComponent(engagement)}/document-tasks/missing`,
      { body, silent: true },
    );
  },
};
