import type {
  AccountingProgram,
  OfficeSetupState,
} from "../services/office-setup.service";

/** What the wizard hands every step. */
export type SetupStepProps = {
  state: OfficeSetupState;
  isLast: boolean;
  /** A save of the wizard state is in flight. */
  saving: boolean;
  /** Marks the step `skipped`, saves, goes to the next step. */
  onSkip: () => Promise<void>;
  /** Marks the step `done` (with the step's own fields), saves, goes on. */
  onDone: (extra?: { accountingProgram?: AccountingProgram }) => Promise<void>;
};
