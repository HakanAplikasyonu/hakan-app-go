import assert from 'node:assert/strict';
import { test } from 'node:test';

import { markFailed, markUploaded, newItem } from '../receipts/queue.ts';
import { localTasks, mergeTasks, serverTasks } from './tasks.ts';

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

test('server tasks sort by due date and lead the list', () => {
  const s = serverTasks([
    { uuid: '2', kind: 'missing_document', title: 'Ekim Z raporları', due_date: null, document_kind: 'z_report' },
    { uuid: '1', kind: 'missing_document', title: 'Akaryakıt fişi', due_date: '2026-11-20', document_kind: 'fuel_receipt' },
  ]);
  const merged = mergeTasks(s, [{ id: 'l', source: 'local', title: 'x', detail: '', action: { type: 'receipt', id: 'a' }, due: null }]);
  assert.deepEqual(merged.map((t) => t.id), ['server:1', 'server:2', 'l']);
  assert.deepEqual(merged[0].action, { type: 'capture', kind: 'fuel_receipt' });
});
