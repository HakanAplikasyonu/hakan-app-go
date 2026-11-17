import assert from 'node:assert/strict';
import { test } from 'node:test';

import { idleLabel, mobileTarget, nextStepOf, openFirst, progressLabel, stepDetail, stepLabels, type Row, type Step } from './format.ts';

const steps = (over: Partial<Record<Step['key'], Partial<Step>>> = {}): Step[] =>
  (['documents', 'bank', 'review', 'journals', 'transfer', 'report'] as const).map((key) => ({
    key,
    status: 'done',
    count: 0,
    action: null,
    ...over[key],
  }));

test('step detail fills the count in Turkish format', () => {
  assert.equal(stepDetail('review', 'todo', 1250), '1.250 öneri onay bekliyor');
  assert.equal(stepDetail('report', 'done', 0), 'Aylık rapor gönderildi');
  assert.equal(stepDetail('transfer', 'waiting', 3), 'Aktarım masaüstü ajanında bekliyor');
});

test('every step has a Turkish label, no raw keys', () => {
  for (const s of steps()) assert.ok(stepLabels[s.key] && stepLabels[s.key] !== s.key);
});

test('next step is the one the server names', () => {
  const row: Row = { next_step: 'review', steps: steps({ review: { status: 'todo', count: 2, action: 'open_review' } }), done_steps: 2, active_steps: 5 };
  assert.equal(nextStepOf(row)?.action, 'open_review');
  assert.equal(nextStepOf({ ...row, next_step: null }), null);
  assert.equal(progressLabel(row), '2/5 adım tamam');
});

test('idle rows: waiting vs closed', () => {
  assert.equal(idleLabel({ next_step: null, steps: steps({ transfer: { status: 'waiting' } }), done_steps: 4, active_steps: 5 }), 'Sonuç bekleniyor');
  assert.equal(idleLabel({ next_step: null, steps: steps(), done_steps: 6, active_steps: 6 }), 'Dönem kapandı');
});

test('screen actions open the app screen, write actions stay on the web', () => {
  assert.equal(mobileTarget('open_documents'), 'tasks');
  assert.equal(mobileTarget('open_review'), 'review');
  assert.equal(mobileTarget('bank_approve'), 'web');
  assert.equal(mobileTarget('open_journals'), 'web');
  assert.equal(mobileTarget(null), null);
});

test('rows with work come first, order kept', () => {
  const a = { id: 'a', next_step: null, steps: [], done_steps: 0, active_steps: 0 };
  const b = { id: 'b', next_step: 'bank' as const, steps: [], done_steps: 0, active_steps: 1 };
  const c = { id: 'c', next_step: 'report' as const, steps: [], done_steps: 0, active_steps: 1 };
  assert.deepEqual(openFirst([a, b, c]).map((r) => r.id), ['b', 'c', 'a']);
});
