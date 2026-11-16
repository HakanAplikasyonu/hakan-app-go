"use client";

import { useCallback } from "react";

import { isApiError } from "@/lib/api";
import { useLocale } from "@/providers/locale-provider";

/** The zero UUID the API used for "no invoice"; never shown (#161). */
export const NIL_UUID = "00000000-0000-0000-0000-000000000000";

export const isRealUUID = (v?: string | null): v is string =>
  !!v && v !== NIL_UUID;

type T = (key: string, vars?: Record<string, string | number>) => string;

/** Translation of key, or undefined when the locale has none. */
function maybe(t: T, key: string, vars?: Record<string, string | number>) {
  const s = t(key, vars);
  return s === key ? undefined : s;
}

function paramsOf(message?: string): Record<string, string | number> {
  if (!message) return {};
  try {
    const v: unknown = JSON.parse(message);
    if (v && typeof v === "object" && !Array.isArray(v)) {
      return Object.fromEntries(
        Object.entries(v as Record<string, unknown>).map(([k, x]) => [
          k,
          typeof x === "number" ? x : String(x),
        ]),
      );
    }
  } catch {
    // not JSON: a plain message
  }
  return {};
}

/**
 * Turkish (or English) texts for the office panel's API codes (#161): UBL
 * upload error codes, journal / export error codes, export issue codes and
 * export driver names. Unknown codes fall back to a generic text, never to
 * the server's English message.
 */
export function useOfficeMessages() {
  const { t } = useLocale();

  /** UBL upload report: error_code of a failed file. */
  const uploadError = useCallback(
    (code?: string) =>
      (code && maybe(t, `office.errors.upload.${code}`)) ??
      t("office.errors.upload.unknown"),
    [t],
  );

  /** A failed API call (journal drafts, exports, uploads). */
  const apiError = useCallback(
    (err: unknown) => {
      if (isApiError(err)) {
        const known = err.code
          ? maybe(t, `office.errors.api.${err.code}`)
          : undefined;
        if (known) return known;
        if (err.status === 403) return t("office.errors.api.forbidden");
        if (err.status === 404) return t("office.errors.api.not_found");
        if (err.status === 409) return t("office.errors.api.conflict");
      }
      return t("office.errors.api.generic");
    },
    [t],
  );

  /** One issue that stopped an export (error.details of EXPORT_BLOCKED). */
  const exportIssue = useCallback(
    (d: { code?: string; field?: string; message?: string }) => {
      const vars = paramsOf(d.message);
      const text =
        (d.code && maybe(t, `office.errors.issue.${d.code}`, vars)) ??
        t("office.errors.issue.unknown", { code: d.code ?? "" });
      const m = /^entry:([^/]+)(?:\/line:(\d+))?$/.exec(d.field ?? "");
      if (!m) return text;
      const where = m[2]
        ? t("office.errors.issue.where_line", { entry: m[1], line: m[2] })
        : t("office.errors.issue.where_entry", { entry: m[1] });
      return `${where}: ${text}`;
    },
    [t],
  );

  /** Export driver name → label ("luca" → "Luca"). */
  const driverLabel = useCallback(
    (name: string) => maybe(t, `office.journals.driver_label.${name}`) ?? name,
    [t],
  );

  return { uploadError, apiError, exportIssue, driverLabel };
}
