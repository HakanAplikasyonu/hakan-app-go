/**
 * Response envelope of the Go API: `{ success: true, data, meta }` or
 * `{ success: false, error: { code, message, details }, meta }`.
 * Pure (no React Native imports) so `node --test` can run it.
 */

export type ErrorDetail = { field?: string; message?: string; code?: string };

export class ApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly details: ErrorDetail[];
  readonly requestId?: string;

  constructor(init: { status: number; code: string; message: string; details?: ErrorDetail[]; requestId?: string }) {
    super(init.message);
    this.name = 'ApiError';
    this.status = init.status;
    this.code = init.code;
    this.details = init.details ?? [];
    this.requestId = init.requestId;
  }
}

export const isApiError = (e: unknown): e is ApiError => e instanceof ApiError;

type Envelope = { success?: unknown; data?: unknown; error?: { code?: unknown; message?: unknown; details?: unknown } };

/** Returns `data` of a 2xx envelope, or throws the API's error as ApiError. */
export function unwrap<T>(status: number, payload: unknown, requestId?: string): T {
  const body = (payload ?? {}) as Envelope;
  if (status >= 200 && status < 300 && body.success !== false) {
    return body.data as T;
  }
  const err = body.error ?? {};
  throw new ApiError({
    status,
    code: typeof err.code === 'string' && err.code ? err.code : status === 0 ? 'NETWORK_ERROR' : `HTTP_${status}`,
    message: typeof err.message === 'string' && err.message ? err.message : `Request failed (${status})`,
    details: Array.isArray(err.details) ? (err.details as ErrorDetail[]) : [],
    requestId,
  });
}

const UNEXPECTED = 'Beklenmeyen bir hata oluştu.';

/** Messages for codes both apps can hit. */
export const commonMessages: Readonly<Record<string, string>> = {
  INVALID_CREDENTIALS: 'E-posta veya parola hatalı.',
  MFA_REQUIRED: 'Doğrulama uygulamanızdaki kodu girin.',
  INVALID_MFA_CODE: 'Doğrulama kodu hatalı.',
  ACCOUNT_DEACTIVATED: 'Bu hesap kapatılmış.',
  RATE_LIMITED: 'Çok fazla deneme yapıldı. Biraz sonra tekrar deneyin.',
  NETWORK_ERROR: 'Sunucuya ulaşılamadı. Bağlantınızı kontrol edin.',
  TIMEOUT: 'Sunucuya ulaşılamadı. Bağlantınızı kontrol edin.',
};

/**
 * Builds the app's `errorMessage`: Turkish, user-facing text for an error.
 * `messages` adds the app's own codes (e.g. NO_TAXPAYER_MEMBERSHIP); other
 * codes fall back to the server message.
 */
export function createErrorMessage(messages: Readonly<Record<string, string>> = {}): (e: unknown) => string {
  return (e) => {
    if (!isApiError(e)) return UNEXPECTED;
    if (e.code === 'VALIDATION_ERROR') return e.details[0]?.message || 'Girilen bilgileri kontrol edin.';
    if (Object.hasOwn(messages, e.code)) return messages[e.code];
    if (Object.hasOwn(commonMessages, e.code)) return commonMessages[e.code];
    return e.message || UNEXPECTED;
  };
}
