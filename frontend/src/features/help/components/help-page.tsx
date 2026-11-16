"use client";

import Link from "next/link";

import { routes } from "@/config/routes";
import { cn } from "@/lib/utils";
import { useLocale } from "@/providers/locale-provider";

import { HELP_GUIDES, type HelpGuide } from "../lib/guides";
import { HelpGuideView } from "./help-guide-view";

/**
 * `/yardim` and `/yardim/{guide}` (#274): public usage guides for the office
 * panel, the mükellef portal / mobile app and the desktop agent. The guides
 * are Turkish only; the page chrome follows the locale.
 */
export function HelpPage({ guide }: { guide?: HelpGuide }) {
  const { t, locale } = useLocale();
  return (
    <div className="mx-auto flex min-h-svh w-full max-w-4xl flex-col gap-6 px-4 py-8 sm:px-6">
      <nav aria-label={t("help.nav_label")}>
        <p className="text-muted-foreground mb-2 text-xs font-medium tracking-wide uppercase">
          <Link href={routes.help.index()} className="hover:underline">
            {t("help.title")}
          </Link>
        </p>
        <ul className="flex flex-wrap gap-2">
          {HELP_GUIDES.map((g) => (
            <li key={g.slug}>
              <Link
                href={routes.help.guide(g.slug)}
                aria-current={g.slug === guide?.slug ? "page" : undefined}
                className={cn(
                  "border-border hover:bg-accent inline-flex rounded-md border px-3 py-1.5 text-sm transition-colors",
                  g.slug === guide?.slug &&
                    "bg-primary text-primary-foreground hover:bg-primary/90 border-transparent",
                )}
              >
                {g.title}
              </Link>
            </li>
          ))}
        </ul>
      </nav>

      <article
        lang="tr"
        className="border-border bg-card text-card-foreground space-y-4 rounded-lg border p-5 sm:p-8"
      >
        {locale !== "tr" ? (
          <p className="text-muted-foreground text-xs" lang={locale}>
            {t("help.turkish_only")}
          </p>
        ) : null}
        {guide ? (
          <HelpGuideView body={guide.body} />
        ) : (
          <div className="space-y-4">
            <h1 className="text-2xl font-semibold tracking-tight" lang={locale}>
              {t("help.title")}
            </h1>
            <p className="text-muted-foreground text-sm" lang={locale}>
              {t("help.description")}
            </p>
            <ul className="grid gap-3 sm:grid-cols-2">
              {HELP_GUIDES.map((g) => (
                <li key={g.slug}>
                  <Link
                    href={routes.help.guide(g.slug)}
                    className="border-border hover:bg-accent block h-full rounded-md border p-4 transition-colors"
                  >
                    <span className="block font-medium">{g.title}</span>
                    {g.summary ? (
                      <span className="text-muted-foreground mt-1 block text-sm">
                        {g.summary}
                      </span>
                    ) : null}
                  </Link>
                </li>
              ))}
            </ul>
          </div>
        )}
      </article>
    </div>
  );
}
