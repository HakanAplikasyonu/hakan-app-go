import assert from 'node:assert/strict';
import { test } from 'node:test';

import { helpGuideSlug, helpGuideUrl } from './help.ts';

test('sahip ve yönetici ofis yöneticisi kılavuzunu açar', () => {
  assert.equal(helpGuideSlug('owner'), 'ofis-yoneticisi');
  assert.equal(helpGuideSlug('admin'), 'ofis-yoneticisi');
});

test('diğer roller ofis çalışanı kılavuzunu açar', () => {
  for (const role of ['accountant', 'intern', 'member', undefined]) assert.equal(helpGuideSlug(role), 'ofis-calisani');
});

test('adres boşsa bağlantı yok; sondaki eğik çizgi yinelenmez', () => {
  assert.equal(helpGuideUrl('', 'owner'), null);
  assert.equal(helpGuideUrl('https://app.example.com/yardim/', 'intern'), 'https://app.example.com/yardim/ofis-calisani');
});
