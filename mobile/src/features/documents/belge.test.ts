import assert from 'node:assert/strict';
import { test } from 'node:test';

import { ApiError, unwrap } from '../../core/api/envelope.ts';
import {
  base64,
  belgeHataMetni,
  belgeHatasi,
  belgeMime,
  belgeTuru,
  cokBuyuk,
  cokBuyukHatasi,
  dosyaBoyutu,
  gecerliBelgeYolu,
  gecerliYerelDosya,
  guvenliDosyaAdi,
  onbellekAdi,
  paylasimAdi,
  silinecekler,
} from './belge.ts';

test('belge türü: sunucu türü önce, genel türde dosya adı', () => {
  assert.equal(belgeTuru('application/pdf'), 'pdf');
  assert.equal(belgeTuru('application/pdf; charset=binary'), 'pdf');
  assert.equal(belgeTuru('IMAGE/JPEG'), 'image');
  assert.equal(belgeTuru('image/heic'), 'image');
  assert.equal(belgeTuru('application/octet-stream', 'vergi-levhasi.PDF'), 'pdf');
  assert.equal(belgeTuru(null, 'fis.png'), 'image');
  assert.equal(belgeTuru('', 'tablo.xlsx'), 'other');
  assert.equal(belgeTuru('application/vnd.openxmlformats-officedocument.spreadsheetml.sheet', 'x.xlsx'), 'other');
  assert.equal(belgeTuru(undefined, undefined), 'other');
  assert.equal(belgeMime('application/octet-stream', 'a.jpeg'), 'image/jpeg');
});

test('önbellek adı: güvenli karakterler ve uzantı', () => {
  assert.equal(onbellekAdi('bordro', 'ab-12', 'Ekim 2026.PDF'), 'bordro-ab-12.pdf');
  assert.equal(onbellekAdi('arsiv', '../x', null, 'image/jpeg'), 'arsiv-x.jpg');
  assert.equal(onbellekAdi('istek', 'u', 'adsiz', 'application/octet-stream'), 'istek-u');
  assert.equal(onbellekAdi('fatura', 'u', null, 'application/pdf'), 'fatura-u.pdf');
  assert.equal(onbellekAdi('', '', null, null), 'belge');
});

test('paylaşım adı: dosya adı yoksa başlıktan', () => {
  assert.equal(paylasimAdi('Ekim bordrosu', 'bordro-2026-10.pdf', 'application/pdf'), 'bordro-2026-10.pdf');
  assert.equal(paylasimAdi('Ekim bordrosu', null, 'application/pdf'), 'Ekim bordrosu.pdf');
  assert.equal(paylasimAdi('Fiş: A/B', '', 'image/png'), 'Fiş A B.png');
  assert.equal(paylasimAdi('', null, null), 'Belge');
  assert.equal(paylasimAdi('x', '../gizli/a.pdf', 'application/pdf'), '_gizli_a.pdf');
  assert.equal(guvenliDosyaAdi('İmza sirküleri\u0000.pdf'), 'İmza sirküleri.pdf');
  assert.equal(guvenliDosyaAdi('...'), '');
});

test('boyut ve sınır', () => {
  assert.equal(dosyaBoyutu(512), '512 B');
  assert.equal(dosyaBoyutu(1536), '1,5 KB');
  assert.equal(dosyaBoyutu(null), '');
  assert.equal(cokBuyuk(31 * 1024 * 1024), true);
  assert.equal(cokBuyuk(5 * 1024 * 1024), false);
  assert.equal(cokBuyuk(null), false);
});

test('indirme yolu yalnızca mükellef API yolu', () => {
  assert.equal(gecerliBelgeYolu('/v1/taxpayer/archive-documents/0f6c-1/file'), true);
  assert.equal(gecerliBelgeYolu('/v1/taxpayer/document-tasks/a/uploads/b/file'), true);
  assert.equal(gecerliBelgeYolu('/v1/tenant/engagements/x/file'), false);
  assert.equal(gecerliBelgeYolu('@evil.example/v1/taxpayer/x'), false);
  assert.equal(gecerliBelgeYolu('/v1/taxpayer//evil.example'), false);
  assert.equal(gecerliBelgeYolu('/v1/taxpayer/x?y=1'), false);
  assert.equal(gecerliBelgeYolu(undefined), false);
  assert.equal(gecerliYerelDosya('file:///data/user/0/app/files/receipts/a.jpg'), true);
  assert.equal(gecerliYerelDosya('https://x/a.jpg'), false);
  assert.equal(gecerliYerelDosya('file:///data/../etc/passwd'), false);
});

test('hata metinleri: 401, 403, 404, ağ, büyük dosya, sunucu', () => {
  const zarf = (status: number, body?: unknown) => {
    try {
      unwrap(status, body);
    } catch (e) {
      return e;
    }
    throw new Error('unwrap did not throw');
  };
  assert.deepEqual(belgeHatasi(zarf(401)), { code: 'UNAUTHENTICATED', message: 'Oturumunuzun süresi doldu. Çıkış yapıp yeniden giriş yapın.' });
  assert.equal(belgeHatasi(zarf(403)).code, 'FORBIDDEN');
  assert.match(belgeHatasi(zarf(403)).message, /yetkiniz yok/);
  // Sunucu kodu korunur, metin Türkçe.
  const nf = belgeHatasi(zarf(404, { success: false, error: { code: 'NOT_FOUND', message: 'archive document not found' } }));
  assert.equal(nf.code, 'NOT_FOUND');
  assert.match(nf.message, /bulunamadı/);
  assert.equal(belgeHatasi(zarf(404)).code, 'NOT_FOUND');
  const uye = belgeHatasi(zarf(403, { success: false, error: { code: 'NO_TAXPAYER_MEMBERSHIP', message: 'no' } }));
  assert.deepEqual(uye, { code: 'NO_TAXPAYER_MEMBERSHIP', message: 'Bu şirkete erişiminiz yok.' });

  const ag = belgeHatasi(new ApiError({ status: 0, code: 'NETWORK_ERROR', message: 'Network request failed' }));
  assert.equal(ag.code, 'NETWORK_ERROR');
  assert.match(ag.message, /bağlantınızı/i);
  assert.equal(belgeHatasi(new ApiError({ status: 0, code: 'TIMEOUT', message: '' })).code, 'TIMEOUT');

  assert.equal(belgeHatasi(cokBuyukHatasi(40 * 1024 * 1024)).code, 'FILE_TOO_LARGE');
  assert.equal(belgeHatasi(zarf(413, { success: false, error: { code: 'UPLOAD_TOO_LARGE', message: 'x' } })).code, 'UPLOAD_TOO_LARGE');
  assert.match(belgeHatasi(zarf(413)).message, /büyük/);

  const s = belgeHatasi(zarf(503, { success: false, error: { code: 'STORAGE_UNAVAILABLE', message: 'x' } }));
  assert.equal(s.code, 'STORAGE_UNAVAILABLE');
  assert.match(s.message, /Sunucu/);
  assert.equal(belgeHatasi(zarf(502)).code, 'HTTP_502');

  assert.deepEqual(belgeHatasi(new Error('boom')), { code: 'UNEXPECTED', message: 'Belge açılamadı. Yeniden deneyin.' });
  assert.equal(belgeHataMetni(zarf(404)), 'Belge bulunamadı. Müşaviriniz belgeyi kaldırmış ya da paylaşımı kapatmış olabilir. (Hata kodu: NOT_FOUND)');
});

test('önbellek temizliği: eski dosyalar, sonra boyut sınırı; açık belge kalır', () => {
  const now = 10 * 86_400_000;
  const gun = 86_400_000;
  const files = [
    { name: 'eski.pdf', size: 10, modified: now - 3 * gun },
    { name: 'acik.pdf', size: 60, modified: now - 5 * gun },
    { name: 'a.pdf', size: 40, modified: now - 3_600_000 },
    { name: 'b.pdf', size: 40, modified: now - 60_000 },
    { name: 'zamansiz.jpg', size: 5, modified: null },
  ];
  assert.deepEqual(silinecekler(files, now, { maxAgeMs: gun, maxBytes: 1000, keep: 'acik.pdf' }).sort(), ['eski.pdf']);
  // 60 (açık) + 40 + 40 + 5 = 145 > 110: önce zamansız, sonra en eski (a).
  assert.deepEqual(silinecekler(files, now, { maxAgeMs: gun, maxBytes: 110, keep: 'acik.pdf' }).sort(), ['a.pdf', 'eski.pdf', 'zamansiz.jpg']);
  assert.deepEqual(silinecekler([], now, { maxAgeMs: gun, maxBytes: 0 }), []);
});

test('base64 Buffer ile aynı', () => {
  for (const n of [0, 1, 2, 3, 4, 5, 12287, 12288, 12289, 40000]) {
    const bytes = new Uint8Array(n);
    for (let i = 0; i < n; i++) bytes[i] = (i * 31 + 7) & 255;
    assert.equal(base64(bytes), Buffer.from(bytes).toString('base64'), `n=${n}`);
  }
});
