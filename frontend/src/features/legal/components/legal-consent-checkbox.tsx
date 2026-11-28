"use client";

import { FileText } from "lucide-react";
import { useId, useState } from "react";

import { Button } from "@/components/ui/button";
import { Checkbox } from "@/components/ui/checkbox";
import { Label } from "@/components/ui/label";
import { LegalDocumentDialog } from "@/features/legal/components/legal-document-dialog";
import { LegalDraftNotice } from "@/features/legal/components/legal-draft-notice";
import type { LegalConsentState } from "@/features/legal/hooks/use-legal-consent";
import { legalKindLabelKey } from "@/features/legal/lib/kinds";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

type LegalConsentCheckboxProps = {
  consent: LegalConsentState;
  /** e.g. "Açık rıza metnini okudum ve onaylıyorum." */
  label: string;
  /** Dialog button that ticks the box (default "Okudum, onaylıyorum"). */
  acceptLabel?: string;
  disabled?: boolean;
  className?: string;
};

/**
 * Checkbox + "read the text" link for a legal text a flow must send the
 * version of. The text opens in a dialog whose primary button ticks the
 * box. Shows the draft notice and the "text changed" message after a 409.
 */
export function LegalConsentCheckbox({
  consent,
  label,
  acceptLabel,
  disabled = false,
  className,
}: LegalConsentCheckboxProps) {
  const { t } = useLocale();
  const id = useId();
  const [open, setOpen] = useState(false);
  const { document, query } = consent;

  return (
    <div className={cn("space-y-2", className)}>
      <div className="flex items-start gap-2">
        <Checkbox
          id={id}
          className="mt-0.5"
          checked={consent.checked}
          disabled={disabled || !document}
          onCheckedChange={(value) => consent.setChecked(value === true)}
          aria-describedby={consent.outdated ? `${id}-outdated` : undefined}
        />
        <div className="min-w-0 space-y-1">
          <Label htmlFor={id} className="leading-snug font-normal">
            {label}
          </Label>
          <div className="flex flex-wrap items-center gap-x-3 gap-y-1">
            <Button
              type="button"
              variant="link"
              size="sm"
              className="text-brand-text h-auto max-w-full min-w-0 shrink p-0 text-start text-sm whitespace-normal"
              onClick={() => setOpen(true)}
              disabled={!document && !query.isError}
            >
              <FileText className="size-3.5" aria-hidden />
              {document
                ? t("legal.read_link_version", {
                    title: t(legalKindLabelKey(consent.kind)),
                    version: document.version,
                  })
                : t("legal.read_link", {
                    title: t(legalKindLabelKey(consent.kind)),
                  })}
            </Button>
            {document?.legal_review_pending ? (
              <LegalDraftNotice compact />
            ) : null}
          </div>
        </div>
      </div>

      {query.isError && !document ? (
        <div className="text-destructive flex flex-wrap items-center gap-2 text-sm">
          <span>{t("legal.load_error")}</span>
          <Button
            type="button"
            size="sm"
            variant="outline"
            onClick={() => void query.refetch()}
          >
            {t("common.retry")}
          </Button>
        </div>
      ) : null}

      {consent.outdated ? (
        <p
          id={`${id}-outdated`}
          role="alert"
          className="text-sm text-amber-700 dark:text-amber-300"
        >
          {t("legal.outdated_message")}
        </p>
      ) : null}

      <LegalDocumentDialog
        open={open}
        onOpenChange={setOpen}
        kind={consent.kind}
        document={document}
        acceptLabel={acceptLabel ?? t("legal.read_and_accept")}
        onAccept={disabled ? undefined : () => consent.setChecked(true)}
      />
    </div>
  );
}
