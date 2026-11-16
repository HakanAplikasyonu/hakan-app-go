// Kaynak: mobile/src/core/api/envelope.ts (#236'da kopyalandı; ortak paket ayrı issue).
/**
 * Response envelope of the Go API: `{ success: true, data, meta }` or
 * `{ success: false, error: { code, message, details }, meta }`.
 * Pure (no React Native imports) so `node --test` can run it.
 */
import type { components } from './schema';

export type Schemas = components['schemas'];

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

/** Turkish, user-facing message for an error (codes the screens can hit). */
export function errorMessage(e: unknown): string {
  if (!isApiError(e)) return 'Beklenmeyen bir hata oluştu.';
  switch (e.code) {
    case 'INVALID_CREDENTIALS':
      return 'E-posta veya parola hatalı.';
    case 'MFA_REQUIRED':
      return 'Doğrulama uygulamanızdaki kodu girin.';
    case 'INVALID_MFA_CODE':
      return 'Doğrulama kodu hatalı.';
    case 'ACCOUNT_DEACTIVATED':
      return 'Bu hesap kapatılmış.';
    case 'RATE_LIMITED':
      return 'Çok fazla deneme yapıldı. Biraz sonra tekrar deneyin.';
    case 'NO_TENANT_MEMBERSHIP':
      return 'Bu ofise erişiminiz yok.';
    case 'ORGANIZATION_SUSPENDED':
      return 'Bu ofisin hesabı askıya alınmış.';
    case 'ORGANIZATION_ACCESS_EXPIRED':
      return 'Bu ofisin erişim süresi dolmuş.';
    case 'PERMISSION_DENIED':
    case 'FORBIDDEN':
      return 'Bu işlem için yetkiniz yok.';
    case 'NETWORK_ERROR':
    case 'TIMEOUT':
      return 'Sunucuya ulaşılamadı. Bağlantınızı kontrol edin.';
    case 'VALIDATION_ERROR':
      return e.details[0]?.message || 'Girilen bilgileri kontrol edin.';
    default:
      return e.message || 'Beklenmeyen bir hata oluştu.';
  }
}
