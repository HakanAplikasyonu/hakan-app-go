"use client";

import { BookOpen, Search } from "lucide-react";
import Link from "next/link";
import { useDeferredValue, useId, useMemo, useState } from "react";

import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
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
import { guideOutline, searchGuides } from "../lib/outline";

const guideHref = (slug: string, section?: string) =>
  `${routes.help.guide(slug)}${section ? `#${encodeURIComponent(section)}` : ""}`;

/** Yardım merkezi ana sayfası: arama, role göre kılavuzlar, sık bakılanlar. */
export function HelpHome() {
  const { t, locale } = useLocale();
  const { user } = useAuth();
  const id = useId();
  const [query, setQuery] = useState("");
  const deferred = useDeferredValue(query);
  const { mine, others } = guidesFor(helpRoles(user));
  const hits = useMemo(() => searchGuides(deferred, PUBLIC_GUIDES), [deferred]);
  const searching = deferred.trim().length >= 2;
  // Sık bakılanlar: kullanıcının kılavuzunun (ziyaretçide ilk kılavuzun)
  // bölümleri ve "Sık sorulanlar" bölümü olan kılavuzlar.
  const focus = mine[0] ?? PUBLIC_GUIDES[0];
  const topics = useMemo(
    () => (focus ? guideOutline(focus.body).sections.slice(0, 6) : []),
    [focus],
  );
  const faqs = useMemo(
    () =>
      PUBLIC_GUIDES.flatMap((g) => {
        const s = guideOutline(g.body).sections.find((x) =>
          x.title.toLocaleLowerCase("tr").startsWith("sık sorulan"),
        );
        return s ? [{ guide: g, section: s }] : [];
      }),
    [],
  );

  return (
    <>
      <header className="bg-brand-tint border-brand/30 space-y-4 rounded-2xl border p-6 sm:p-8">
        <div className="space-y-1">
          <h1
            className="font-display text-2xl font-bold tracking-tight sm:text-3xl"
            lang={locale}
          >
            {t("help.title")}
          </h1>
          <p className="text-foreground text-sm" lang={locale}>
            {t("help.description")}
          </p>
        </div>
        <div className="relative max-w-xl">
          <Label htmlFor={`${id}-q`} className="sr-only">
            {t("help.search_label")}
          </Label>
          <Search
            aria-hidden
            className="text-muted-foreground pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2"
          />
          <Input
            id={`${id}-q`}
            type="search"
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder={t("help.search_placeholder")}
            className="bg-background h-11 pl-9"
            autoComplete="off"
          />
        </div>
        {locale !== "tr" ? (
          <p className="text-foreground text-xs" lang={locale}>
            {t("help.turkish_only")}
          </p>
        ) : null}
      </header>

      {searching ? (
        <section
          aria-labelledby={`${id}-results`}
          aria-live="polite"
          className="space-y-3"
          lang="tr"
        >
          <h2
            id={`${id}-results`}
            className="text-base font-semibold"
            lang={locale}
          >
            {t("help.search_results", { count: hits.length })}
          </h2>
          {hits.length === 0 ? (
            <p className="text-muted-foreground text-sm" lang={locale}>
              {t("help.search_empty")}
            </p>
          ) : (
            <ul className="bg-card divide-border divide-y rounded-lg border">
              {hits.map((h, i) => (
                <li key={`${h.guide.slug}-${h.section?.id ?? "giris"}-${i}`}>
                  <Link
                    href={guideHref(h.guide.slug, h.section?.id)}
                    className="hover:bg-accent focus-visible:ring-ring block px-4 py-3 focus-visible:ring-2 focus-visible:outline-none"
                  >
                    <span className="block text-sm font-medium">
                      {h.section?.title ?? h.guide.title}
                    </span>
                    <span className="text-muted-foreground block text-xs">
                      {h.guide.title}
                    </span>
                    <span className="text-text-2 mt-1 block text-sm">
                      {h.snippet}
                    </span>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </section>
      ) : (
        <div className="space-y-8" lang="tr">
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
            <h2 id="help-all" className="text-base font-semibold" lang={locale}>
              {t("help.all_guides")}
            </h2>
            <GuideCards guides={mine.length ? others : PUBLIC_GUIDES} />
          </section>
          {focus && topics.length ? (
            <section aria-labelledby="help-topics" className="space-y-3">
              <h2
                id="help-topics"
                className="text-base font-semibold"
                lang={locale}
              >
                {t("help.topics")}
              </h2>
              <ul className="grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
                {topics.map((s) => (
                  <li key={s.id}>
                    <Link
                      href={guideHref(focus.slug, s.id)}
                      className="hover:bg-accent border-border block rounded-md border px-3 py-2 text-sm transition-colors"
                    >
                      {s.title}
                    </Link>
                  </li>
                ))}
                {faqs.map(({ guide, section }) => (
                  <li key={`faq-${guide.slug}`}>
                    <Link
                      href={guideHref(guide.slug, section.id)}
                      className="hover:bg-accent border-border block rounded-md border px-3 py-2 text-sm transition-colors"
                    >
                      {section.title}
                      <span className="text-muted-foreground block text-xs">
                        {guide.title}
                      </span>
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          ) : null}
        </div>
      )}
    </>
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
    <ul className="grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {guides.map((g) => (
        <li key={g.slug}>
          <Link
            href={routes.help.guide(g.slug)}
            className={cn(
              "hover:bg-accent focus-visible:ring-ring bg-card flex h-full gap-3 rounded-lg border p-4 transition-colors focus-visible:ring-2 focus-visible:outline-none",
              featured ? "border-brand bg-brand-tint" : "border-border",
            )}
          >
            <BookOpen
              aria-hidden
              className={cn(
                "mt-0.5 size-5 shrink-0",
                featured ? "text-foreground" : "text-muted-foreground",
              )}
            />
            <span>
              <span className="block font-medium">{g.title}</span>
              {g.summary ? (
                <span
                  className={cn(
                    "mt-1 block text-sm",
                    // Renkli zeminde soluk metin kontrastı düşürür.
                    featured ? "text-foreground" : "text-muted-foreground",
                  )}
                >
                  {g.summary}
                </span>
              ) : null}
            </span>
          </Link>
        </li>
      ))}
    </ul>
  );
}
