"use client";

import { LegalDraftNotice } from "@/features/legal/components/legal-draft-notice";
import { LegalMarkdown } from "@/features/legal/components/legal-markdown";
import type { LegalDocument } from "@/features/legal/services/legal.service";
import { date } from "@/lib/utils/format";
import { useLocale } from "@/providers/locale-provider";

/** "Sürüm 2 · Yayım: 01.11.2026" (+ status / language notes). */
export function LegalDocumentMeta({ document }: { document: LegalDocument }) {
  const { t, locale } = useLocale();
  const parts = [t("legal.version", { version: document.version })];
  if (document.published_at) {
    parts.push(
      t("legal.published_at", {
        date: date(document.published_at, "dd.MM.yyyy", locale),
      }),
    );
  }
  if (document.status === "retired") parts.push(t("legal.status_retired"));
  return (
    <div className="text-muted-foreground space-y-1 text-xs">
      <p>{parts.join(" · ")}</p>
      {document.language !== locale ? (
        <p>
          {document.language === "tr"
            ? t("legal.language_note_tr")
            : t("legal.language_note_en")}
        </p>
      ) : null}
    </div>
  );
}

/** Draft notice + rendered body of one text (title is rendered by the caller). */
export function LegalDocumentBody({ document }: { document: LegalDocument }) {
  return (
    <div className="space-y-4">
      {document.legal_review_pending ? <LegalDraftNotice /> : null}
      <LegalMarkdown source={document.body} />
    </div>
  );
}
