// Pure helper (no React Native globals) so node --test can import it.

/** An https:// (or mailto: when allowed) link, or '' — never another scheme. */
export function safeLink(raw: string | undefined, allowMailto = false): string {
  const v = raw?.trim() ?? '';
  if (/^https:\/\/[^\s/]+/i.test(v)) return v;
  if (allowMailto && /^mailto:[^\s@]+@[^\s@]+$/i.test(v)) return v;
  return '';
}
