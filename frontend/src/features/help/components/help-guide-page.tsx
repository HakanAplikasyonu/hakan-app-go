"use client";

import { ArrowLeft, ArrowRight, ThumbsDown, ThumbsUp } from "lucide-react";
import Link from "next/link";
import { useEffect, useMemo, useState } from "react";

import { Button } from "@/components/ui/button";
import { Label } from "@/components/ui/label";
import {
  NativeSelect,
  NativeSelectOption,
} from "@/components/ui/native-select";
import { routes } from "@/config/routes";
import { openClickFix } from "@/lib/support/clickfix";
import { cn } from "@/lib/utils";
import { useAuth } from "@/providers/auth-provider";
import { useLocale } from "@/providers/locale-provider";

import { guidesFor, helpRoles, type HelpGuide } from "../lib/guides";
import { guideOutline, type GuideSection } from "../lib/outline";
import { HelpGuideView } from "./help-guide-view";

/** Görünür bölümü izler: içindekilerde etkin bölüm vurgulanır. */
function useActiveSection(ids: string[]) {
  const [active, setActive] = useState<string | null>(ids[0] ?? null);
  useEffect(() => {
    if (typeof IntersectionObserver === "undefined") return;
    const seen = new Map<string, boolean>();
    const io = new IntersectionObserver(
      (entries) => {
        for (const e of entries) seen.set(e.target.id, e.isIntersecting);
        const first = ids.find((id) => seen.get(id));
        if (first) setActive(first);
      },
      // Üst bantta görünen bölüm etkindir.
      { rootMargin: "-10% 0px -70% 0px" },
    );
    for (const id of ids) {
      const el = document.getElementById(id);
      if (el) io.observe(el);
    }
    return () => io.disconnect();
  }, [ids]);
  return active;
}

/** Bir kılavuz: içindekiler (yapışkan / telefonda açılır), bölümler, gezinme. */
export function HelpGuidePage({ guide }: { guide: HelpGuide }) {
  const { t, locale } = useLocale();
  const { user } = useAuth();
  const outline = useMemo(() => guideOutline(guide.body), [guide.body]);
  const ids = useMemo(() => outline.sections.map((s) => s.id), [outline]);
  const active = useActiveSection(ids);
  // Önceki / sonraki: kullanıcının sırası (kendi kılavuzları önce).
  const { mine, others } = guidesFor(helpRoles(user));
  const ordered = [...mine, ...others];
  const at = ordered.findIndex((g) => g.slug === guide.slug);
  const prev = at > 0 ? ordered[at - 1] : undefined;
  const next = at >= 0 ? ordered[at + 1] : undefined;
  const jump = (id: string) => {
    document.getElementById(id)?.scrollIntoView({ block: "start" });
    history.replaceState(null, "", `#${encodeURIComponent(id)}`);
  };

  return (
    <>
      <nav aria-label={t("help.nav_label")}>
        <Link
          href={routes.help.index()}
          className="text-muted-foreground hover:text-foreground inline-flex items-center gap-1.5 text-sm"
        >
          <ArrowLeft aria-hidden className="size-4" />
          {t("help.title")}
        </Link>
      </nav>

      <div className="grid gap-6 lg:grid-cols-[15rem_minmax(0,1fr)] lg:gap-10">
        {outline.sections.length ? (
          <>
            {/* Telefon ve tablet: içindekiler açılır menü. */}
            <div className="space-y-1 lg:hidden">
              <Label htmlFor="help-toc-select" lang={locale}>
                {t("help.toc")}
              </Label>
              <NativeSelect
                id="help-toc-select"
                wrapperClassName="w-full"
                value={active ?? ""}
                onChange={(e) => jump(e.target.value)}
                lang="tr"
              >
                {outline.sections.map((s) => (
                  <NativeSelectOption key={s.id} value={s.id}>
                    {s.title}
                  </NativeSelectOption>
                ))}
              </NativeSelect>
            </div>
            <aside className="hidden lg:block">
              <nav
                aria-label={t("help.toc")}
                className="sticky top-6 max-h-[calc(100svh-3rem)] overflow-y-auto"
              >
                <p
                  className="text-muted-foreground mb-2 text-xs font-medium tracking-wide uppercase"
                  lang={locale}
                >
                  {t("help.toc")}
                </p>
                <ul className="border-border space-y-0.5 border-s" lang="tr">
                  {outline.sections.map((s) => (
                    <li key={s.id}>
                      <a
                        href={`#${encodeURIComponent(s.id)}`}
                        aria-current={active === s.id ? "location" : undefined}
                        className={cn(
                          "focus-visible:ring-ring -ms-px block border-s-2 py-1.5 ps-3 text-sm transition-colors focus-visible:ring-2 focus-visible:outline-none",
                          active === s.id
                            ? "border-brand text-foreground font-medium"
                            : "text-muted-foreground hover:text-foreground border-transparent",
                        )}
                      >
                        {s.title}
                      </a>
                    </li>
                  ))}
                </ul>
              </nav>
            </aside>
          </>
        ) : null}

        <div className="min-w-0 space-y-6">
          <article
            lang="tr"
            className="bg-card text-card-foreground space-y-6 rounded-xl border p-5 sm:p-8"
          >
            <header className="space-y-2">
              <h1 className="font-display text-2xl font-bold tracking-tight sm:text-3xl">
                {outline.title || guide.title}
              </h1>
              {outline.summary ? (
                <p className="text-text-2 text-base">{outline.summary}</p>
              ) : null}
            </header>
            {outline.intro ? <GuideBody source={outline.intro} /> : null}
            {outline.sections.map((s) => (
              <Section key={s.id} section={s} />
            ))}
          </article>

          <Feedback key={guide.slug} />

          <nav
            aria-label={t("help.pager")}
            className="grid gap-3 sm:grid-cols-2"
          >
            {prev ? (
              <PagerLink guide={prev} label={t("help.prev")} />
            ) : (
              <span className="max-sm:hidden" />
            )}
            {next ? (
              <PagerLink guide={next} label={t("help.next")} end />
            ) : null}
          </nav>
        </div>
      </div>
    </>
  );
}

function GuideBody({ source }: { source: string }) {
  return (
    <div className="[&_blockquote]:border-brand [&_blockquote]:bg-brand-tint max-w-[72ch] text-[15px] leading-7 [&_blockquote]:rounded-md [&_blockquote]:border-s-4 [&_blockquote]:px-4 [&_blockquote]:py-3 [&_ol]:space-y-1.5 [&_ul]:space-y-1.5">
      <HelpGuideView body={source} />
    </div>
  );
}

function Section({ section }: { section: GuideSection }) {
  return (
    <section
      id={section.id}
      aria-labelledby={`${section.id}-baslik`}
      className="scroll-mt-6 space-y-3 border-t pt-6"
    >
      <h2
        id={`${section.id}-baslik`}
        className="font-display text-xl font-semibold tracking-tight"
      >
        {section.title}
      </h2>
      <GuideBody source={section.source} />
    </section>
  );
}

function PagerLink({
  guide,
  label,
  end = false,
}: {
  guide: HelpGuide;
  label: string;
  end?: boolean;
}) {
  return (
    <Link
      href={routes.help.guide(guide.slug)}
      className={cn(
        "hover:bg-accent focus-visible:ring-ring bg-card flex items-center gap-3 rounded-lg border p-4 transition-colors focus-visible:ring-2 focus-visible:outline-none",
        end && "justify-end text-end sm:col-start-2",
      )}
    >
      {end ? null : <ArrowLeft aria-hidden className="size-4 shrink-0" />}
      <span>
        <span className="text-muted-foreground block text-xs">{label}</span>
        <span className="block text-sm font-medium" lang="tr">
          {guide.title}
        </span>
      </span>
      {end ? <ArrowRight aria-hidden className="size-4 shrink-0" /> : null}
    </Link>
  );
}

/**
 * "Bu sayfa yardımcı oldu mu?": yanıt sunucuya gitmez. "Hayır" destek
 * sohbetini açar (varsa), kullanıcı eksik olanı oradan yazar.
 */
function Feedback() {
  const { t, locale } = useLocale();
  const [answer, setAnswer] = useState<"yes" | "no" | null>(null);
  const [chat, setChat] = useState(false);
  return (
    <section
      aria-labelledby="help-feedback"
      className="bg-card flex flex-wrap items-center gap-3 rounded-lg border p-4"
      lang={locale}
    >
      <h2 id="help-feedback" className="flex-1 text-sm font-medium">
        {t("help.feedback_question")}
      </h2>
      {answer ? (
        <p role="status" className="text-text-2 text-sm">
          {answer === "yes"
            ? t("help.feedback_thanks")
            : chat
              ? t("help.feedback_chat_opened")
              : t("help.feedback_contact")}
        </p>
      ) : (
        <div className="flex gap-2">
          <Button size="sm" variant="outline" onClick={() => setAnswer("yes")}>
            <ThumbsUp aria-hidden className="size-4" />
            {t("help.feedback_yes")}
          </Button>
          <Button
            size="sm"
            variant="outline"
            onClick={() => {
              setChat(openClickFix());
              setAnswer("no");
            }}
          >
            <ThumbsDown aria-hidden className="size-4" />
            {t("help.feedback_no")}
          </Button>
        </div>
      )}
    </section>
  );
}
