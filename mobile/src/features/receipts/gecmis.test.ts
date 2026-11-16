import assert from 'node:assert/strict';
import { test } from 'node:test';

import { ayGruplari, sunucuSatiri, tl } from './gecmis.ts';

test('server rows carry crown and tone', () => {
  const r = sunucuSatiri({ uuid: 'a', kind: 'fuel_receipt', status: 'failed', created_at: '2026-10-09T07:00:00Z', total_amount: '2140.00' });
  assert.equal(r.tac, 'sorun');
  assert.equal(r.ton, 'danger');
  assert.equal(r.baslik, 'Yakıt fişi');
  assert.equal(r.tutar, 2140);
  const ok = sunucuSatiri({ uuid: 'b', status: 'approved', vendor_name: 'Kalem Kırtasiye', created_at: '2026-10-10T07:00:00Z' });
  assert.equal(ok.tac, 'onaylandi');
  assert.equal(ok.alt, 'Onaylandı');
  assert.equal(ok.tutar, null);
});

test('month groups newest first with totals', () => {
  const rows = [
    sunucuSatiri({ uuid: 'a', status: 'draft', created_at: '2026-10-09T07:00:00Z', total_amount: '6212.40' }),
    sunucuSatiri({ uuid: 'b', status: 'draft', created_at: '2026-11-02T07:00:00Z', total_amount: '100' }),
    sunucuSatiri({ uuid: 'c', status: 'draft', created_at: '2026-10-31T22:30:00Z' }), // İstanbul'da 1 Kasım
  ];
  const g = ayGruplari(rows);
  assert.deepEqual(g.map((x) => x.baslik), ['Kasım 2026 · 2 fiş · ₺ 100', 'Ekim 2026 · 1 fiş · ₺ 6.212']);
  assert.equal(tl(1533.6), '1.533,60');
});
