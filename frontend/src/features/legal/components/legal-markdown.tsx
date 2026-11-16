import { Fragment, type ReactNode } from "react";

import { cn } from "@/lib/utils";

/**
 * Minimal Markdown renderer for the KVKK texts (issue #57).
 *
 * The texts are authored by the platform, but they are still rendered as
 * React elements only: no HTML is parsed and nothing goes through
 * `dangerouslySetInnerHTML`, so a stray tag shows up as text. Supported:
 * headings, paragraphs, bullet and numbered lists, tables, block quotes,
 * horizontal rules, **bold**, *italic*, `code` and [links](https://…)
 * (http(s), mailto and same-site paths only).
 */

type Block =
  | { type: "heading"; level: number; text: string }
  | { type: "paragraph"; text: string }
  | { type: "list"; ordered: boolean; start: number; items: string[] }
  | { type: "table"; header: string[]; rows: string[][] }
  | { type: "quote"; text: string }
  | { type: "rule" };

const HEADING_RE = /^(#{1,6})\s+(.*?)\s*#*\s*$/;
const RULE_RE = /^\s*([-*_])(\s*\1){2,}\s*$/;
const BULLET_RE = /^\s*[-*+]\s+(.*)$/;
const ORDERED_RE = /^\s*(\d+)[.)]\s+(.*)$/;
const TABLE_SEPARATOR_RE = /^\s*\|?\s*:?-{2,}:?\s*(\|\s*:?-{2,}:?\s*)*\|?\s*$/;
const QUOTE_RE = /^\s*>\s?(.*)$/;

function splitRow(line: string): string[] {
  let row = line.trim();
  if (row.startsWith("|")) row = row.slice(1);
  if (row.endsWith("|") && !row.endsWith("\\|")) row = row.slice(0, -1);
  // Split on "|" but keep escaped "\|" inside a cell.
  const cells: string[] = [];
  let cell = "";
  for (let i = 0; i < row.length; i++) {
    const ch = row[i];
    if (ch === "\\" && row[i + 1] === "|") {
      cell += "|";
      i++;
    } else if (ch === "|") {
      cells.push(cell.trim());
      cell = "";
    } else {
      cell += ch;
    }
  }
  cells.push(cell.trim());
  return cells;
}

function isTableStart(lines: string[], i: number) {
  return (
    lines[i]?.trim().startsWith("|") &&
    i + 1 < lines.length &&
    TABLE_SEPARATOR_RE.test(lines[i + 1])
  );
}

function startsBlock(lines: string[], i: number) {
  const line = lines[i];
  return (
    HEADING_RE.test(line) ||
    RULE_RE.test(line) ||
    BULLET_RE.test(line) ||
    ORDERED_RE.test(line) ||
    QUOTE_RE.test(line) ||
    isTableStart(lines, i)
  );
}

export function parseMarkdown(source: string): Block[] {
  const lines = source.replace(/\r\n?/g, "\n").split("\n");
  const blocks: Block[] = [];
  let i = 0;

  while (i < lines.length) {
    const line = lines[i];
    if (!line.trim()) {
      i++;
      continue;
    }

    const heading = HEADING_RE.exec(line);
    if (heading) {
      blocks.push({
        type: "heading",
        level: heading[1].length,
        text: heading[2],
      });
      i++;
      continue;
    }

    if (RULE_RE.test(line)) {
      blocks.push({ type: "rule" });
      i++;
      continue;
    }

    if (isTableStart(lines, i)) {
      const header = splitRow(line);
      const rows: string[][] = [];
      i += 2;
      while (i < lines.length && lines[i].trim().startsWith("|")) {
        rows.push(splitRow(lines[i]));
        i++;
      }
      blocks.push({ type: "table", header, rows });
      continue;
    }

    const bullet = BULLET_RE.exec(line);
    const ordered = ORDERED_RE.exec(line);
    if (bullet || ordered) {
      const isOrdered = !bullet;
      const items: string[] = [];
      const start = ordered ? Number(ordered[1]) : 1;
      while (i < lines.length) {
        const cur = lines[i];
        const m = isOrdered ? ORDERED_RE.exec(cur) : BULLET_RE.exec(cur);
        if (m) {
          items.push(isOrdered ? m[2] : m[1]);
          i++;
          continue;
        }
        // Indented continuation of the previous item.
        if (cur.trim() && /^\s+/.test(cur) && !startsBlock(lines, i)) {
          items[items.length - 1] += ` ${cur.trim()}`;
          i++;
          continue;
        }
        break;
      }
      blocks.push({ type: "list", ordered: isOrdered, start, items });
      continue;
    }

    if (QUOTE_RE.test(line)) {
      const parts: string[] = [];
      while (i < lines.length) {
        const m = QUOTE_RE.exec(lines[i]);
        if (!m) break;
        parts.push(m[1]);
        i++;
      }
      blocks.push({ type: "quote", text: parts.join(" ").trim() });
      continue;
    }

    const parts: string[] = [];
    while (i < lines.length && lines[i].trim() && !startsBlock(lines, i)) {
      parts.push(lines[i].trim());
      i++;
    }
    blocks.push({ type: "paragraph", text: parts.join(" ") });
  }

  return blocks;
}

const INLINE_RE =
  /\*\*(.+?)\*\*|__(.+?)__|`([^`]+)`|\[([^\]]+)\]\(([^)\s]+)\)|\*([^*\s](?:[^*]*[^*\s])?)\*/g;

function safeHref(href: string): string | null {
  if (/^(https?:|mailto:)/i.test(href)) return href;
  // Same-site paths only; "//host" would leave the site.
  if (href.startsWith("/") && !href.startsWith("//")) return href;
  if (href.startsWith("#")) return href;
  return null;
}

function renderInline(text: string, keyPrefix = "i"): ReactNode[] {
  const out: ReactNode[] = [];
  let last = 0;
  let n = 0;
  for (const m of text.matchAll(INLINE_RE)) {
    const index = m.index ?? 0;
    if (index > last) out.push(text.slice(last, index));
    const key = `${keyPrefix}-${n++}`;
    if (m[1] !== undefined || m[2] !== undefined) {
      out.push(
        <strong key={key} className="font-semibold">
          {renderInline(m[1] ?? m[2], key)}
        </strong>,
      );
    } else if (m[3] !== undefined) {
      out.push(
        <code key={key} className="bg-muted rounded px-1 py-0.5 text-[0.9em]">
          {m[3]}
        </code>,
      );
    } else if (m[4] !== undefined) {
      const href = safeHref(m[5]);
      const label = renderInline(m[4], key);
      if (!href) {
        out.push(<Fragment key={key}>{label}</Fragment>);
      } else {
        const external = /^https?:/i.test(href);
        out.push(
          <a
            key={key}
            href={href}
            className="text-primary underline underline-offset-2"
            {...(external
              ? { target: "_blank", rel: "noopener noreferrer" }
              : {})}
          >
            {label}
          </a>,
        );
      }
    } else if (m[6] !== undefined) {
      out.push(<em key={key}>{renderInline(m[6], key)}</em>);
    }
    last = index + m[0].length;
  }
  if (last < text.length) out.push(text.slice(last));
  return out;
}

const HEADING_CLASS: Record<number, string> = {
  2: "mt-6 text-base font-semibold first:mt-0",
  3: "mt-5 text-sm font-semibold first:mt-0",
  4: "mt-4 text-sm font-medium first:mt-0",
};

function Heading({ level, children }: { level: number; children: ReactNode }) {
  // The page/dialog title is the h1/h2 above the body: start at h2.
  const tag = Math.min(level + 1, 4);
  const className = HEADING_CLASS[tag];
  if (tag === 2) return <h2 className={className}>{children}</h2>;
  if (tag === 3) return <h3 className={className}>{children}</h3>;
  return <h4 className={className}>{children}</h4>;
}

export function LegalMarkdown({
  source,
  className,
}: {
  source: string;
  className?: string;
}) {
  const blocks = parseMarkdown(source);
  return (
    <div className={cn("space-y-3 text-sm leading-relaxed", className)}>
      {blocks.map((block, idx) => {
        const key = `b-${idx}`;
        switch (block.type) {
          case "heading":
            return (
              <Heading key={key} level={block.level}>
                {renderInline(block.text, key)}
              </Heading>
            );
          case "paragraph":
            return <p key={key}>{renderInline(block.text, key)}</p>;
          case "quote":
            return (
              <blockquote
                key={key}
                className="border-border text-muted-foreground border-s-2 ps-3"
              >
                {renderInline(block.text, key)}
              </blockquote>
            );
          case "rule":
            return <hr key={key} className="border-border" />;
          case "list": {
            const items = block.items.map((item, j) => (
              <li key={`${key}-${j}`}>{renderInline(item, `${key}-${j}`)}</li>
            ));
            return block.ordered ? (
              <ol
                key={key}
                start={block.start}
                className="list-decimal space-y-1 ps-5"
              >
                {items}
              </ol>
            ) : (
              <ul key={key} className="list-disc space-y-1 ps-5">
                {items}
              </ul>
            );
          }
          case "table":
            return (
              <div key={key} className="overflow-x-auto">
                <table className="border-border w-full border-collapse border text-left text-xs sm:text-sm">
                  <thead className="bg-muted/50">
                    <tr>
                      {block.header.map((cell, j) => (
                        <th
                          key={`${key}-h-${j}`}
                          scope="col"
                          className="border-border border px-2 py-1.5 font-medium"
                        >
                          {renderInline(cell, `${key}-h-${j}`)}
                        </th>
                      ))}
                    </tr>
                  </thead>
                  <tbody>
                    {block.rows.map((row, r) => (
                      <tr key={`${key}-r-${r}`}>
                        {block.header.map((_, c) => (
                          <td
                            key={`${key}-r-${r}-${c}`}
                            className="border-border border px-2 py-1.5 align-top"
                          >
                            {renderInline(row[c] ?? "", `${key}-r-${r}-${c}`)}
                          </td>
                        ))}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            );
        }
      })}
    </div>
  );
}
