"use client";

import Link from "next/link";
import {
  BookOpenCheck,
  ClipboardCheck,
  FileDown,
  FileText,
  Landmark,
  Mail,
  Percent,
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
  CopilotAction,
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
  luca_import: FileDown,
  vat: Percent,
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
 * belgeler → banka → öneriler → fiş → aktarım → Luca'dan alındı → KDV
 * bildirildi → rapor (#780). Each chip names
 * its step and state for screen readers and in its tooltip.
 */
export function StepChain({
  steps,
  next,
  compact = false,
  singleEntry = false,
  hrefFor,
  onNavigate,
}: {
  steps: CopilotStep[];
  next?: CopilotStepKey | null;
  compact?: boolean;
  /** İşletme / serbest meslek defteri: banka, fiş ve aktarım adımı yok. */
  singleEntry?: boolean;
  /**
   * Verilirse her rozet adımın ekranına bağlantı olur (#562); adımın işi tek
   * kayıtsa (açık belge isteği) `target` o kaydın kimliği.
   */
  hrefFor?: (
    key: CopilotStepKey,
    action?: CopilotAction | null,
    target?: string | null,
  ) => string;
  /**
   * Rozete tıklanınca (gezinmeden önce): çağıran, zincirin sayıldığı dönemi
   * üst çubuğa sabitler ki açılan ekran aynı dönemi göstersin (#566).
   */
  onNavigate?: () => void;
}) {
  const { t } = useLocale();
  return (
    <ol
      className="flex flex-wrap items-center gap-1"
      aria-label={t("office.copilot.chain_label")}
    >
      {steps.map((s) => {
        const Icon = STEP_ICON[s.key];
        const detail = stepDetail(
          t,
          s.key,
          s.status,
          s.count,
          s.action,
          singleEntry,
        );
        const name = stepLabel(t, s.key, s.status);
        const label = `${name}: ${detail}`;
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
                {name}
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
                    href={hrefFor(s.key, s.action, s.target_uuid)}
                    className={chipClass}
                    data-step={s.key}
                    onClick={(e) => {
                      e.stopPropagation();
                      onNavigate?.();
                    }}
                  >
                    {chipBody}
                  </Link>
                ) : (
                  <span
                    tabIndex={0}
                    className={chipClass}
                    data-step={s.key}
                  >
                    {chipBody}
                  </span>
                )}
              </TooltipTrigger>
              <TooltipContent>
                <p className="font-medium">
                  {name} · {statusLabel(t, s.status)}
                </p>
                <p>{detail}</p>
              </TooltipContent>
            </Tooltip>
          </li>
        );
      })}
    </ol>
  );
}
