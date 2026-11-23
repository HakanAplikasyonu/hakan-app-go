import { describe, expect, it } from "vitest";

import en from "@/locales/en/office.json";
import tr from "@/locales/tr/office.json";

import type { components } from "@/generated/api";

type Action = components["schemas"]["CopilotAction"];
type Key = components["schemas"]["CopilotStep"]["key"];
type Status = components["schemas"]["CopilotStep"]["status"];
type NoteCode = components["schemas"]["CopilotNote"]["code"];

const actions: Action[] = [
  "bank_suggest",
  "bank_approve",
  "bank_drafts",
  "journals_approve",
  "export_transfer",
  "report_send",
  "open_documents",
  "open_review",
  "open_journals",
];
const keys: Key[] = [
  "documents",
  "bank",
  "review",
  "journals",
  "transfer",
  "luca_import",
  "vat",
  "report",
];
/** Ay sonu kapanış ekranları (#780): yalnız ekran açar. */
const closeActions: Action[] = [
  "open_expenses",
  "open_z_reports",
  "open_export",
  "open_payroll",
  "open_trial_balance",
  "open_reconciliation",
  "open_vat",
  "open_bank",
];
const statuses: Status[] = ["done", "todo", "waiting", "blocked", "na"];
const notes: NoteCode[] = [
  "ENGAGEMENT_NOT_FOUND",
  "AMBIGUOUS_ENGAGEMENT",
  "ACTION_NOT_UNDERSTOOD",
  "NO_PERMISSION",
  "NOTHING_TO_DO",
  "ALREADY_DONE",
  "PREREQUISITE_ADDED",
  "TOO_MANY_STEPS",
  "AI_UNAVAILABLE",
];

/** Every key the co-pilot builds at run time exists in both locales. */
describe("copilot labels", () => {
  const wanted = [
    ...actions.map((a) => `copilot.action.${a}`),
    ...actions.map((a) => `copilot.run.done_${a}`),
    ...keys.map((k) => `copilot.step.${k}`),
    // Bitmemiş adımın adı: geçmiş zamanlı ad yalnız "Tamam"da.
    ...keys.map((k) => `copilot.step_open.${k}`),
    "copilot.detail.single_entry_na",
    "copilot.detail.single_entry_bank_na",
    "copilot.detail.single_entry_bank_done",
    "copilot.detail.single_entry_journals_na",
    "copilot.detail.single_entry_journals_done",
    "copilot.error.LEDGER_NOT_BALANCE_SHEET",
    ...statuses.map((s) => `copilot.status.${s}`),
    ...keys.flatMap((k) => statuses.map((s) => `copilot.detail.${k}_${s}`)),
    ...notes.map((n) => `copilot.note.${n}`),
    ...closeActions.map((a) => `copilot.action.${a}`),
    ...closeActions
      .filter((a) => a !== "open_reconciliation" && a !== "open_vat")
      .map((a) => `copilot.detail.${a}`),
    "copilot.driver.luca",
    "copilot.driver.zirve",
    "copilot.error.generic",
    ...["proposed", "running", "done", "cancelled"].map(
      (s) => `copilot.recent.status_${s}`,
    ),
  ];
  it.each([
    ["tr", tr as Record<string, string>],
    ["en", en as Record<string, string>],
  ])("%s has every key", (_, messages) => {
    const missing = wanted.filter((k) => !messages[k]);
    expect(missing).toEqual([]);
  });
});
