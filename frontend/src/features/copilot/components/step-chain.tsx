"use client";

import Link from "next/link";
import {
  BookOpenCheck,
  ClipboardCheck,
  FileText,
  Landmark,
  Mail,
  Send,
  type LucideIcon,
} from "lucide-react";

import { statusToneClass, type StatusTone } from "@/components/tables";
import {
  Tooltip,
  TooltipContent,
  TooltipTrigger,
} from "@/components/ui/tooltip";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { statusLabel, stepDetail, stepLabel } from "../lib/labels";
import type {
  CopilotStep,
  CopilotStepKey,
  CopilotStepStatus,
} from "../services/copilot.service";

export const STEP_ICON: Record<CopilotStepKey, LucideIcon> = {
  documents: FileText,
  bank: Landmark,
  review: ClipboardCheck,
  journals: BookOpenCheck,
  transfer: Send,
  report: Mail,
};

export const STATUS_TONE: Record<CopilotStepStatus, StatusTone> = {
  done: "ok",
  todo: "warn",
  waiting: "info",
  blocked: "danger",
  na: "neutral",
};

/**
 * The period close chain of one engagement as a row of small chips:
 * belgeler → banka → öneriler → fiş → aktarım → rapor. Each chip names
 * its step and state for screen readers and in its tooltip.
 */
export function StepChain({
  steps,
  next,
  compact = false,
  hrefFor,
}: {
  steps: CopilotStep[];
  next?: CopilotStepKey | null;
  compact?: boolean;
  /** Verilirse her rozet adımın ekranına bağlantı olur (#562). */
  hrefFor?: (key: CopilotStepKey) => string;
}) {
  const { t } = useLocale();
  return (
    <ol
      className="flex flex-wrap items-center gap-1"
      aria-label={t("office.copilot.chain_label")}
    >
      {steps.map((s) => {
        const Icon = STEP_ICON[s.key];
        const label = `${stepLabel(t, s.key)}: ${stepDetail(t, s.key, s.status, s.count)}`;
        const chipClass = cn(
          "inline-flex items-center gap-1 rounded-md px-1.5 py-1 text-xs font-medium",
          statusToneClass[STATUS_TONE[s.status]],
          s.status === "na" &&
            "border-border border border-dashed bg-transparent",
          next === s.key && "ring-ring ring-2 ring-offset-1",
          hrefFor && "hover:brightness-95 dark:hover:brightness-125",
        );
        const chipBody = (
          <>
            <Icon aria-hidden className="size-3.5" />
            <span className="sr-only">{label}</span>
            {compact ? null : (
              <span aria-hidden className="hidden @3xl/content:inline">
                {stepLabel(t, s.key)}
              </span>
            )}
            {s.count > 0 && s.status !== "done" ? (
              <span aria-hidden className="tabular-nums">
                {s.count}
              </span>
            ) : null}
          </>
        );
        return (
          <li key={s.key}>
            <Tooltip>
              <TooltipTrigger asChild>
                {hrefFor && s.status !== "na" ? (
                  <Link
                    href={hrefFor(s.key)}
                    className={chipClass}
                    onClick={(e) => e.stopPropagation()}
                  >
                    {chipBody}
                  </Link>
                ) : (
                  <span tabIndex={0} className={chipClass}>
                    {chipBody}
                  </span>
                )}
              </TooltipTrigger>
              <TooltipContent>
                <p className="font-medium">
                  {stepLabel(t, s.key)} · {statusLabel(t, s.status)}
                </p>
                <p>{stepDetail(t, s.key, s.status, s.count)}</p>
              </TooltipContent>
            </Tooltip>
          </li>
        );
      })}
    </ol>
  );
}
