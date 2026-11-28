import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  addCorrection,
  backoffMs,
  BLOCKED_RETRY_MS,
  classify,
  correctionOutcome,
  markCorrections,
  correctionsDue,
  CUT_UPLOAD_MESSAGE,
  failedItems,
  fileExt,
  isCutUpload,
  itemPhotos,
  reviveCutUploads,
  MAX_PARTS,
  uploadFiles,
  due,
  markFailed,
  markRead,
  markUploaded,
  markUploading,
  newItem,
  READ_FOLLOW_MS,
  readingItems,
  recover,
  retry,
  summarize,
  type ReceiptResult,
} from './queue.ts';
import { statusLabel } from './fields.ts';

const T = 1_000_000;
const item = (id: string, at: number, taxpayer = 'tp1') =>
  newItem({ id, taxpayer, kind: 'expense_receipt', fileUri: `file:///${id}.jpg`, mimeType: 'image/jpeg', now: at });
const ok: ReceiptResult = { uuid: 'r1', status: 'draft', kind: 'expense_receipt', fields: {}, duplicate_of: null, error: null };

test('a receipt shot offline waits, then uploads when the network returns', () => {
  let a = item('a', T);
  // offline: the request never reached the server
  a = markFailed(markUploading(a), 'network', 'Network request failed', T + 1);
  assert.equal(a.status, 'pending');
  assert.equal(a.nextAttemptAt, T + 1 + 30_000);
  assert.deepEqual(due([a], 'tp1', T + 2), []);
  // network back: the sync runs forced, regardless of the backoff
  assert.deepEqual(due([a], 'tp1', T + 2, { force: true }).map((i) => i.id), ['a']);
  a = markUploaded(markUploading(a), ok);
  assert.equal(a.status, 'uploaded');
  assert.equal(a.result?.uuid, 'r1');
  assert.deepEqual(due([a], 'tp1', T + 3, { force: true }), []);
});

test('backoff doubles and is capped', () => {
  assert.equal(backoffMs(1), 30_000);
  assert.equal(backoffMs(2), 60_000);
  assert.equal(backoffMs(3), 120_000);
  assert.equal(backoffMs(20), 30 * 60_000);
});

test('status classification', () => {
  assert.equal(classify(0), 'network');
  assert.equal(classify(503), 'server');
  assert.equal(classify(429), 'server');
  assert.equal(classify(401), 'auth');
  assert.equal(classify(404), 'unsupported');
  assert.equal(classify(405), 'unsupported');
  assert.equal(classify(400), 'rejected');
  assert.equal(classify(413), 'rejected');
});

test('missing server endpoint keeps the photo on the phone', () => {
  const a = markFailed(item('a', T), 'unsupported', 'HTTP_404', T);
  assert.equal(a.status, 'blocked');
  assert.equal(a.nextAttemptAt, T + BLOCKED_RETRY_MS);
  assert.deepEqual(due([a], 'tp1', T + 1, { force: true }).map((i) => i.id), ['a']);
});

test('a rejected photo stops until the user retries', () => {
  const a = markFailed(item('a', T), 'rejected', 'too large', T);
  assert.equal(a.status, 'failed');
  assert.deepEqual(due([a], 'tp1', T + 10, { force: true }), []);
  assert.deepEqual(due([retry(a)], 'tp1', T + 10).map((i) => i.id), ['a']);
});

test('only the active company, oldest first', () => {
  const list = [item('b', T + 5), item('a', T), item('x', T, 'tp2')];
  assert.deepEqual(due(list, 'tp1', T + 10).map((i) => i.id), ['a', 'b']);
});

test('an interrupted upload is retried after a restart', () => {
  const [a] = recover([markUploading(item('a', T))]);
  assert.equal(a.status, 'pending');
});

test('corrections replace per field and wait for upload', () => {
  let a = item('a', T);
  a = addCorrection(a, { field: 'total_amount', value: '100.00' });
  a = addCorrection(a, { field: 'total_amount', value: '120.00' });
  assert.deepEqual(a.corrections, [{ field: 'total_amount', value: '120.00' }]);
  assert.deepEqual(correctionsDue([a], 'tp1'), []); // not uploaded yet
  a = markUploaded(a, ok);
  assert.deepEqual(correctionsDue([a], 'tp1').map((i) => i.id), ['a']);
  assert.deepEqual(correctionsDue([{ ...a, correctionsSent: true }], 'tp1'), []);
});

test('summary counts per company', () => {
  const list = [
    item('a', T),
    markFailed(item('b', T), 'unsupported', '', T),
    markFailed(item('c', T), 'rejected', '', T),
    markUploaded(item('d', T), ok),
    item('e', T, 'tp2'),
  ];
  assert.deepEqual(summarize(list, 'tp1'), { waiting: 1, blocked: 1, failed: 1, uploaded: 1 });
});

test('classify: the receipt endpoint answers (#199)', () => {
  assert.equal(classify(409, 'NO_ACTIVE_ENGAGEMENT'), 'unsupported');
  assert.equal(classify(409, 'CONFLICT'), 'rejected');
  assert.equal(classify(503), 'server');
  assert.equal(classify(405), 'unsupported');
  assert.equal(classify(400, 'VALIDATION_ERROR'), 'rejected');
});

test('corrections refused by the server are not retried; network errors are', () => {
  assert.equal(correctionOutcome(0), 'retry');
  assert.equal(correctionOutcome(503), 'retry');
  assert.equal(correctionOutcome(401), 'retry');
  assert.equal(correctionOutcome(400), 'drop');
  assert.equal(correctionOutcome(409), 'drop');
  let a = markUploaded(item('c', T), ok);
  a = addCorrection(a, { field: 'total_amount', value: 'abc' });
  a = markCorrections(a, 'total_amount value "abc" is not valid');
  assert.equal(a.correctionsSent, true);
  assert.deepEqual(correctionsDue([a], 'tp1'), []);
  // A new correction clears the refusal and is sent again.
  a = addCorrection(a, { field: 'total_amount', value: '1.250,00' });
  assert.equal(a.correctionsError, null);
  assert.deepEqual(correctionsDue([a], 'tp1').map((i) => i.id), ['c']);
});

test('a long Z report in parts goes as one upload: image, image_2, image_3', () => {
  const z = newItem({
    id: 'z1',
    taxpayer: 'tp1',
    kind: 'z_report',
    fileUri: 'file:///z1.jpg',
    mimeType: 'image/jpeg',
    now: T,
    extraParts: [
      { fileUri: 'file:///z1-2.png', mimeType: 'image/png' },
      { fileUri: 'file:///z1-3.jpg', mimeType: 'image/jpeg' },
    ],
  });
  assert.deepEqual(
    uploadFiles(z, 'image').map((f) => [f.field, f.fileUri, f.name]),
    [
      ['image', 'file:///z1.jpg', 'z1.jpeg'],
      ['image_2', 'file:///z1-2.png', 'z1-2.png'],
      ['image_3', 'file:///z1-3.jpg', 'z1-3.jpeg'],
    ],
  );
  assert.deepEqual(itemPhotos(z), ['file:///z1.jpg', 'file:///z1-2.png', 'file:///z1-3.jpg']);
  // A single photo stays as before: no parts, one file.
  const one = item('a', T);
  assert.equal(one.extraParts, undefined);
  assert.deepEqual(uploadFiles(one, 'image').map((f) => f.field), ['image']);
  // At most MAX_PARTS photos make one document.
  const many = newItem({
    id: 'z2',
    taxpayer: 'tp1',
    kind: 'z_report',
    fileUri: 'file:///z2.jpg',
    mimeType: 'image/jpeg',
    now: T,
    extraParts: Array.from({ length: 8 }, (_, i) => ({ fileUri: `file:///z2-${i}.jpg`, mimeType: 'image/jpeg' })),
  });
  assert.equal(uploadFiles(many, 'image').length, MAX_PARTS);
});

// #780: on a slow mobile network the server stopped reading the photo after
// 10 s and answered 400 "multipart form with an image is required"; the queue
// took that as final, the receipt stayed "failed" on the phone and never
// reached the office.
test('a cut upload (408 or the old 400 multipart answer) is retried, not given up', () => {
  assert.equal(classify(408, 'UPLOAD_TIMEOUT'), 'server');
  assert.equal(classify(400, 'VALIDATION_ERROR', 'multipart form with an image is required (max 10 MB per photo)'), 'server');
  assert.equal(isCutUpload(400, 'VALIDATION_ERROR', 'multipart form with an image is required (max 10 MB)'), true);
  // a real validation error stays final
  assert.equal(classify(400, 'VALIDATION_ERROR', 'image must be JPEG, PNG, WebP or GIF (got text/plain)'), 'rejected');
  assert.equal(classify(413, 'UPLOAD_TOO_LARGE'), 'rejected');
  assert.equal(isCutUpload(400, 'VALIDATION_ERROR'), false);

  let a = item('a', T);
  a = markFailed(markUploading(a), classify(408, 'UPLOAD_TIMEOUT'), CUT_UPLOAD_MESSAGE, T + 1);
  assert.equal(a.status, 'pending');
  assert.equal(a.lastError, CUT_UPLOAD_MESSAGE);
  assert.deepEqual(due([a], 'tp1', T + 1 + 30_000).map((i) => i.id), ['a']);
});

test('photos an older build gave up on after a cut upload go back to the queue once', () => {
  const cut = { ...markFailed(item('a', T), 'rejected', 'Girilen bilgileri kontrol edin.', T + 1) };
  const other = markFailed(item('b', T + 1), 'rejected', 'Fotoğraf çok büyük (en çok 10 MB). Yeniden çekin.', T + 2);
  const [a, b] = recover([cut, other]);
  assert.equal(a.status, 'pending');
  assert.equal(a.nextAttemptAt, 0);
  assert.equal(a.lastError, null);
  assert.equal(a.revived, true);
  assert.equal(b.status, 'failed');
  // refused again: it stays failed, no endless revive
  const again = markFailed(a, 'rejected', 'Girilen bilgileri kontrol edin.', T + 3);
  assert.equal(reviveCutUploads([again])[0].status, 'failed');
});

test('failedItems lists only this company\'s failed photos', () => {
  const a = markFailed(item('a', T), 'rejected', 'x', T);
  const b = markFailed(item('b', T, 'tp2'), 'rejected', 'x', T);
  const c = item('c', T);
  assert.deepEqual(failedItems([a, b, c], 'tp1').map((i) => i.id), ['a']);
  assert.deepEqual(failedItems([a, b, c], 'tp1').map(retry).map((i) => i.status), ['pending']);
});

test('belge isteğine eklenen ekstre dosyası doğru uzantıyla gönderilir', () => {
  assert.equal(fileExt('image/jpeg'), 'jpeg');
  assert.equal(fileExt('image/heic'), 'heic');
  assert.equal(fileExt('application/pdf'), 'pdf');
  assert.equal(fileExt('application/vnd.openxmlformats-officedocument.spreadsheetml.sheet'), 'xlsx');
  assert.equal(fileExt('application/vnd.ms-excel'), 'xls');
  assert.equal(fileExt('text/csv'), 'csv');
  assert.equal(fileExt('text/plain'), 'txt');
  assert.equal(fileExt('application/octet-stream'), 'bin');
});

// #780: the server takes the photo with 202 "processing" and reads it in the
// background; the app follows the read and then shows the read fields.
test('a receipt accepted for a background read is sent and followed until read', () => {
  const item = newItem({ id: 'bg', taxpayer: 'tp', kind: 'fuel_receipt', fileUri: 'file:///bg.jpg', mimeType: 'image/jpeg', now: 1 });
  const processing: ReceiptResult = { uuid: 'r-bg', status: 'processing', kind: 'fuel_receipt', fields: {}, duplicate_of: null, error: null };
  const sent = markUploaded(item, processing, 1_000);
  assert.equal(sent.status, 'uploaded');
  assert.equal(statusLabel(sent), 'Okunuyor');
  assert.deepEqual(readingItems([sent], 'tp', 2_000).map((i) => i.id), ['bg']);
  assert.deepEqual(readingItems([sent], 'other', 2_000), []);
  // Followed for READ_FOLLOW_MS after the upload, then left to the server lists.
  assert.deepEqual(readingItems([sent], 'tp', 1_000 + READ_FOLLOW_MS), []);
  const read = markRead(sent, { ...processing, status: 'draft', fields: { total_amount: { raw: '10,00', value: '10.00', confidence: 0.9, valid: true } } });
  assert.equal(read.status, 'uploaded');
  assert.equal(read.uploadedAt, 1_000);
  assert.equal(statusLabel(read), 'Müşavir onayında');
  assert.deepEqual(readingItems([read], 'tp', 2_000), []);
});
