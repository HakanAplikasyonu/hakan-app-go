"use client";

import Link from "next/link";

import { routes } from "@/config/routes";
import { cn } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

import {
  guidesFor,
  helpRoles,
  PUBLIC_GUIDES,
  type HelpGuide,
} from "../lib/guides";
import { HelpGuideView } from "./help-guide-view";

/**
 * `/yardim` and `/yardim/{guide}` (#274): public usage guides for the office
 * panel, the mükellef portal / mobile app and the desktop agent. The guides
 * are Turkish only; the page chrome follows the locale.
 */
export function HelpPage({ guide }: { guide?: HelpGuide }) {
  const { t, locale } = useLocale();
  const { user } = useAuth();
  // Role göre (#522): kullanıcının kendi kılavuzları önce; iç belgeler yok.
  const { mine, others } = guidesFor(helpRoles(user));
  const ordered = [...mine, ...others];
  return (
    <div className="mx-auto flex min-h-svh w-full max-w-4xl flex-col gap-6 px-4 py-8 sm:px-6">
      <nav aria-label={t("help.nav_label")}>
        <p className="text-muted-foreground mb-2 text-xs font-medium tracking-wide uppercase">
          <Link href={routes.help.index()} className="hover:underline">
            {t("help.title")}
          </Link>
        </p>
        <ul className="flex flex-wrap gap-2">
          {ordered.map((g) => (
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
            {mine.length ? (
              <section aria-labelledby="help-mine" className="space-y-3">
                <h2
                  id="help-mine"
                  className="text-base font-semibold"
                  lang={locale}
                >
                  {t("help.for_you")}
                </h2>
                <GuideCards guides={mine} featured />
              </section>
            ) : null}
            <section aria-labelledby="help-all" className="space-y-3">
              {mine.length ? (
                <h2
                  id="help-all"
                  className="text-base font-semibold"
                  lang={locale}
                >
                  {t("help.all_guides")}
                </h2>
              ) : null}
              <GuideCards guides={mine.length ? others : PUBLIC_GUIDES} />
            </section>
          </div>
        )}
      </article>
    </div>
  );
}

function GuideCards({
  guides,
  featured = false,
}: {
  guides: readonly HelpGuide[];
  featured?: boolean;
}) {
  return (
    <ul className="grid gap-3 sm:grid-cols-2">
      {guides.map((g) => (
        <li key={g.slug}>
          <Link
            href={routes.help.guide(g.slug)}
            className={cn(
              "hover:bg-accent block h-full rounded-md border p-4 transition-colors",
              featured ? "border-brand bg-brand-tint" : "border-border",
            )}
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
  );
}
