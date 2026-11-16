import assert from 'node:assert/strict';
import { test } from 'node:test';

import { hasPermission, initials, isSelectable, officeAccess, pickOffice, sortOffices, statusLabel, type OfficeOption } from './offices.ts';

const office = (slug: string, extra: Partial<OfficeOption> = {}): OfficeOption => ({
  uuid: `uuid-${slug}`,
  slug,
  name: slug.toUpperCase(),
  status: 'active',
  access_ends_at: null,
  role: { key: 'member', name: 'Üye' },
  permissions: [],
  ...extra,
});

const NOW = Date.parse('2026-11-16T12:00:00Z');

test('only office members get in; taxpayer accounts are told apart', () => {
  assert.equal(officeAccess({ organizations: [office('a')] }), 'office');
  assert.equal(officeAccess({ organizations: [office('a')], taxpayers: [{}] }), 'office');
  assert.equal(officeAccess({ organizations: [], taxpayers: [{}] }), 'taxpayerOnly');
  assert.equal(officeAccess({ taxpayers: [{}] }), 'taxpayerOnly');
  assert.equal(officeAccess({}), 'none');
});

test('suspended and expired offices are not selectable', () => {
  assert.equal(isSelectable(office('a'), NOW), true);
  assert.equal(isSelectable(office('a', { status: 'suspended' }), NOW), false);
  assert.equal(isSelectable(office('a', { access_ends_at: '2026-11-01T00:00:00Z' }), NOW), false);
  assert.equal(isSelectable(office('a', { access_ends_at: '2027-01-01T00:00:00Z' }), NOW), true);
  assert.equal(statusLabel(office('a', { status: 'suspended' }), NOW), 'Askıda');
  assert.equal(statusLabel(office('a'), NOW), null);
});

test('pickOffice prefers last used, then the only open office', () => {
  const list = [office('a'), office('b')];
  assert.equal(pickOffice(list, 'b', NOW), 'b');
  assert.equal(pickOffice(list, 'uuid-a', NOW), 'a');
  assert.equal(pickOffice(list, 'gone', NOW), null);
  assert.equal(pickOffice([office('a')], null, NOW), 'a');
  assert.equal(pickOffice([office('a'), office('b', { status: 'suspended' })], null, NOW), 'a');
  assert.equal(pickOffice([office('a', { status: 'suspended' })], 'a', NOW), null);
});

test('sortOffices puts open offices first, Turkish order', () => {
  const list = [office('z'), office('c', { status: 'suspended' }), office('ç')];
  assert.deepEqual(sortOffices(list, NOW).map((o) => o.slug), ['ç', 'z', 'c']);
});

test('initials and permissions', () => {
  assert.equal(initials('Yıldız Mali Müşavirlik'), 'YM');
  assert.equal(initials('SMMM Ayşe Kaya'), 'AK');
  assert.equal(initials(''), '?');
  assert.equal(hasPermission(['tenant.review.approve'], 'tenant.review.approve'), true);
  assert.equal(hasPermission(undefined, 'x'), false);
});
