import { isApiError } from "@/lib/api";

type Translate = (key: string) => string;

/** API error in Turkish when the code is known, else the server message (#161). */
export function journalErrorText(t: Translate, err: unknown, fallback: string) {
  if (isApiError(err)) {
    const key = `office.journals.error_${err.code}`;
    const msg = err.code ? t(key) : key;
    return msg !== key ? msg : err.message || fallback;
  }
  return err instanceof Error ? err.message : fallback;
}
