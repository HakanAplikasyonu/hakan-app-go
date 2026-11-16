import { notFound } from "next/navigation";

import { LegalPublicPage } from "@/features/legal";
import { isLegalKind, parseLegalVersion } from "@/features/legal/lib/kinds";

type PageProps = {
  params: Promise<{ kind: string }>;
  searchParams: Promise<{
    version?: string | string[];
    lang?: string | string[];
  }>;
};

function first(value: string | string[] | undefined) {
  return Array.isArray(value) ? value[0] : value;
}

/** Public KVKK text (issue #57), readable without signing in. */
export default async function LegalDocumentRoute({
  params,
  searchParams,
}: PageProps) {
  const { kind } = await params;
  if (!isLegalKind(kind)) notFound();
  const query = await searchParams;
  const lang = first(query.lang);
  return (
    <LegalPublicPage
      kind={kind}
      version={parseLegalVersion(first(query.version))}
      lang={lang === "tr" || lang === "en" ? lang : undefined}
    />
  );
}
