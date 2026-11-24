/**
 * Android'de WebView'e verilen sayfalar (#780). Android WebView PDF
 * çizemez: PDF, pdf.js ile (sürümü sabit, jsDelivr) telefonda çizilir; belge
 * verisi sayfaya gömülüdür, sunucuya ya da CDN'e gönderilmez; betikler SRI ile
 * doğrulanır ve CSP ağ isteklerini kapatır. iOS PDF'i
 * WKWebView ile kendisi gösterir. Saf modül, node --test ile sınanır.
 */

export const PDFJS_SURUM = '3.11.174';
const PDFJS = `https://cdn.jsdelivr.net/npm/pdfjs-dist@${PDFJS_SURUM}/build`;

/**
 * Alt kaynak bütünlüğü (SRI): jsDelivr'in yayımladığı SHA-256 özetleri
 * (data.jsdelivr.com, pdfjs-dist@3.11.174). Dosya CDN'de değişirse tarayıcı
 * betiği çalıştırmaz, sayfa "önizleme yok" der.
 */
export const PDFJS_SRI = {
  lib: 'sha256-W1eZ5vjGgGYyB6xbQu4U7tKkBvp69I9QwVTwwLFWaUY=',
  worker: 'sha256-/qvfMJdw7SS7oxpUZ4Ns3Iz2OccFryfVK1hbBBu4Uns=',
} as const;

/**
 * İçerik güvenlik politikası: betik yalnızca sayfanın kendisi ve sabit pdf.js
 * klasörü; ağ isteği (fetch/XHR), çerçeve ve Worker yok. Belge verisi sayfaya
 * gömülü olduğu için dışarı çıkacak bir yol kalmaz.
 */
export const PDF_CSP = `default-src 'none'; script-src 'unsafe-inline' ${PDFJS}/; style-src 'unsafe-inline'; img-src data: blob:; font-src data: blob:; connect-src 'none'; worker-src 'none'; frame-src 'none'; form-action 'none'; base-uri 'none'`;
export const GORSEL_CSP = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; img-src data:; connect-src 'none'; form-action 'none'; base-uri 'none'";

/** WebView'den gelen iletiler. */
export type SayfaIletisi = 'tamam' | 'hata' | 'pdfjs-yok';

const VIEWPORT = '<meta name="viewport" content="width=device-width, initial-scale=1, minimum-scale=1, maximum-scale=6, user-scalable=yes">';

/** base64 yalnızca A-Z a-z 0-9 + / = içerir; başka karakter sayfaya girmez. */
function guvenliB64(b64: string): string {
  if (!/^[A-Za-z0-9+/=]*$/.test(b64)) throw new Error('invalid base64');
  return b64;
}

/** Görsel: tam ekran, iki parmakla yakınlaştırma (viewport). */
export function gorselSayfasi(b64: string, mime: string): string {
  const tur = /^image\/[a-z0-9.+-]+$/.test(mime) ? mime : 'image/jpeg';
  return `<!doctype html><html><head><meta charset="utf-8">${VIEWPORT}
<meta http-equiv="Content-Security-Policy" content="${GORSEL_CSP}">
<style>html,body{margin:0;height:100%;background:#000}body{display:flex;align-items:center;justify-content:center}img{max-width:100%;max-height:100%;object-fit:contain}</style>
</head><body><img alt="" src="data:${tur};base64,${guvenliB64(b64)}"
onload="window.ReactNativeWebView&&window.ReactNativeWebView.postMessage('tamam')"
onerror="window.ReactNativeWebView&&window.ReactNativeWebView.postMessage('hata')"></body></html>`;
}

/** PDF: bütün sayfalar alt alta, ekran genişliğinde; yakınlaştırma viewport ile. */
export function pdfSayfasi(b64: string): string {
  return `<!doctype html><html><head><meta charset="utf-8">${VIEWPORT}
<meta http-equiv="Content-Security-Policy" content="${PDF_CSP}">
<style>html,body{margin:0;background:#525659}canvas{display:block;margin:8px auto;background:#fff;box-shadow:0 1px 3px rgba(0,0,0,.4)}#m{color:#fff;font:15px sans-serif;text-align:center;padding:32px 16px}</style>
<script src="${PDFJS}/pdf.min.js" integrity="${PDFJS_SRI.lib}" crossorigin="anonymous"></script>
<script src="${PDFJS}/pdf.worker.min.js" integrity="${PDFJS_SRI.worker}" crossorigin="anonymous"></script>
</head><body><div id="m">Belge hazırlanıyor…</div><script>
(function () {
  function post(t) { if (window.ReactNativeWebView) window.ReactNativeWebView.postMessage(t); }
  var lib = window.pdfjsLib;
  // İşçi de SRI'li <script> ile yüklenir ve ana iş parçacığında çalışır;
  // işçi adresi hiç verilmez, yani SRI'siz bir Worker indirmesi olmaz.
  if (!lib || !window.pdfjsWorker) { post('pdfjs-yok'); return; }
  var raw = atob('${guvenliB64(b64)}');
  var data = new Uint8Array(raw.length);
  for (var i = 0; i < raw.length; i++) data[i] = raw.charCodeAt(i);
  raw = null;
  lib.getDocument({ data: data, isEvalSupported: false }).promise.then(function (pdf) {
    var m = document.getElementById('m');
    var width = Math.max(200, document.documentElement.clientWidth - 16);
    var dpr = Math.min(window.devicePixelRatio || 1, 2);
    var chain = Promise.resolve();
    for (var n = 1; n <= pdf.numPages; n++) {
      (function (n) {
        chain = chain.then(function () { return pdf.getPage(n); }).then(function (page) {
          var scale = width / page.getViewport({ scale: 1 }).width;
          var vp = page.getViewport({ scale: scale * dpr });
          var c = document.createElement('canvas');
          c.width = Math.floor(vp.width);
          c.height = Math.floor(vp.height);
          c.style.width = Math.floor(vp.width / dpr) + 'px';
          c.style.height = Math.floor(vp.height / dpr) + 'px';
          document.body.appendChild(c);
          if (n === 1 && m) { m.remove(); post('tamam'); }
          return page.render({ canvasContext: c.getContext('2d'), viewport: vp }).promise;
        });
      })(n);
    }
    return chain;
  }).catch(function () { post('hata'); });
})();
</script></body></html>`;
}
