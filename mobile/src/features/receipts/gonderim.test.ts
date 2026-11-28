import assert from 'node:assert/strict';
import { test } from 'node:test';

import {
  asamaEtiketi,
  bekleyisNedeni,
  gonderimSatirlari,
  kuyrukAsamasi,
  sunucuAsamasi,
  sunucuNedeni,
  takiliMi,
  teslimDurumu,
  type SunucuGonderimi,
} from './gonderim.ts';
import { classify, failureCode, markFailed, markTaskUploaded, markUploaded, markUploading, newItem, type ReceiptResult } from './queue.ts';

const T = Date.UTC(2026, 10, 20, 11, 30); // 20 Kas 14:30 TSİ
const item = (id = 'q1', taxpayer = 'tp1') =>
  newItem({ id, taxpayer, kind: 'fuel_receipt', fileUri: `file:///${id}.jpg`, mimeType: 'image/jpeg', now: T });
const result = (uuid: string, status: ReceiptResult['status'] = 'draft'): ReceiptResult => ({
  uuid,
  status,
  kind: 'fuel_receipt',
  fields: { vendor_name: { raw: 'PETROL', value: 'Örnek Petrol', confidence: 0.9, valid: true } },
  duplicate_of: null,
  error: null,
});
const server = (p: Partial<SunucuGonderimi>): SunucuGonderimi => ({
  uuid: 'r1',
  source: 'receipt',
  kind: 'fuel_receipt',
  title: '',
  status: 'received',
  reason: '',
  media_type: 'image/jpeg',
  vendor_name: 'Örnek Petrol',
  total_amount: '120.00',
  office_name: 'Deneme Müşavirlik',
  task_uuid: null,
  created_at: new Date(T).toISOString(),
  updated_at: new Date(T).toISOString(),
  ...p,
});
const fail = (status: number, code?: string) => {
  const i = item();
  return markFailed(i, classify(status, code), 'Sunucu hatası.', T, failureCode(status, code));
};

test('failure codes are stored with the failure', () => {
  assert.equal(failureCode(0), 'NETWORK');
  assert.equal(failureCode(409, 'NO_ACTIVE_ENGAGEMENT'), 'NO_ACTIVE_ENGAGEMENT');
  assert.equal(failureCode(404), 'HTTP_404');
  assert.equal(failureCode(0, 'NETWORK_ERROR'), 'NETWORK');
  assert.equal(fail(409, 'NO_ACTIVE_ENGAGEMENT').errorCode, 'NO_ACTIVE_ENGAGEMENT');
  assert.equal(markUploaded(fail(500), result('r1'), T + 5).errorCode, null);
});

test('status mapping: queue and server', () => {
  assert.equal(kuyrukAsamasi(item()), 'telefonda');
  assert.equal(kuyrukAsamasi(markUploading(item())), 'gonderiliyor');
  assert.equal(kuyrukAsamasi(markUploaded(item(), result('r1'))), 'ulasti');
  assert.equal(kuyrukAsamasi(markUploaded(item(), result('r1', 'failed'))), 'okunamadi');
  assert.equal(kuyrukAsamasi(markUploaded(item(), result('r1', 'duplicate'))), 'mukerrer');
  assert.equal(sunucuAsamasi('received'), 'ulasti');
  assert.equal(sunucuAsamasi('approved'), 'onaylandi');
  assert.equal(sunucuAsamasi('rejected'), 'reddedildi');
  assert.equal(sunucuAsamasi('unreadable'), 'okunamadi');
  assert.equal(sunucuAsamasi('cancelled'), 'kapatildi');
  assert.equal(asamaEtiketi('ulasti'), 'Müşavirinize ulaştı');
});

test('reason messages tell the two blocked causes apart (#780)', () => {
  assert.match(bekleyisNedeni(fail(409, 'NO_ACTIVE_ENGAGEMENT'), true)!, /bağlı bir müşavirlik ofisi yok/);
  assert.match(bekleyisNedeni(fail(404), true)!, /Sunucu şu an fiş kabul etmiyor/);
  assert.match(bekleyisNedeni(fail(0), false)!, /İnternet bağlantısı yok/);
  assert.match(bekleyisNedeni(fail(408, 'UPLOAD_TIMEOUT'), true)!, /yarıda kaldı/);
  assert.match(bekleyisNedeni(fail(503), true)!, /yoğun ya da ulaşılamıyor/);
  assert.match(bekleyisNedeni(fail(401), true)!, /Oturumunuz yenileniyor/);
  const refused = markFailed(item(), 'rejected', 'Fotoğraf çok büyük (en çok 10 MB). Yeniden çekin.', T, 'UPLOAD_TOO_LARGE');
  assert.equal(bekleyisNedeni(refused, true), 'Fotoğraf çok büyük (en çok 10 MB). Yeniden çekin. Tekrar deneyin ya da kuyruktan kaldırın.');
  assert.equal(bekleyisNedeni(item(), true), null);
  assert.equal(bekleyisNedeni(item(), false), 'İnternet bağlantısı yok. Bağlanınca kendiliğinden gönderilecek.');
  // Old items without a code: still a sensible reason.
  const old = { ...fail(409, 'NO_ACTIVE_ENGAGEMENT'), errorCode: undefined };
  assert.match(bekleyisNedeni(old, true)!, /Sunucu şu an/);
});

test('stuck items offer retry / remove', () => {
  assert.equal(takiliMi(item()), false);
  assert.equal(takiliMi(fail(500)), true);
  assert.equal(takiliMi(fail(404)), true);
  assert.equal(takiliMi(fail(422)), true);
  assert.equal(takiliMi(markUploaded(item(), result('r1'))), false);
});

test('server reasons: office words first, else a plain explanation', () => {
  assert.equal(sunucuNedeni({ status: 'received', reason: '', source: 'receipt' }), null);
  assert.match(sunucuNedeni({ status: 'rejected', reason: '', source: 'receipt' })!, /kabul etmedi/);
  assert.equal(sunucuNedeni({ status: 'rejected', reason: 'Tarih okunmuyor', source: 'document_request' }), 'Tarih okunmuyor');
  assert.match(sunucuNedeni({ status: 'rejected', reason: ' ', source: 'document_request' })!, /yeniden istedi/);
  assert.match(sunucuNedeni({ status: 'unreadable', reason: '', source: 'receipt' })!, /okunamadı/);
  assert.equal(sunucuNedeni({ status: 'cancelled', reason: 'Gerek kalmadı', source: 'document_request' }), 'Gerek kalmadı');
});

test('rows merge the phone queue with the server, newest first', () => {
  const sent = markUploaded(item('a'), result('r1'), T + 1000);
  const waiting = fail(0);
  const other = item('x', 'tp2');
  const task = markTaskUploaded(
    newItem({ id: 't', taxpayer: 'tp1', kind: 'expense_receipt', fileUri: 'file:///t.jpg', mimeType: 'image/jpeg', now: T - 60_000, target: { type: 'task', taskUuid: 'task-1' } }),
    { uploadUuid: 'u1', matched: false },
  );
  const rows = gonderimSatirlari(
    [sent, waiting, other, task],
    [
      server({ uuid: 'r1', status: 'rejected' }),
      server({ uuid: 'u1', source: 'document_request', title: 'Ekim dökümü', status: 'approved', task_uuid: 'task-1', media_type: 'application/pdf', created_at: new Date(T - 60_000).toISOString() }),
      server({ uuid: 'r9', status: 'approved', vendor_name: '', kind: 'z_report', created_at: new Date(T - 3_600_000).toISOString() }),
    ],
    'tp1',
    true,
  );
  assert.deepEqual(
    rows.map((r) => [r.key, r.asama]),
    [
      ['q:a', 'reddedildi'],
      ['q:q1', 'telefonda'],
      ['q:t', 'onaylandi'],
      ['s:r9', 'onaylandi'],
    ],
  );
  const [rejected, phone, doc, onlyServer] = rows;
  assert.equal(rejected.baslik, 'Örnek Petrol');
  assert.match(rejected.alt, /Deneme Müşavirlik/);
  assert.match(rejected.neden!, /kabul etmedi/);
  assert.equal(rejected.belge?.localUri, 'file:///a.jpg');
  assert.equal(phone.takili, true);
  assert.doesNotMatch(phone.alt, /Deneme/);
  assert.equal(doc.baslik, 'Ekim dökümü');
  assert.equal(onlyServer.baslik, 'Z raporu');
  assert.equal(onlyServer.yerelId, null);
  assert.equal(onlyServer.belge?.path, '/v1/taxpayer/receipts/r9/image');
  const docOnly = gonderimSatirlari([], [server({ uuid: 'u2', source: 'document_request', task_uuid: 'k', title: 'Kira' })], 'tp1', true);
  assert.equal(docOnly[0].belge?.path, '/v1/taxpayer/document-tasks/k/uploads/u2/file');
});

test('confirmation: reached with office and time, or queued with the reason', () => {
  const up = markUploaded(item(), result('r1'), T);
  const ok = teslimDurumu([up], true, 'Deneme Müşavirlik');
  assert.equal(ok.durum, 'ulasti');
  assert.equal(ok.baslik, 'Müşavirinize ulaştı ✓');
  assert.match(ok.aciklama, /^Deneme Müşavirlik · 20 Kas/);
  assert.match(ok.aciklama, /14:30/);
  assert.match(teslimDurumu([up], true, '').aciklama, /^Müşavir ofisiniz/);

  const offline = teslimDurumu([item()], false, 'X');
  assert.equal(offline.durum, 'sirada');
  assert.equal(offline.baslik, 'Telefonda sırada, bağlanınca gönderilecek');
  assert.match(offline.aciklama, /İnternet bağlantısı yok/);

  const noOffice = teslimDurumu([fail(409, 'NO_ACTIVE_ENGAGEMENT')], true, null);
  assert.equal(noOffice.durum, 'sirada');
  assert.match(noOffice.aciklama, /müşavirlik ofisi yok/);

  assert.equal(teslimDurumu([fail(422)], true, null).durum, 'gonderilemedi');
  assert.equal(teslimDurumu([markUploading(item())], true, null).durum, 'gonderiliyor');
  assert.match(teslimDurumu([item('a'), item('b')], false, null).aciklama, /^2 fotoğraf/);
});
