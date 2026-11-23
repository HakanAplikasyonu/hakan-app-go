"use client";

import { ArrowRight, Compass } from "lucide-react";
import Link from "next/link";

import { EntityPage, EntitySectionCard } from "@/components/entity";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { routes } from "@/config/routes";
import { useTenant } from "@/features/tenant/providers/tenant-provider";
import { useLocale } from "@/providers/locale-provider";

import { GUIDE_SECTIONS, type GuideSection } from "../lib/steps";
import { startTour } from "../lib/tour-store";

/**
 * `/t/{slug}/guide`: Rehber. İlk kurulum, her ay yapılanlar sırasıyla,
 * kilitli aya gelen belge ve beyanname takibi; her adımdan gerçek ekrana
 * tek tıkla gidilir. Adım adım tur buradan yeniden başlatılır.
 */
export function GuidePage() {
  const { t } = useLocale();
  const tenant = useTenant();
  return (
    <EntityPage
      title={t("help.guide.title")}
      description={t("help.guide.description")}
      breadcrumbs={[
        {
          label: tenant.organization.name,
          href: routes.tenant.home(tenant.slug),
        },
        { label: t("help.guide.title") },
      ]}
      actions={
        <Button type="button" onClick={startTour} data-testid="guide-start-tour">
          <Compass aria-hidden className="size-4" />
          {t("help.guide.start_tour")}
        </Button>
      }
    >
      <div className="flex min-w-0 flex-col gap-6">
        {GUIDE_SECTIONS.map((section) => (
          <GuideSectionCard
            key={section.id}
            section={section}
            slug={tenant.slug}
          />
        ))}
      </div>
    </EntityPage>
  );
}

function GuideSectionCard({
  section,
  slug,
}: {
  section: GuideSection;
  slug: string;
}) {
  const { t } = useLocale();
  const List = section.ordered ? "ol" : "ul";
  return (
    <section data-testid={`guide-section-${section.id}`}>
      <EntitySectionCard title={t(section.titleKey)}>
        <p className="text-muted-foreground mb-4 text-sm">
          {t(section.descriptionKey)}
        </p>
        <List className="flex flex-col divide-y">
          {section.steps.map((step, index) => (
            <li
              key={step.id}
              className="flex flex-col gap-2 py-3 first:pt-0 last:pb-0 sm:flex-row sm:items-start sm:gap-4"
              data-testid={`guide-step-${step.id}`}
            >
              {section.ordered ? (
                <span
                  aria-hidden
                  className="bg-brand-tint text-brand-text flex size-7 shrink-0 items-center justify-center rounded-full font-mono text-xs font-semibold"
                >
                  {index + 1}
                </span>
              ) : null}
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <h3 className="flex flex-wrap items-center gap-2 text-sm font-semibold">
                  {t(step.titleKey)}
                  {step.soon ? (
                    <Badge variant="secondary" className="font-normal">
                      {t("help.guide.soon")}
                    </Badge>
                  ) : null}
                </h3>
                <p className="text-muted-foreground text-sm leading-relaxed">
                  {t(step.bodyKey)}
                </p>
              </div>
              {step.href && !step.soon ? (
                <Button
                  asChild
                  variant="outline"
                  size="sm"
                  className="shrink-0 self-start"
                >
                  <Link href={step.href(slug)}>
                    {t("help.guide.go")}
                    <ArrowRight aria-hidden className="size-4" />
                  </Link>
                </Button>
              ) : null}
            </li>
          ))}
        </List>
      </EntitySectionCard>
    </section>
  );
}
