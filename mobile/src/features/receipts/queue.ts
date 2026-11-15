/**
 * Çevrimdışı fiş kuyruğu: saf durum geçişleri (React Native yok; node --test ile denenir).
 *
 * Bir fiş telefonda çekilir, kuyruğa `pending` girer ve bağlantı olduğunda
 * yüklenir. Sunucu cevabına göre:
 *   - 2xx              → uploaded (OCR sonucu `result`)
 *   - ağ / 5xx / 429   → pending, artan bekleme (30 sn … 30 dk)
 *   - 401              → pending (oturum yenilenince tekrar)
 *   - 404 / 405        → blocked: sunucu bu uç noktayı henüz sunmuyor (6 saatte bir dener)
 *   - diğer 4xx        → failed: kullanıcı tekrar dener ya da siler
 */
export type ReceiptKind = 'expense_receipt' | 'fuel_receipt' | 'z_report';

export type QueueStatus = 'pending' | 'uploading' | 'uploaded' | 'failed' | 'blocked';

export type ReceiptField = { raw: string; value: string; confidence: number; valid: boolean };

/** Subset of the API's `ReceiptCapture`. */
export type ReceiptResult = {
  uuid: string;
  status: 'draft' | 'duplicate' | 'failed' | 'approved' | 'rejected';
  kind: string | null;
  fields: Record<string, ReceiptField>;
  duplicate_of: string | null;
  error: string | null;
  plate_check?: 'registered' | 'not_registered' | 'no_plate' | 'error';
  in_review?: string[];
};

export type Correction = { field: string; value: string; note?: string };

export type QueueItem = {
  /** Client id; also sent as `Idempotency-Key`. */
  id: string;
  taxpayer: string;
  kind: ReceiptKind;
  /** Photo kept on the device until the upload succeeds. */
  fileUri: string;
  mimeType: string;
  /** epoch ms */
  createdAt: number;
  status: QueueStatus;
  attempts: number;
  /** epoch ms; 0 = now */
  nextAttemptAt: number;
  lastError: string | null;
  result: ReceiptResult | null;
  /** Corrections the user proposed for OCR fields (sent with the next sync). */
  corrections: Correction[];
  correctionsSent: boolean;
};

export type FailureKind = 'network' | 'server' | 'auth' | 'unsupported' | 'rejected';

const SECOND = 1000;
const MINUTE = 60 * SECOND;
export const BLOCKED_RETRY_MS = 6 * 60 * MINUTE;

export function newItem(input: { id: string; taxpayer: string; kind: ReceiptKind; fileUri: string; mimeType: string; now: number }): QueueItem {
  return {
    id: input.id,
    taxpayer: input.taxpayer,
    kind: input.kind,
    fileUri: input.fileUri,
    mimeType: input.mimeType,
    createdAt: input.now,
    status: 'pending',
    attempts: 0,
    nextAttemptAt: 0,
    lastError: null,
    result: null,
    corrections: [],
    correctionsSent: false,
  };
}

/** Backoff after `attempts` failures: 30 s, 1 m, 2 m, … capped at 30 m. */
export function backoffMs(attempts: number): number {
  return Math.min(30 * SECOND * 2 ** Math.max(0, attempts - 1), 30 * MINUTE);
}

/** Classifies an HTTP status (0 = no response) into a failure kind. */
export function classify(status: number): FailureKind {
  if (status === 0 || status === 429 || status >= 500) return status === 0 ? 'network' : 'server';
  if (status === 401) return 'auth';
  if (status === 404 || status === 405 || status === 501) return 'unsupported';
  return 'rejected';
}

/** Items of this company to upload now, oldest first. A forced run ignores the backoff. */
export function due(items: readonly QueueItem[], taxpayer: string, now: number, opts: { force?: boolean } = {}): QueueItem[] {
  return items
    .filter((i) => i.taxpayer === taxpayer)
    .filter((i) => i.status === 'pending' || i.status === 'blocked' || (i.status === 'uploading' && opts.force))
    .filter((i) => opts.force || i.nextAttemptAt <= now)
    .sort((a, b) => a.createdAt - b.createdAt);
}

export function markUploading(item: QueueItem): QueueItem {
  return { ...item, status: 'uploading' };
}

export function markUploaded(item: QueueItem, result: ReceiptResult): QueueItem {
  return { ...item, status: 'uploaded', result, lastError: null, nextAttemptAt: 0 };
}

export function markFailed(item: QueueItem, kind: FailureKind, message: string, now: number): QueueItem {
  const attempts = item.attempts + 1;
  switch (kind) {
    case 'network':
    case 'server':
      return { ...item, status: 'pending', attempts, nextAttemptAt: now + backoffMs(attempts), lastError: message };
    case 'auth':
      return { ...item, status: 'pending', attempts, nextAttemptAt: now + 30 * SECOND, lastError: message };
    case 'unsupported':
      return { ...item, status: 'blocked', attempts, nextAttemptAt: now + BLOCKED_RETRY_MS, lastError: message };
    case 'rejected':
      return { ...item, status: 'failed', attempts, nextAttemptAt: 0, lastError: message };
  }
}

/** Manual retry from the list. */
export function retry(item: QueueItem): QueueItem {
  return { ...item, status: 'pending', nextAttemptAt: 0 };
}

/** An item interrupted mid-upload (app killed) goes back to pending on the next launch. */
export function recover(items: readonly QueueItem[]): QueueItem[] {
  return items.map((i) => (i.status === 'uploading' ? { ...i, status: 'pending', nextAttemptAt: 0 } : i));
}

export function addCorrection(item: QueueItem, c: Correction): QueueItem {
  const rest = item.corrections.filter((x) => x.field !== c.field);
  return { ...item, corrections: [...rest, c], correctionsSent: false };
}

/** Uploaded items whose corrections still have to reach the server. */
export function correctionsDue(items: readonly QueueItem[], taxpayer: string): QueueItem[] {
  return items.filter((i) => i.taxpayer === taxpayer && i.status === 'uploaded' && i.result && i.corrections.length > 0 && !i.correctionsSent);
}

export type QueueSummary = { waiting: number; blocked: number; failed: number; uploaded: number };

export function summarize(items: readonly QueueItem[], taxpayer: string): QueueSummary {
  const mine = items.filter((i) => i.taxpayer === taxpayer);
  return {
    waiting: mine.filter((i) => i.status === 'pending' || i.status === 'uploading').length,
    blocked: mine.filter((i) => i.status === 'blocked').length,
    failed: mine.filter((i) => i.status === 'failed').length,
    uploaded: mine.filter((i) => i.status === 'uploaded').length,
  };
}
