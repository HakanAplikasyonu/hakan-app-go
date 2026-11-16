"use client";

import Link from "next/link";

import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import {
  LegalDocumentBody,
  LegalDocumentMeta,
} from "@/features/legal/components/legal-document-view";
import { useLegalDocument } from "@/features/legal/hooks/use-legal";
import { LEGAL_KINDS, legalKindLabelKey } from "@/features/legal/lib/kinds";
import type { LegalKind } from "@/features/legal/services/legal.service";
import { ApiError } from "@/lib/api/errors";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

/**
 * `/legal/{kind}` — public KVKK text (issue #57): the current version, or
 * `?version=` (what someone accepted, possibly retired) in `?lang=`. Links
 * to the other texts on top.
 */
export function LegalPublicPage({
  kind,
  version,
  lang,
}: {
  kind: LegalKind;
  version?: number;
  lang?: string;
}) {
  const { t } = useLocale();
  const query = useLegalDocument(kind, { version, lang });
  const doc = query.data;
  const notFound =
    query.error instanceof ApiError &&
    (query.error.isNotFound || query.error.isValidation);

  return (
    <div className="mx-auto flex min-h-svh w-full max-w-3xl flex-col gap-6 px-4 py-8 sm:px-6">
      <nav aria-label={t("legal.nav_label")}>
        <p className="text-muted-foreground mb-2 text-xs font-medium tracking-wide uppercase">
          {t("legal.nav_label")}
        </p>
        <ul className="flex flex-wrap gap-2">
          {LEGAL_KINDS.map((k) => (
            <li key={k}>
              <Link
                href={routes.legal(k)}
                aria-current={k === kind ? "page" : undefined}
                className={cn(
                  "border-border hover:bg-accent inline-flex rounded-md border px-3 py-1.5 text-sm transition-colors",
                  k === kind &&
                    "bg-primary text-primary-foreground hover:bg-primary/90 border-transparent",
                )}
              >
                {t(legalKindLabelKey(k))}
              </Link>
            </li>
          ))}
        </ul>
      </nav>

      <article className="border-border bg-card text-card-foreground space-y-4 rounded-lg border p-5 sm:p-8">
        {doc ? (
          <>
            <header className="space-y-2">
              <h1 className="text-xl font-semibold tracking-tight">
                {doc.title}
              </h1>
              <LegalDocumentMeta document={doc} />
              {version && doc.status === "retired" ? (
                <p className="text-sm">
                  {t("legal.retired_note")}{" "}
                  <Link
                    href={routes.legal(kind)}
                    className="text-primary underline underline-offset-2"
                  >
                    {t("legal.see_current")}
                  </Link>
                </p>
              ) : null}
            </header>
            <LegalDocumentBody document={doc} />
          </>
        ) : query.isLoading ? (
          <p className="text-muted-foreground text-sm">{t("common.loading")}</p>
        ) : (
          <div className="space-y-3">
            <h1 className="text-xl font-semibold tracking-tight">
              {notFound ? t("legal.not_found_title") : t("legal.load_error")}
            </h1>
            {notFound ? (
              <p className="text-muted-foreground text-sm">
                {t("legal.not_found_description")}
              </p>
            ) : (
              <Button variant="outline" onClick={() => void query.refetch()}>
                {t("common.retry")}
              </Button>
            )}
          </div>
        )}
      </article>
    </div>
  );
}
