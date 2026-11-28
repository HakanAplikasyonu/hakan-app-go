import assert from 'node:assert/strict';
import { test } from 'node:test';

import { gorselSayfasi, pdfSayfasi, PDFJS_SRI, PDFJS_SURUM } from './html.ts';

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

test('pdf.js betikleri SRI ile, CSP ağ isteğini ve Worker\'ı kapatır', () => {
  const html = pdfSayfasi('AAAA');
  const scripts = [...html.matchAll(/<script src="[^"]+"[^>]*>/g)].map((m) => m[0]);
  assert.equal(scripts.length, 2);
  for (const tag of scripts) {
    assert.match(tag, /integrity="sha(256|384|512)-[A-Za-z0-9+/=]+"/);
    assert.match(tag, /crossorigin="anonymous"/);
  }
  assert.ok(html.includes(`integrity="${PDFJS_SRI.lib}"`));
  assert.ok(html.includes(`integrity="${PDFJS_SRI.worker}"`));
  assert.ok(!html.includes('workerSrc'));
  assert.match(html, /http-equiv="Content-Security-Policy"/);
  assert.match(html, /connect-src 'none'/);
  assert.match(html, /worker-src 'none'/);
  assert.match(html, /script-src 'unsafe-inline' https:\/\/cdn\.jsdelivr\.net\/npm\/pdfjs-dist@3\.11\.174\/build\/;/);
  assert.match(gorselSayfasi('AAAA', 'image/png'), /img-src data:; connect-src 'none'/);
});
