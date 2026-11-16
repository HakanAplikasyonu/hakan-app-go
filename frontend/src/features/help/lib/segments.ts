/**
 * Splits a guide (docs/kullanim/*.md, #274) into what the help renderer
 * shows: text for LegalMarkdown, standalone images and fenced code. Links
 * between guides (`ofis-calisani.md#bolum`) become `/yardim/...` routes and
 * screenshot paths (`../../frontend/public/yardim/img/x.jpg`) become
 * `/yardim/img/x.jpg`, so the same Markdown reads well on GitHub and in the
 * app.
 */
export type HelpSegment =
  | { type: "text"; source: string }
  | { type: "image"; alt: string; src: string }
  | { type: "code"; code: string };

const IMAGE_LINE_RE = /^\s*!\[([^\]]*)\]\(([^)\s]+)\)\s*$/;
const IMG_PREFIX_RE =
  /^(?:\.\.\/)*frontend\/public(\/yardim\/img\/[A-Za-z0-9._-]+)$/;
const GUIDE_LINK_RE = /\]\((?:\.\/)?([a-z0-9-]+)\.md(#[A-Za-z0-9_-]+)?\)/g;

/** `/yardim/img/x.jpg` for a screenshot path, null for anything else. */
export function imageSrc(raw: string): string | null {
  const m = IMG_PREFIX_RE.exec(raw.trim());
  return m ? m[1] : null;
}

export function rewriteGuideLinks(source: string): string {
  return source.replace(GUIDE_LINK_RE, (_, slug: string, hash?: string) =>
    slug === "README" ? "](/yardim)" : `](/yardim/${slug}${hash ?? ""})`,
  );
}

export function splitGuide(body: string): HelpSegment[] {
  const lines = body.replace(/\r\n?/g, "\n").split("\n");
  const segments: HelpSegment[] = [];
  let text: string[] = [];
  const flush = () => {
    const source = rewriteGuideLinks(text.join("\n")).trim();
    if (source) segments.push({ type: "text", source });
    text = [];
  };
  for (let i = 0; i < lines.length; i++) {
    const line = lines[i];
    if (line.trimStart().startsWith("```")) {
      flush();
      const code: string[] = [];
      i++;
      while (i < lines.length && !lines[i].trimStart().startsWith("```")) {
        code.push(lines[i]);
        i++;
      }
      segments.push({ type: "code", code: code.join("\n") });
      continue;
    }
    const img = IMAGE_LINE_RE.exec(line);
    const src = img ? imageSrc(img[2]) : null;
    if (img && src) {
      flush();
      segments.push({ type: "image", alt: img[1], src });
      continue;
    }
    text.push(line);
  }
  flush();
  return segments;
}
