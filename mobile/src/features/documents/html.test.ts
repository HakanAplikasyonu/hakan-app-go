import assert from 'node:assert/strict';
import { test } from 'node:test';

import { gorselSayfasi, pdfSayfasi, PDFJS_SURUM } from './html.ts';

test('PDF sayfası sabit pdf.js sürümünü ve gömülü veriyi kullanır', () => {
  const html = pdfSayfasi('JVBERi0xLjQK');
  assert.match(html, new RegExp(`pdfjs-dist@${PDFJS_SURUM.replace(/\./g, '\\.')}/build/pdf\\.min\\.js`));
  assert.ok(html.includes("atob('JVBERi0xLjQK')"));
  assert.ok(html.includes('isEvalSupported: false'));
  assert.ok(html.includes('user-scalable=yes'));
});

test('base64 dışı karakter sayfaya girmez', () => {
  assert.throws(() => pdfSayfasi("x');alert(1);//"));
  assert.throws(() => gorselSayfasi('a"b', 'image/png'));
});

test('görsel sayfası türü denetler', () => {
  assert.ok(gorselSayfasi('AAAA', 'image/png').includes('data:image/png;base64,AAAA'));
  assert.ok(gorselSayfasi('AAAA', 'text/html"><script>').includes('data:image/jpeg;base64,AAAA'));
});
