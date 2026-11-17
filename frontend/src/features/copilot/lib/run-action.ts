import { bankPostingService } from "@/features/office/services/bank-posting.service";
import { journalService } from "@/features/office/services/journals.service";
import { reportService } from "@/features/reports/services/reports.service";
import { ApiError } from "@/lib/api";

import type { RunnableStep } from "./actions";

/** Outcome of one approved step. */
export type StepOutcome =
  | { status: "done"; count: number }
  | { status: "nothing" }
  | { status: "failed"; code: string };

/** Pages of suggested bank movements to approve (the endpoint takes 500). */
async function suggestedMovements(
  slug: string,
  engagement: string,
  period: string,
) {
  const ids: string[] = [];
  for (let offset = 0; ids.length < 500; offset += 200) {
    const page = await bankPostingService.list(slug, engagement, {
      period,
      status: "suggested",
      limit: 200,
      offset,
    });
    ids.push(...page.items.map((m) => m.uuid));
    if (page.items.length < 200) break;
  }
  return ids.slice(0, 500);
}

/**
 * Runs one approved step through the existing endpoint(s) it stands for,
 * with the user's own session and permissions. Nothing here decides on its
 * own: the caller runs it only after the user approved the step.
 */
export async function runStep(
  slug: string,
  step: RunnableStep,
): Promise<StepOutcome> {
  const eng = step.engagement_uuid;
  try {
    switch (step.action) {
      case "bank_suggest": {
        const r = await bankPostingService.suggest(slug, eng, step.period);
        const n = r.suggested + r.auto_approved;
        return n > 0 ? { status: "done", count: n } : { status: "nothing" };
      }
      case "bank_approve": {
        const ids = await suggestedMovements(slug, eng, step.period);
        if (ids.length === 0) return { status: "nothing" };
        const r = await bankPostingService.approve(slug, eng, ids);
        return r.approved > 0
          ? { status: "done", count: r.approved }
          : { status: "nothing" };
      }
      case "bank_drafts": {
        const r = await bankPostingService.drafts(slug, eng, step.period);
        return r.drafts.length > 0
          ? { status: "done", count: r.drafts.length }
          : { status: "nothing" };
      }
      case "journals_approve": {
        const page = await journalService.drafts(slug, eng, step.period);
        const open = page.items.filter((d) => d.status === "draft");
        if (open.length === 0) return { status: "nothing" };
        for (const d of open) {
          await journalService.approve(slug, eng, d.uuid);
        }
        return { status: "done", count: open.length };
      }
      case "export_transfer": {
        const batch = await journalService.export(slug, eng, {
          driver: step.driver ?? "luca",
          period: step.period,
        });
        await journalService.requestTransfer(slug, eng, batch.uuid);
        return { status: "done", count: batch.entries };
      }
      case "report_send": {
        await reportService.send(slug, eng, {
          kind: "month",
          period: step.period,
        });
        return { status: "done", count: 1 };
      }
      default:
        return { status: "nothing" };
    }
  } catch (err) {
    if (err instanceof ApiError) {
      if (err.code === "NOTHING_TO_EXPORT") return { status: "nothing" };
      return { status: "failed", code: err.code };
    }
    return { status: "failed", code: "NETWORK" };
  }
}
