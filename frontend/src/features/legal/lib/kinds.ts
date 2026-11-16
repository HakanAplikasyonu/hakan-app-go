import type {
  LegalAcceptanceContext,
  LegalKind,
} from "@/features/legal/services/legal.service";

/** Display order of the KVKK texts (public page navigation, profile). */
export const LEGAL_KINDS = [
  "privacy_notice",
  "history_share_consent",
  "data_processing_agreement",
  "retention_policy",
] as const satisfies readonly LegalKind[];

export function isLegalKind(value: string): value is LegalKind {
  return (LEGAL_KINDS as readonly string[]).includes(value);
}

/** Short, localized name of a text (the API title is the binding one). */
export const LEGAL_KIND_LABEL_KEYS: Record<LegalKind, string> = {
  privacy_notice: "legal.kind_privacy_notice",
  history_share_consent: "legal.kind_history_share_consent",
  data_processing_agreement: "legal.kind_data_processing_agreement",
  retention_policy: "legal.kind_retention_policy",
};

export function legalKindLabelKey(kind: string): string {
  return isLegalKind(kind) ? LEGAL_KIND_LABEL_KEYS[kind] : "legal.kind_other";
}

/** In which flow an acceptance was recorded. */
export const LEGAL_CONTEXT_LABEL_KEYS: Record<LegalAcceptanceContext, string> =
  {
    self: "legal.context_self",
    history_share: "legal.context_history_share",
    onboarding_consent: "legal.context_onboarding_consent",
    office_dpa: "legal.context_office_dpa",
  };

/** A positive integer `?version=` value, or undefined. */
export function parseLegalVersion(raw: string | undefined | null) {
  if (!raw || !/^\d+$/.test(raw)) return undefined;
  const n = Number(raw);
  return Number.isSafeInteger(n) && n >= 1 ? n : undefined;
}
