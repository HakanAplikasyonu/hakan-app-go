import type { components } from "@/generated/api";
import { withLegalVersionCheck } from "@/features/legal/services/legal.service";
import { taxpayerRequest } from "@/features/taxpayer/services/taxpayer.service";

export type HistoryShare = components["schemas"]["HistoryShare"];
export type HistoryShareList = components["schemas"]["HistoryShareList"];
export type CreateHistoryShareBody =
  components["schemas"]["CreateHistoryShareRequest"];

export const historyShareKeys = {
  list: (taxpayer: string) =>
    ["taxpayer", taxpayer.toLowerCase(), "history-shares"] as const,
};

/**
 * Mükellef panel: consents letting the current office see documents from
 * before the engagement started (issue #8, D2). Owners only; anyone else
 * gets 403.
 */
export const historyShareService = {
  list: (taxpayer: string) =>
    taxpayerRequest<HistoryShareList>("GET", "/v1/taxpayer/history-shares", {
      // Non-owners are refused: the card hides itself, no toast.
      taxpayer,
      silent: true,
    }),
  /**
   * `consent_version` is the `history_share_consent` text the owner accepted
   * (issue #57); a stale one rejects with LegalVersionOutdatedError.
   */
  create: (taxpayer: string, body: CreateHistoryShareBody) =>
    withLegalVersionCheck(() =>
      taxpayerRequest<HistoryShare>("POST", "/v1/taxpayer/history-shares", {
        taxpayer,
        body,
        // Other errors are toasted by the mutation cache.
        silent: true,
      }),
    ),
  /** Also withdraws the explicit consent given for the share. */
  revoke: (taxpayer: string, uuid: string) =>
    taxpayerRequest<unknown>(
      "DELETE",
      `/v1/taxpayer/history-shares/${encodeURIComponent(uuid)}`,
      { taxpayer },
    ),
};
