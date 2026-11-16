"use client";

import Link from "next/link";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";
import { routes } from "@/config/routes";
import {
  LegalDocumentBody,
  LegalDocumentMeta,
} from "@/features/legal/components/legal-document-view";
import { useLegalDocument } from "@/features/legal/hooks/use-legal";
import { legalKindLabelKey } from "@/features/legal/lib/kinds";
import type {
  LegalDocument,
  LegalKind,
} from "@/features/legal/services/legal.service";
import { useLocale } from "@/providers/locale-provider";

type LegalDocumentDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  kind: LegalKind;
  /** Already loaded text (e.g. the version a flow will send). */
  document?: LegalDocument;
  /** Without `document`: load this version (default: current). */
  version?: number;
  /** Primary action, e.g. "Okudum, onaylıyorum" ticking the checkbox. */
  acceptLabel?: string;
  onAccept?: () => void;
};

/** A KVKK text in a scrollable dialog, with a link to the public page. */
export function LegalDocumentDialog({
  open,
  onOpenChange,
  kind,
  document,
  version,
  acceptLabel,
  onAccept,
}: LegalDocumentDialogProps) {
  const { t } = useLocale();
  const query = useLegalDocument(kind, {
    version,
    enabled: open && !document,
  });
  const doc = document ?? query.data;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="flex max-h-[90svh] max-w-2xl flex-col gap-4">
        <DialogHeader className="pe-6">
          <DialogTitle>{doc?.title ?? t(legalKindLabelKey(kind))}</DialogTitle>
          {doc ? (
            <DialogDescription asChild>
              <div>
                <LegalDocumentMeta document={doc} />
              </div>
            </DialogDescription>
          ) : (
            <DialogDescription className="sr-only">
              {t(legalKindLabelKey(kind))}
            </DialogDescription>
          )}
        </DialogHeader>

        <div className="min-h-0 flex-1 overflow-y-auto pe-1">
          {doc ? (
            <LegalDocumentBody document={doc} />
          ) : query.isError ? (
            <div className="space-y-3 text-sm">
              <p className="text-destructive">{t("legal.load_error")}</p>
              <Button
                size="sm"
                variant="outline"
                onClick={() => void query.refetch()}
              >
                {t("common.retry")}
              </Button>
            </div>
          ) : (
            <p className="text-muted-foreground text-sm">
              {t("common.loading")}
            </p>
          )}
        </div>

        <DialogFooter className="gap-2">
          <Button asChild variant="ghost" className="sm:me-auto">
            <Link
              href={routes.legal(kind, doc ? { version: doc.version } : {})}
              target="_blank"
              rel="noopener"
            >
              {t("legal.open_page")}
            </Link>
          </Button>
          <Button variant="outline" onClick={() => onOpenChange(false)}>
            {t("legal.close")}
          </Button>
          {onAccept && doc ? (
            <Button
              onClick={() => {
                onAccept();
                onOpenChange(false);
              }}
            >
              {acceptLabel ?? t("legal.read_and_accept")}
            </Button>
          ) : null}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
