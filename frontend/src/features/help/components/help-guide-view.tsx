import { LegalMarkdown } from "@/features/legal/components/legal-markdown";

import { splitGuide } from "../lib/segments";

/**
 * One usage guide (#274). Text goes through the same React-only Markdown
 * renderer as the KVKK texts (no HTML parsing); screenshots and code blocks
 * are rendered here.
 */
export function HelpGuideView({ body }: { body: string }) {
  return (
    <div className="space-y-4">
      {splitGuide(body).map((seg, i) => {
        const key = `s-${i}`;
        switch (seg.type) {
          case "text":
            return (
              <LegalMarkdown
                key={key}
                source={seg.source}
                className="[&_h1]:text-2xl [&_h2]:mt-6 [&_h3]:mt-4"
              />
            );
          case "image":
            return (
              <figure key={key} className="space-y-1">
                {/* Static screenshots from public/; next/image adds nothing here. */}
                {/* eslint-disable-next-line @next/next/no-img-element */}
                <img
                  src={seg.src}
                  alt={seg.alt}
                  loading="lazy"
                  className="border-border w-full rounded-md border"
                />
                {seg.alt ? (
                  <figcaption className="text-muted-foreground text-xs">
                    {seg.alt}
                  </figcaption>
                ) : null}
              </figure>
            );
          case "code":
            return (
              <pre
                key={key}
                className="bg-muted overflow-x-auto rounded-md p-3 text-xs"
              >
                <code>{seg.code}</code>
              </pre>
            );
        }
      })}
    </div>
  );
}
