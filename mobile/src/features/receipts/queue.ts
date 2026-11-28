/**
 * Çevrimdışı fiş kuyruğu: saf durum geçişleri (React Native yok; node --test ile denenir).
 *
 * Bir fiş telefonda çekilir, kuyruğa `pending` girer ve bağlantı olduğunda
 * yüklenir. Sunucu cevabına göre:
 *   - 2xx              → uploaded (OCR sonucu `result`)
 *   - ağ / 5xx / 429   → pending, artan bekleme (30 sn … 30 dk)
 *   - 408 UPLOAD_TIMEOUT ve eski sunucunun yarıda kesilen yükleme için
 *     döndüğü 400 ("multipart form …") → pending, artan bekleme (#780:
 *     yavaş mobil ağda fotoğraf sunucunun okuma süresini aşıyordu; fiş
 *     "failed" kalıp ofise hiç ulaşmıyordu)
 *   - 401              → pending (oturum yenilenince tekrar)
 *   - 404 / 405 / 501  → blocked: sunucu bu uç noktayı sunmuyor (eski sürüm; 6 saatte bir dener)
 *   - 409 NO_ACTIVE_ENGAGEMENT → blocked: şirketin etkin ofisi yok, ofis bağlanınca yüklenir
 *   - diğer 4xx        → failed: kullanıcı tekrar dener ya da siler
 *
 * Uç nokta: POST /v1/taxpayer/receipts (#199). Aynı fotoğraf aynı
 * Idempotency-Key ile tekrar gönderilirse sunucu ilk kaydı 200 ile döner.
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

/** Where a queued photo goes: a receipt (default) or a document task (#200). */
export type QueueTarget = { type: 'task'; taskUuid: string };

/** The server's answer to a task photo (subset of `DocumentTaskUploadResult`). */
export type TaskUploadResult = { uploadUuid: string; matched: boolean };

/** A further photo of one document photographed in parts (parça 2, 3, …). */
export type QueuePart = { fileUri: string; mimeType: string };

/** At most this many photos make one document (the server's limit). */
export const MAX_PARTS = 5;

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
  /** Why the server refused the corrections (not retried). */
  correctionsError?: string | null;
  /** Set for a document task photo (#200); absent = receipt. */
  target?: QueueTarget | null;
  /** Result of a task photo upload. */
  taskUpload?: TaskUploadResult | null;
  /**
   * Parça 2, 3, … of a long Z report sent as one document (Oğuz'un demo geri
   * bildirimi); `fileUri` is parça 1. Absent = one photo.
   */
  extraParts?: QueuePart[];
  /**
   * Set once a failed item was put back in the queue by `reviveCutUploads`
   * (#780), so an item that really is refused is not revived again.
   */
  revived?: boolean;
  /** epoch ms the server took the photo (#780 Senaryo B: "Müşavirinize ulaştı · 14:32"). */
  uploadedAt?: number;
  /**
   * Why the last attempt failed, as a code (#780 Senaryo B): the server's
   * error code (NO_ACTIVE_ENGAGEMENT, UPLOAD_TIMEOUT, …), `HTTP_<status>`
   * when it gave none, or `NETWORK` with no response. Shown as a reason.
   */
  errorCode?: string | null;
};

export type FailureKind = 'network' | 'server' | 'auth' | 'unsupported' | 'rejected';

const SECOND = 1000;
const MINUTE = 60 * SECOND;
export const BLOCKED_RETRY_MS = 6 * 60 * MINUTE;

export function newItem(input: {
  id: string;
  taxpayer: string;
  kind: ReceiptKind;
  fileUri: string;
  mimeType: string;
  now: number;
  target?: QueueTarget | null;
  extraParts?: QueuePart[];
}): QueueItem {
  const extra = (input.extraParts ?? []).slice(0, MAX_PARTS - 1);
  return {
    ...(extra.length ? { extraParts: extra } : null),
    target: input.target ?? null,
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

/** Shown while an upload that was cut on a slow connection waits for its retry. */
export const CUT_UPLOAD_MESSAGE = 'Bağlantı yavaş olduğu için fotoğraf yarıda kaldı; kendiliğinden yeniden gönderilecek.';

/**
 * The server stopped receiving the photo before it had all of it (#780).
 * New servers answer 408 UPLOAD_TIMEOUT; older ones answered 400
 * VALIDATION_ERROR "multipart form with an image is required …" because the
 * cut body could not be parsed. The app always sends a well-formed form, so
 * that 400 means a cut transfer, not a bad request.
 */
export function isCutUpload(status: number, code?: string, message?: string): boolean {
  if (status === 408) return true;
  return status === 400 && code === 'VALIDATION_ERROR' && /^multipart form/i.test(message ?? '');
}

/**
 * Classifies an HTTP status (0 = no response), error code and the server's
 * message into a failure kind.
 */
export function classify(status: number, code?: string, message?: string): FailureKind {
  if (status === 0 || status === 429 || status >= 500) return status === 0 ? 'network' : 'server';
  if (isCutUpload(status, code, message)) return 'server';
  if (status === 401) return 'auth';
  if (status === 404 || status === 405 || status === 501) return 'unsupported';
  // The company has no active office yet: keep the photo until it has one.
  if (status === 409 && code === 'NO_ACTIVE_ENGAGEMENT') return 'unsupported';
  return 'rejected';
}

/** What to do after sending corrections failed with this status (0 = no response). */
export function correctionOutcome(status: number): 'retry' | 'drop' {
  return status === 0 || status === 401 || status === 429 || status >= 500 ? 'retry' : 'drop';
}

/** Corrections were sent, or refused for good (`error`). */
export function markCorrections(item: QueueItem, error: string | null = null): QueueItem {
  return { ...item, correctionsSent: true, correctionsError: error };
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

export function markUploaded(item: QueueItem, result: ReceiptResult, now: number = Date.now()): QueueItem {
  return { ...item, status: 'uploaded', result, lastError: null, errorCode: null, nextAttemptAt: 0, uploadedAt: now };
}

/** A document task photo reached the server (#200). */
export function markTaskUploaded(item: QueueItem, result: TaskUploadResult, now: number = Date.now()): QueueItem {
  return { ...item, status: 'uploaded', taskUpload: result, lastError: null, errorCode: null, nextAttemptAt: 0, uploadedAt: now };
}

/** Receipt photos only (task photos are shown on their task). */
export function receiptItems(items: readonly QueueItem[]): QueueItem[] {
  return items.filter((i) => !i.target);
}

/** Queued photos of one task, oldest first. */
export function taskItems(items: readonly QueueItem[], taxpayer: string, taskUuid: string): QueueItem[] {
  return items
    .filter((i) => i.taxpayer === taxpayer && i.target?.type === 'task' && i.target.taskUuid === taskUuid)
    .sort((a, b) => a.createdAt - b.createdAt);
}

/** The code stored with a failure: the server's code, else `HTTP_<status>`, else `NETWORK`. */
export function failureCode(status: number, code?: string): string {
  if (status === 0) return 'NETWORK';
  return code && code !== 'NETWORK_ERROR' ? code : `HTTP_${status}`;
}

export function markFailed(item: QueueItem, kind: FailureKind, message: string, now: number, code: string | null = null): QueueItem {
  const attempts = item.attempts + 1;
  const base = { ...item, attempts, lastError: message, errorCode: code };
  switch (kind) {
    case 'network':
    case 'server':
      return { ...base, status: 'pending', nextAttemptAt: now + backoffMs(attempts) };
    case 'auth':
      return { ...base, status: 'pending', nextAttemptAt: now + 30 * SECOND };
    case 'unsupported':
      return { ...base, status: 'blocked', nextAttemptAt: now + BLOCKED_RETRY_MS };
    case 'rejected':
      return { ...base, status: 'failed', nextAttemptAt: 0 };
  }
}

/** Manual retry from the list. */
export function retry(item: QueueItem): QueueItem {
  return { ...item, status: 'pending', nextAttemptAt: 0 };
}

/** An item interrupted mid-upload (app killed) goes back to pending on the next launch. */
export function recover(items: readonly QueueItem[]): QueueItem[] {
  return reviveCutUploads(items.map((i) => (i.status === 'uploading' ? { ...i, status: 'pending', nextAttemptAt: 0 } : i)));
}

/**
 * The message an older app build stored for an upload the server cut
 * (400 VALIDATION_ERROR without details, #780).
 */
const OLD_CUT_UPLOAD_ERROR = 'Girilen bilgileri kontrol edin.';

/**
 * Puts back in the queue, once, the photos an older build gave up on after
 * a cut upload (#780): they never reached the server, so the office never
 * saw them. A photo that is refused again stays failed (`revived`).
 */
export function reviveCutUploads(items: readonly QueueItem[]): QueueItem[] {
  return items.map((i) =>
    i.status === 'failed' && !i.revived && !i.target && i.lastError === OLD_CUT_UPLOAD_ERROR
      ? { ...i, status: 'pending', nextAttemptAt: 0, lastError: null, revived: true }
      : i,
  );
}

/** Failed receipt photos of this company (the user can send them all again). */
export function failedItems(items: readonly QueueItem[], taxpayer: string): QueueItem[] {
  return items.filter((i) => i.taxpayer === taxpayer && i.status === 'failed');
}

export function addCorrection(item: QueueItem, c: Correction): QueueItem {
  const rest = item.corrections.filter((x) => x.field !== c.field);
  return { ...item, corrections: [...rest, c], correctionsSent: false, correctionsError: null };
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

/** One file of an upload: its multipart field and the photo on the device. */
export type UploadFile = { field: string; fileUri: string; mimeType: string; name: string };

/**
 * The files to post for an item, in order: parça 1 under `field`, then
 * parça 2… as `image_2`… (the server joins them into one document).
 */
export function uploadFiles(item: QueueItem, field: string): UploadFile[] {
  const ext = (m: string) => m.split('/')[1] ?? 'jpg';
  const out: UploadFile[] = [{ field, fileUri: item.fileUri, mimeType: item.mimeType, name: `${item.id}.${ext(item.mimeType)}` }];
  (item.extraParts ?? []).forEach((p, i) => {
    out.push({ field: `image_${i + 2}`, fileUri: p.fileUri, mimeType: p.mimeType, name: `${item.id}-${i + 2}.${ext(p.mimeType)}` });
  });
  return out;
}

/** Every photo the item keeps on the device (removed with the item). */
export function itemPhotos(item: QueueItem): string[] {
  return [item.fileUri, ...(item.extraParts ?? []).map((p) => p.fileUri)];
}
