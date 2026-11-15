import type {
  TaxpayerKind,
  TaxpayerRequestStatus,
} from "@/features/taxpayer-onboarding/services/taxpayer-onboarding.service";

export const REQUEST_STATUS_LABEL_KEYS: Record<TaxpayerRequestStatus, string> =
  {
    pending: "onboarding.status_pending",
    approved: "onboarding.status_approved",
    rejected: "onboarding.status_rejected",
    cancelled: "onboarding.status_cancelled",
    expired: "onboarding.status_expired",
  };

export const REQUEST_STATUS_TONES: Record<
  TaxpayerRequestStatus,
  "default" | "success" | "warning" | "danger"
> = {
  pending: "warning",
  approved: "success",
  rejected: "danger",
  cancelled: "default",
  expired: "default",
};

export const KIND_LABEL_KEYS: Record<TaxpayerKind, string> = {
  legal: "onboarding.kind_legal",
  individual: "onboarding.kind_individual",
};

/** Same normalization the API applies: spaces, dots and dashes are ignored. */
export function normalizeTaxId(value: string) {
  return value.replace(/[\s.-]/g, "");
}
