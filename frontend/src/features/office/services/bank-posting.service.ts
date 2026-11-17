import { apiConfig } from "@/config/api";
import type { components } from "@/generated/api";
import { unwrap } from "@/lib/api";
import { ORGANIZATION_HEADER, tenantRequest } from "@/lib/api/tenant-request";

export type BankPosting = components["schemas"]["BankPosting"];
export type BankPostingSummary = components["schemas"]["BankPostingSummary"];
export type BankPostingPage =
  components["schemas"]["EnvelopeBankPostingPage"]["data"];
export type BankPostingRun =
  components["schemas"]["EnvelopeBankPostingRun"]["data"];
export type BankPostingApproval =
  components["schemas"]["EnvelopeBankPostingApproval"]["data"];
export type BankPostingDrafts =
  components["schemas"]["EnvelopeBankPostingDrafts"]["data"];
export type BankLedgerAccount = components["schemas"]["BankLedgerAccount"];
export type BankAlias = components["schemas"]["BankCounterpartyAlias"];
export type BankAliasInput =
  components["schemas"]["BankCounterpartyAliasInput"];
export type BankAliasCandidate =
  components["schemas"]["BankCounterpartyAliasCandidate"];
export type BankStatementReport = components["schemas"]["BankStatementReport"];
export type BankStatementAnalysis =
  components["schemas"]["BankStatementAnalysis"];
export type BankStatementField = components["schemas"]["BankStatementField"];
export type BankStatementTemplate =
  components["schemas"]["BankStatementTemplate"];
export type BankStatementTemplateInput =
  components["schemas"]["BankStatementTemplateInput"];
export type PostingStatus = BankPosting["status"];

const eng = (engagement: string) =>
  `/v1/tenant/engagements/${encodeURIComponent(engagement)}`;

async function upload<T>(slug: string, path: string, file: File): Promise<T> {
  const base = apiConfig.baseUrl.replace(/\/$/, "");
  const form = new FormData();
  form.append("file", file);
  const response = await fetch(`${base}${path}`, {
    method: "POST",
    credentials: "include",
    headers: { Accept: "application/json", [ORGANIZATION_HEADER]: slug },
    body: form,
  });
  const payload = await response.json().catch(() => undefined);
  // Şablon akışı hatayı kendisi gösterir (STATEMENT_LAYOUT_UNKNOWN → analiz).
  return unwrap<T>({ data: payload, response }, { silent: true });
}

/**
 * Banka hareketi → mizan hesabı eşleştirme (#383): öneriler, onay, fiş
 * taslağı, karşı taraf takma adları ve ekstre şablonları.
 */
export const bankPostingService = {
  list(
    slug: string,
    engagement: string,
    q: {
      period?: string;
      status?: PostingStatus;
      limit: number;
      offset: number;
    },
  ) {
    return tenantRequest<BankPostingPage>(
      slug,
      "GET",
      `${eng(engagement)}/bank-postings`,
      { query: q },
    );
  },
  suggest(slug: string, engagement: string, period: string) {
    return tenantRequest<BankPostingRun>(
      slug,
      "POST",
      `${eng(engagement)}/bank-postings/suggest`,
      { body: { period } },
    );
  },
  approve(slug: string, engagement: string, transactionUuids: string[]) {
    return tenantRequest<BankPostingApproval>(
      slug,
      "POST",
      `${eng(engagement)}/bank-postings/approve`,
      { body: { transaction_uuids: transactionUuids } },
    );
  },
  setAccount(
    slug: string,
    engagement: string,
    transaction: string,
    accountCode: string,
    accountName: string,
  ) {
    return tenantRequest<void>(
      slug,
      "PUT",
      `${eng(engagement)}/bank-postings/${encodeURIComponent(transaction)}`,
      { body: { account_code: accountCode, account_name: accountName } },
    );
  },
  drafts(slug: string, engagement: string, period?: string) {
    return tenantRequest<BankPostingDrafts>(
      slug,
      "POST",
      `${eng(engagement)}/bank-postings/drafts`,
      { body: period ? { period } : {} },
    );
  },
  ledgerAccounts(slug: string, engagement: string) {
    return tenantRequest<{ items: BankLedgerAccount[] }>(
      slug,
      "GET",
      `${eng(engagement)}/bank-ledger-accounts`,
    );
  },
  setLedgerAccount(
    slug: string,
    engagement: string,
    cashBox: string,
    accountCode: string,
  ) {
    return tenantRequest<void>(
      slug,
      "PUT",
      `${eng(engagement)}/bank-ledger-accounts/${encodeURIComponent(cashBox)}`,
      { body: { account_code: accountCode } },
    );
  },
  aliases(slug: string, engagement: string) {
    return tenantRequest<{ items: BankAlias[] }>(
      slug,
      "GET",
      `${eng(engagement)}/bank-counterparty-aliases`,
    );
  },
  aliasCandidates(slug: string, engagement: string) {
    return tenantRequest<{ items: BankAliasCandidate[] }>(
      slug,
      "GET",
      `${eng(engagement)}/bank-counterparty-aliases/candidates`,
    );
  },
  createAlias(slug: string, engagement: string, body: BankAliasInput) {
    return tenantRequest<BankAlias>(
      slug,
      "POST",
      `${eng(engagement)}/bank-counterparty-aliases`,
      { body },
    );
  },
  promoteAlias(slug: string, engagement: string, alias: string) {
    return tenantRequest<BankAlias>(
      slug,
      "POST",
      `${eng(engagement)}/bank-counterparty-aliases/${encodeURIComponent(alias)}/promote`,
    );
  },
  deleteAlias(slug: string, engagement: string, alias: string) {
    return tenantRequest<void>(
      slug,
      "DELETE",
      `${eng(engagement)}/bank-counterparty-aliases/${encodeURIComponent(alias)}`,
    );
  },
  importStatement(slug: string, engagement: string, file: File) {
    return upload<BankStatementReport>(
      slug,
      `${eng(engagement)}/bank-statements`,
      file,
    );
  },
  analyzeStatement(slug: string, file: File) {
    return upload<BankStatementAnalysis>(
      slug,
      "/v1/tenant/bank-statement-templates/analyze",
      file,
    );
  },
  saveTemplate(slug: string, body: BankStatementTemplateInput) {
    return tenantRequest<BankStatementTemplate>(
      slug,
      "POST",
      "/v1/tenant/bank-statement-templates",
      { body },
    );
  },
};
