import { isWriteAction, type RunnableStep } from "./actions";
import type { CopilotWorkflowRow } from "../services/copilot.service";

/** The step that runs a row's next work, or null. */
export function nextStepOf(row: CopilotWorkflowRow) {
  if (!row.next_step) return null;
  return row.steps.find((s) => s.key === row.next_step) ?? null;
}

/** Plan steps for the next work of each row (rows whose next step only
 * opens a screen are left out). */
export function bulkSteps(
  rows: CopilotWorkflowRow[],
): Omit<RunnableStep, "index">[] {
  const out: Omit<RunnableStep, "index">[] = [];
  for (const row of rows) {
    const step = nextStepOf(row);
    if (!step?.action || !isWriteAction(step.action)) continue;
    out.push({
      action: step.action,
      engagement_uuid: row.engagement_uuid,
      taxpayer_title: row.taxpayer_title,
      period: row.period,
    });
  }
  return out;
}

/** True when a key event comes from a text field (shortcuts stay off). */
export function fromTextField(target: EventTarget | null) {
  if (!(target instanceof HTMLElement)) return false;
  const tag = target.tagName;
  return (
    tag === "INPUT" ||
    tag === "TEXTAREA" ||
    tag === "SELECT" ||
    target.isContentEditable
  );
}
