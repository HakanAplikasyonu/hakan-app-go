import type {
  OfficeSetupInput,
  OfficeSetupState,
  OfficeSetupStepStatus,
} from "../services/office-setup.service";

/** Wizard order; the keys are the API's step names. */
export const SETUP_STEPS = [
  "office",
  "team",
  "taxpayers",
  "accounting",
  "integrations",
  "invites",
] as const;

export type SetupStepKey = (typeof SETUP_STEPS)[number];

/** Static label keys so the i18n linter can verify each one. */
export const SETUP_STEP_TITLE_KEYS: Record<SetupStepKey, string> = {
  office: "setup.step_office",
  team: "setup.step_team",
  taxpayers: "setup.step_taxpayers",
  accounting: "setup.step_accounting",
  integrations: "setup.step_integrations",
  invites: "setup.step_invites",
};

export const SETUP_STATUS_LABEL_KEYS: Record<OfficeSetupStepStatus, string> = {
  pending: "setup.status_pending",
  done: "setup.status_done",
  skipped: "setup.status_skipped",
};

export function stepStatus(
  state: OfficeSetupState | undefined,
  key: SetupStepKey,
): OfficeSetupStepStatus {
  return state?.steps[key]?.status ?? "pending";
}

/** The first pending step, or the last step when none is pending. */
export function initialStepIndex(state: OfficeSetupState | undefined) {
  const i = SETUP_STEPS.findIndex((k) => stepStatus(state, k) === "pending");
  return i === -1 ? SETUP_STEPS.length - 1 : i;
}

/** Steps that are no longer pending (done or skipped). */
export function settledCount(state: OfficeSetupState | undefined) {
  return SETUP_STEPS.filter((k) => stepStatus(state, k) !== "pending").length;
}

/**
 * PUT body from the current state plus changes. Only the fields the API
 * accepts are sent; timestamps and `completed_at` are the server's.
 */
export function setupInput(
  state: OfficeSetupState | undefined,
  change: {
    step?: { key: SetupStepKey; status: OfficeSetupStepStatus };
    accountingProgram?: OfficeSetupInput["accounting_program"];
    importUuid?: string;
  } = {},
): OfficeSetupInput {
  const steps: NonNullable<OfficeSetupInput["steps"]> = {};
  for (const key of SETUP_STEPS) {
    steps[key] = { status: stepStatus(state, key) };
  }
  if (change.step) steps[change.step.key] = { status: change.step.status };
  const accounting = change.accountingProgram ?? state?.accounting_program;
  const importUuid = change.importUuid ?? state?.import_uuid;
  return {
    steps,
    ...(accounting ? { accounting_program: accounting } : {}),
    ...(importUuid ? { import_uuid: importUuid } : {}),
  };
}
