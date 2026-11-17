import type { ExportIssue } from "../services/journals.service";

export type Translate = (
  key: string,
  vars?: Record<string, string | number>,
) => string;

/** Issue codes of the double booking guard (#466). */
export const DOUBLE_BOOKING_CODES = new Set([
  "DOCUMENT_ALREADY_EXPORTED",
  "PROGRAM_PULLS_DOCUMENT",
]);

export const str = (v: unknown) => (typeof v === "string" ? v : "");

export function dateText(locale: string, iso: string) {
  if (!iso) return "";
  return new Date(iso).toLocaleString(locale === "tr" ? "tr-TR" : "en-GB", {
    dateStyle: "short",
    timeStyle: "short",
  });
}

/** One double booking issue in Turkish: why the document is held back. */
export function doubleBookingText(
  t: Translate,
  locale: string,
  issue: ExportIssue,
) {
  const p = issue.params ?? {};
  if (issue.code === "PROGRAM_PULLS_DOCUMENT") {
    const integrator = str(p.integrator);
    return t("office.journals.dup.pulls", {
      integrator: t(`office.journals.integrator_${integrator}`),
    });
  }
  if (str(p.same_export_entry)) {
    return t("office.journals.dup.same_export", {
      entry: str(p.same_export_entry),
    });
  }
  return t("office.journals.dup.already", {
    file: str(p.previous_file),
    entry: str(p.previous_entry),
    date: dateText(locale, str(p.previous_exported_at)),
  });
}
