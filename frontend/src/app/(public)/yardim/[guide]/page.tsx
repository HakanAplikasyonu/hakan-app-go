import type { Metadata } from "next";
import { notFound } from "next/navigation";

import { findGuide, HelpPage, HELP_GUIDES } from "@/features/help";

type PageProps = { params: Promise<{ guide: string }> };

export function generateStaticParams() {
  return HELP_GUIDES.map((g) => ({ guide: g.slug }));
}

export async function generateMetadata({
  params,
}: PageProps): Promise<Metadata> {
  const guide = findGuide((await params).guide);
  return { title: guide ? `${guide.title} · Yardım` : "Yardım" };
}

/** One usage guide (#274), public. */
export default async function HelpGuideRoute({ params }: PageProps) {
  const guide = findGuide((await params).guide);
  if (!guide) notFound();
  return <HelpPage guide={guide} />;
}
