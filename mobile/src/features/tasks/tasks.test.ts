import assert from 'node:assert/strict';
import { test } from 'node:test';

import { markFailed, markTaskUploaded, markUploaded, newItem, receiptItems, taskItems } from '../receipts/queue.ts';
import { bekleyenIstekMetni, bekleyenIstekSayisi, donemEtiketi, isBankTask, localTasks, mergeTasks, moneyText, serverTasks, taskStatusLabel, taskTitle, transactionLine, type ServerTask } from './tasks.ts';

const item = (id: string) => newItem({ id, taxpayer: 't', kind: 'expense_receipt', fileUri: 'f', mimeType: 'image/jpeg', now: 1 });

test('failed uploads and unsure OCR fields become tasks', () => {
  const failed = markFailed(item('a'), 'rejected', 'Dosya çok büyük', 1);
  const unsure = markUploaded(item('b'), {
    uuid: 'r',
    status: 'draft',
    kind: 'expense_receipt',
    duplicate_of: null,
    error: null,
    fields: { total_amount: { raw: '1', value: '1.00', confidence: 0.5, valid: true } },
  });
  const fine = item('c');
  const tasks = localTasks([failed, unsure, fine], 't');
  assert.deepEqual(tasks.map((t) => t.id), ['failed:a', 'check:b']);
  assert.equal(tasks[1].detail, 'Toplam emin olunamadı.');
  assert.deepEqual(tasks[1].action, { type: 'receipt', id: 'b' });
});

const task = (uuid: string, over: Partial<ServerTask> = {}): ServerTask => ({
  uuid,
  code: 'BNK_UNDOCUMENTED_PAYMENT',
  kind: 'undocumented_payment',
  period: '2026-10',
  status: 'open',
  title: '',
  note: '',
  due_at: '2026-11-20T21:00:00Z',
  overdue: false,
  submitted_at: null,
  matched_at: null,
  bank_transaction: {
    uuid: 'b' + uuid,
    transaction_date: '2026-10-14',
    direction: 'out',
    amount: '12500.00',
    currency: 'TRY',
    counterparty_name: 'Kuzey Gıda',
    description: 'EFT',
  },
  upload_count: 0,
  created_at: '2026-11-01T00:00:00Z',
  updated_at: '2026-11-01T00:00:00Z',
  ...over,
});

test('document tasks (#200): open and submitted ones lead the list by due date', () => {
  const s = serverTasks([
    task('2', { due_at: '2026-12-01T00:00:00Z', kind: 'uninvoiced_collection', code: 'BNK_UNINVOICED_COLLECTION' }),
    task('1', { note: 'Kira faturası', overdue: true }),
    task('3', { status: 'matched' }),
    task('4', { status: 'submitted' }),
  ]);
  const merged = mergeTasks(s, [{ id: 'l', source: 'local', title: 'x', detail: '', action: { type: 'receipt', id: 'a' }, due: null }]);
  assert.deepEqual(merged.map((t) => t.id), ['server:1', 'server:4', 'server:2', 'l']);
  assert.deepEqual(merged[0].action, { type: 'task', uuid: '1' });
  assert.equal(merged[0].title, 'Ödeme için belge');
  assert.equal(merged[0].overdue, true);
  assert.equal(merged[0].detail, '12.500,00 TRY · Kuzey Gıda · 2026-10-14\nKira faturası');
  assert.equal(merged[1].detail.split('\n')[1], 'Gönderildi, müşavir kontrol ediyor');
  assert.equal(merged[2].title, 'Tahsilat için fatura');
  assert.equal(merged[0].due, '2026-11-20');
});

test('transaction line falls back to the description and formats the amount', () => {
  const t = task('x', { bank_transaction: { ...task('x').bank_transaction, counterparty_name: null, amount: '-1250.5' } });
  assert.equal(transactionLine(t), '1.250,50 TRY · EFT · 2026-10-14');
  assert.equal(moneyText('abc', 'TRY'), 'abc TRY');
  assert.equal(taskStatusLabel('matched'), 'Tamamlandı');
});

test('task photos are not local tasks or receipts, and are listed on their task', () => {
  const photo = newItem({ id: 'p', taxpayer: 't', kind: 'expense_receipt', fileUri: 'f', mimeType: 'image/jpeg', now: 2, target: { type: 'task', taskUuid: '1' } });
  const failedPhoto = markFailed(photo, 'rejected', 'UNSUPPORTED_FILE', 3);
  assert.deepEqual(localTasks([failedPhoto], 't'), []);
  assert.deepEqual(receiptItems([failedPhoto, item('r')]).map((i) => i.id), ['r']);
  assert.deepEqual(taskItems([failedPhoto, item('r')], 't', '1').map((i) => i.id), ['p']);
  assert.deepEqual(taskItems([failedPhoto], 't', 'other'), []);
  const done = markTaskUploaded(photo, { uploadUuid: 'u1', matched: true });
  assert.equal(done.status, 'uploaded');
  assert.deepEqual(done.taskUpload, { uploadUuid: 'u1', matched: true });
  assert.equal(done.result, null);
});

test('office-opened request (#279): title from the request, no bank line', () => {
  const req = task('r', {
    code: 'DOC_REQUEST',
    kind: 'document_request',
    title: 'Ekim kira sözleşmesi',
    note: 'İmzalı sayfası eksik',
    bank_transaction: { ...task('r').bank_transaction, amount: '0', counterparty_name: null, description: 'Ekim kira sözleşmesi' },
  });
  assert.equal(isBankTask(req), false);
  const [s] = serverTasks([req]);
  assert.equal(s.title, 'Ekim kira sözleşmesi');
  assert.equal(s.detail, 'İmzalı sayfası eksik');
  const [sub] = serverTasks([{ ...req, status: 'submitted' }]);
  assert.equal(sub.detail, 'Gönderildi, müşavir kontrol ediyor');
});

test('task titles: bank kinds by name, unknown kinds get a generic title', () => {
  assert.equal(taskTitle({ kind: 'undocumented_payment', title: 'yok sayılır' }), 'Ödeme için belge');
  assert.equal(taskTitle({ kind: 'uninvoiced_collection', title: '' }), 'Tahsilat için fatura');
  assert.equal(taskTitle({ kind: 'document_request', title: '  ' }), 'Eksik belge');
  assert.equal(taskTitle({ kind: 'something_new', title: '' }), 'Eksik belge');
  assert.equal(taskTitle({ kind: 'something_new', title: 'Vekaletname' }), 'Vekaletname');
  assert.equal(isBankTask({ kind: 'toString' }), false);
});

test('belge isteği dönemi Türkçe ay adıyla yazılır (#780)', () => {
  assert.equal(donemEtiketi('2026-10'), 'Ekim 2026');
  assert.equal(donemEtiketi('2026-01'), 'Ocak 2026');
  assert.equal(donemEtiketi('2026-13'), '2026-13');
  assert.equal(donemEtiketi(''), '');
});

test('ana sayfa kartı yalnız açık istekleri sayar, 0 ise gizlenir (#780)', () => {
  const list: Pick<ServerTask, 'status'>[] = [{ status: 'open' }, { status: 'open' }, { status: 'submitted' }, { status: 'matched' }, { status: 'cancelled' }];
  assert.equal(bekleyenIstekSayisi(list), 2);
  assert.equal(bekleyenIstekSayisi(null), 0);
  assert.equal(bekleyenIstekMetni(2), '2 belge isteği bekliyor');
  assert.equal(bekleyenIstekMetni(0), null);
});
