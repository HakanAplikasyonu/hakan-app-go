import type {
  CopilotAction,
  CopilotNote,
  CopilotStepKey,
  CopilotStepStatus,
} from "../services/copilot.service";

type Translate = (
  key: string,
  params?: Record<string, string | number>,
) => string;

const ERROR_KEYS = new Set([
  "EXPORT_BLOCKED",
  "ALREADY_EXPORTED",
  "TRANSFER_UNSUPPORTED",
  "TRANSFER_REVERSED",
  "TRANSFER_EXISTS",
  "NO_RECIPIENT",
  "SUGGESTION_STATE",
  "FORBIDDEN",
  "NETWORK",
]);

export function stepLabel(t: Translate, key: CopilotStepKey) {
  return t(`office.copilot.step.${key}`);
}

export function statusLabel(t: Translate, status: CopilotStepStatus) {
  return t(`office.copilot.status.${status}`);
}

/** "2 fiş taslağı onay bekliyor": the step's state in a sentence. */
export function stepDetail(
  t: Translate,
  key: CopilotStepKey,
  status: CopilotStepStatus,
  count: number,
) {
  return t(`office.copilot.detail.${key}_${status}`, { count });
}

export function actionLabel(t: Translate, action: CopilotAction) {
  return t(`office.copilot.action.${action}`);
}

/** Outcome sentence of a finished step. */
export function outcomeLabel(
  t: Translate,
  action: CopilotAction,
  outcome: { status: "done"; count: number } | { status: "nothing" },
) {
  return outcome.status === "nothing"
    ? t("office.copilot.run.nothing")
    : t(`office.copilot.run.done_${action}`, { count: outcome.count });
}

export function errorLabel(t: Translate, code: string) {
  return ERROR_KEYS.has(code)
    ? t(`office.copilot.error.${code}`)
    : t("office.copilot.error.generic");
}

export function noteLabel(t: Translate, note: CopilotNote) {
  if (note.code === "ASSISTANT_MESSAGE") return note.text ?? "";
  const action = note.action
    ? actionLabel(t, note.action as CopilotAction)
    : "";
  return t(`office.copilot.note.${note.code}`, {
    action,
    taxpayer: note.taxpayer_title ?? "",
  });
}

/** "Ekim 2026" for a YYYY-MM period. */
export function periodLabel(locale: string, period: string) {
  const [y, m] = period.split("-").map(Number);
  if (!y || !m) return period;
  return new Intl.DateTimeFormat(locale === "tr" ? "tr-TR" : "en-GB", {
    month: "long",
    year: "numeric",
    timeZone: "UTC",
  }).format(new Date(Date.UTC(y, m - 1, 1)));
}
