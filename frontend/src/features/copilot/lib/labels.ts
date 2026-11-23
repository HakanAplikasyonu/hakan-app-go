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
  "LEDGER_NOT_BALANCE_SHEET",
]);

/**
 * Adımın adı. Geçmiş zamanlı ad ("Fişler onaylı", "Banka eşleşti") yalnız
 * adım bittiyse; değilse işin adı ("Fiş onayı"): metin durumla çelişmez.
 * Durum verilmezse (sayaçlar, süzgeç) işin adı.
 */
export function stepLabel(
  t: Translate,
  key: CopilotStepKey,
  status?: CopilotStepStatus,
) {
  return status === "done"
    ? t(`office.copilot.step.${key}`)
    : t(`office.copilot.step_open.${key}`);
}

/**
 * İşletme / serbest meslek defterinde kendi cümlesi olan adımlar: banka
 * yalnız ekstre kontrolüdür (hesap yoksa isteğe bağlı), fişler masraf ve Z
 * raporlarıdır, aktarım yoktur.
 */
const SINGLE_ENTRY_DETAIL: Partial<
  Record<CopilotStepKey, Partial<Record<CopilotStepStatus, string>>>
> = {
  bank: { na: "single_entry_bank_na", done: "single_entry_bank_done" },
  journals: {
    na: "single_entry_journals_na",
    done: "single_entry_journals_done",
  },
  transfer: { na: "single_entry_na" },
};

export function statusLabel(t: Translate, status: CopilotStepStatus) {
  return t(`office.copilot.status.${status}`);
}

/**
 * Adımın durumu bir cümlede ("2 fiş taslağı onay bekliyor"). Aynı adımın
 * farklı işleri (onay bekleyen masraf, Z raporu, eksik bordro …) kendi
 * cümlesini söyler (#780).
 */
export function stepDetail(
  t: Translate,
  key: CopilotStepKey,
  status: CopilotStepStatus,
  count: number,
  action?: CopilotAction | null,
  /** Mükellef işletme defteri / serbest meslek defteri tutuyor. */
  singleEntry = false,
) {
  const single = singleEntry ? SINGLE_ENTRY_DETAIL[key]?.[status] : undefined;
  if (single) return t(`office.copilot.detail.${single}`, { count });
  if (action && ACTION_DETAIL.has(action))
    return t(`office.copilot.detail.${action}`, { count });
  return t(`office.copilot.detail.${key}_${status}`, { count });
}

const ACTION_DETAIL = new Set<CopilotAction>([
  "open_expenses",
  "open_z_reports",
  "open_export",
  "open_payroll",
  "open_trial_balance",
  "open_bank",
]);

export function actionLabel(t: Translate, action: CopilotAction) {
  return t(`office.copilot.action.${action}`);
}

/**
 * Sıradaki iş düğmesinin etiketi (#566): fiş onayı ve aktarım burada
 * çalışmaz, kontrol ekranına götürür; düğme bunu söyler.
 */
export function nextActionLabel(t: Translate, action: CopilotAction) {
  return action === "journals_approve" || action === "export_transfer"
    ? t(`office.copilot.next.${action}`)
    : actionLabel(t, action);
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
