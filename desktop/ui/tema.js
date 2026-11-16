// Tema seçimi (#324): <html data-theme="m3-light|m3-dark|n3-light|n3-dark">.
// Varsayılan M3 + "sistemi izle". Kullanıcı seçimi localStorage'da tutulur
// (ayar ekranı #325 ile gelir). Modül olarak erken yüklenir (index.html).
const PALET = 'hakan.palet';
const KIP = 'hakan.kip';
const mq = window.matchMedia('(prefers-color-scheme: dark)');

function oku(anahtar, gecerli, varsayilan) {
  try {
    const v = localStorage.getItem(anahtar);
    return gecerli.includes(v) ? v : varsayilan;
  } catch {
    return varsayilan;
  }
}

export function temaUygula() {
  const palet = oku(PALET, ['m3', 'n3'], 'm3');
  const kip = oku(KIP, ['light', 'dark', 'system'], 'system');
  const koyu = kip === 'dark' || (kip === 'system' && mq.matches);
  document.documentElement.dataset.theme = `${palet}-${koyu ? 'dark' : 'light'}`;
}

/** palet: 'm3' | 'n3'; kip: 'light' | 'dark' | 'system'. */
export function temaSec({ palet, kip } = {}) {
  try {
    if (palet) localStorage.setItem(PALET, palet);
    if (kip) localStorage.setItem(KIP, kip);
  } catch {
    // depo kapalıysa seçim yalnızca bu oturumda geçerli
  }
  temaUygula();
}

mq.addEventListener('change', temaUygula);
temaUygula();
