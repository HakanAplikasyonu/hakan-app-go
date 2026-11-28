import assert from 'node:assert/strict';
import { test } from 'node:test';

import { belgeBolumleri, belgeEtiketi, gecerlilikMetni } from './liste.ts';

test('etiket: önemli belge türü, değilse kategori', () => {
  assert.equal(belgeEtiketi({ category: 'tax', important_kind: 'tax_certificate' }), 'Vergi levhası');
  assert.equal(belgeEtiketi({ category: 'contract', important_kind: null }), 'Sözleşme');
  assert.equal(belgeEtiketi({ category: 'yeni', important_kind: null }), 'Belge');
});

test('geçerlilik metni', () => {
  assert.equal(gecerlilikMetni({ expiry_status: 'expiring', days_left: 12 }), '12 gün kaldı');
  assert.equal(gecerlilikMetni({ expiry_status: 'expiring', days_left: 0 }), 'Bugün bitiyor');
  assert.equal(gecerlilikMetni({ expiry_status: 'expired', days_left: -3 }), 'Süresi geçti (3 gün)');
  assert.equal(gecerlilikMetni({ expiry_status: 'valid', days_left: 200 }), '');
  assert.equal(gecerlilikMetni({ expiry_status: 'none', days_left: null }), '');
});

test('bölümler: önemli üstte, tekrar yok, boş bölüm yok', () => {
  const a = { uuid: 'a' };
  const b = { uuid: 'b' };
  const c = { uuid: 'c' };
  assert.deepEqual(belgeBolumleri([a], [a, b, c]), [
    { title: 'Önemli belgeler', data: [a] },
    { title: 'Paylaşılan belgeler', data: [b, c] },
  ]);
  assert.deepEqual(belgeBolumleri([], [b]), [{ title: 'Paylaşılan belgeler', data: [b] }]);
  assert.deepEqual(belgeBolumleri([a], [a]), [{ title: 'Önemli belgeler', data: [a] }]);
  assert.deepEqual(belgeBolumleri([], []), []);
});
