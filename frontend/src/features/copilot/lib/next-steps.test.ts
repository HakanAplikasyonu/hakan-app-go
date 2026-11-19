import { describe, expect, it } from "vitest";

import type { CopilotWorkflowRow } from "../services/copilot.service";
import { bulkSteps, nextStepOf } from "./next-steps";

function row(
  uuid: string,
  next: CopilotWorkflowRow["next_step"],
  action: CopilotWorkflowRow["steps"][number]["action"],
): CopilotWorkflowRow {
  return {
    engagement_uuid: uuid,
    taxpayer_uuid: uuid,
    taxpayer_title: `Mükellef ${uuid}`,
    taxpayer_tax_id: "1234567890",
    period: "2026-10",
    next_step: next,
    done_steps: 1,
    active_steps: 6,
    steps: [
      { key: "documents", status: "done", count: 0, action: null },
      {
        key: next ?? "report",
        status: next ? "todo" : "done",
        count: 2,
        action,
      },
    ],
  };
}

describe("copilot next steps", () => {
  it("finds the next step of a row", () => {
    expect(nextStepOf(row("a", "journals", "journals_approve"))?.action).toBe(
      "journals_approve",
    );
    expect(nextStepOf(row("b", null, null))).toBeNull();
  });

  it("plans only write actions in bulk", () => {
    const steps = bulkSteps([
      row("a", "journals", "journals_approve"),
      row("b", "review", "open_review"),
      row("c", null, null),
      row("d", "report", "report_send"),
    ]);
    // Taslak fiş onayı kontrol ekranına götürür, plana girmez (#566).
    expect(steps.map((s) => [s.engagement_uuid, s.action])).toEqual([
      ["d", "report_send"],
    ]);
    expect(steps[0]?.period).toBe("2026-10");
  });
});
