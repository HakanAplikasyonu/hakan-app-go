// HAKAN ortak bileşenleri · Tauri (#324). Web (frontend/src/components/hakan)
// ve Expo (packages/mobile-core/src/ui/hakan.tsx) ile aynı ad ve prop'lar.
// Her işlev bir DOM öğesi döndürür; stiller hakan.css'te.

const TAC_ETIKET = {
  bekleniyor: 'Bekleniyor',
  yuklendi: 'Yüklendi · öneri hazır',
  onaylandi: 'Müşavir onayladı',
  aktarildi: "Luca'ya aktarıldı",
  sorun: 'Sorun var',
};

function el(tag, cls, text) {
  const n = document.createElement(tag);
  if (cls) n.className = cls;
  if (text != null) n.textContent = text;
  return n;
}

function kristaller(cls, n = 3) {
  const s = el('span', cls);
  for (let i = 0; i < n; i++) s.append(el('i'));
  return s;
}

/**
 * Taç rozeti. durum: bekleniyor | yuklendi | onaylandi | aktarildi | sorun;
 * boyut: 16 | 30 | 72. Etiket yoksa aria-label taşır (durum yalnız renkle anlatılmaz).
 */
export function TacRozeti({ durum = 'bekleniyor', boyut = 16, etiket = false, metin } = {}) {
  const label = metin ?? TAC_ETIKET[durum] ?? durum;
  const tac = kristaller(`tac${boyut === 30 ? ' b30' : boyut === 72 ? ' b72' : ''}`);
  tac.dataset.durum = durum;
  if (!etiket) {
    tac.setAttribute('role', 'img');
    tac.setAttribute('aria-label', label);
    return tac;
  }
  tac.setAttribute('aria-hidden', 'true');
  const wrap = el('span', 'tac-etiket');
  wrap.dataset.durum = durum;
  wrap.append(tac, el('span', null, label));
  return wrap;
}

/** < .6 düşük, < .9 orta, aksi yüksek; kural eşleşmesi her zaman yüksek. */
export function guvenKademesi(deger, kaynak) {
  if (kaynak === 'kural') return 'yuksek';
  if (deger < 0.6) return 'dusuk';
  if (deger < 0.9) return 'orta';
  return 'yuksek';
}

const KADEME_AD = { dusuk: 'Düşük güven', orta: 'Orta güven', yuksek: 'Yüksek güven' };
const KAYNAK_AD = { kural: 'kural eşleşmesi', gecmis: 'mizan hafızası', model: 'model önerisi', girdi: 'girdi' };

/** AI güven ölçeği. Yüzde yalnızca ipucunda (title) ve ekran okuyucuda. */
export function GuvenOlcegi({ deger = 0, kaynak, etiketli = false } = {}) {
  const kademe = guvenKademesi(deger, kaynak);
  const yuzde = new Intl.NumberFormat('tr-TR', { style: 'percent', maximumFractionDigits: 0 }).format(
    Math.min(1, Math.max(0, deger)),
  );
  const ipucu = [KADEME_AD[kademe], yuzde, kaynak ? KAYNAK_AD[kaynak] : null].filter(Boolean).join(' · ');
  const s = el('span', 'guven');
  s.dataset.kademe = kademe;
  s.setAttribute('role', 'img');
  s.setAttribute('aria-label', ipucu);
  s.title = ipucu;
  const k = kristaller('guven-k');
  k.setAttribute('aria-hidden', 'true');
  s.append(k);
  if (etiketli) {
    const ad = el('span', 'guven-ad', KADEME_AD[kademe]);
    ad.setAttribute('aria-hidden', 'true');
    s.append(ad);
  }
  return s;
}

/**
 * Kristal yükleyici. 400 ms'den kısa işlemlerde görünmez (gecikme).
 * boyut: 40 (varsayılan) ya da 16 (satır içi).
 */
export function KristalYukleyici({ boyut = 40, etiket = 'Yükleniyor…', gecikme = 400 } = {}) {
  const s = kristaller(`yukleyici${boyut <= 16 ? ' kucuk' : ''}`);
  s.setAttribute('role', 'status');
  s.setAttribute('aria-live', 'polite');
  s.setAttribute('aria-label', etiket);
  if (gecikme > 0) {
    s.dataset.gizli = '';
    setTimeout(() => delete s.dataset.gizli, gecikme);
  }
  return s;
}

/** Boş durum: kesik çizgili kristaller, başlık, açıklama ve tek eylem (öğe). */
export function BosDurum({ baslik, aciklama, eylem } = {}) {
  const d = el('div', 'bos-durum');
  const k = kristaller('bos-kristal');
  k.setAttribute('aria-hidden', 'true');
  d.append(k, el('h3', null, baslik));
  if (aciklama) d.append(el('p', null, aciklama));
  if (eylem) d.append(eylem);
  return d;
}

const BULUT_YOK =
  'M2 2l20 20M5.8 5.8A6 6 0 004 10.5 4.5 4.5 0 006.5 19H17m3.6-2.3A4 4 0 0018 9.1h-.5A7 7 0 009.3 4.2';

/**
 * Hata (#332, Durumlar.dc.html): ne oldu + veri güvende mi + ne yapmalı.
 * verilerGuvende yalnızca gerçekten biliniyorsa verilir ("Hiçbir şey
 * kaybolmadı"). kod (ör. NETWORK_TIMEOUT) yalnızca "Ayrıntı"ya basınca görünür.
 */
export function HataDurumu({ baslik = 'Sunucuya ulaşılamıyor', aciklama, verilerGuvende = false, kod, eylem } = {}) {
  const d = el('div', 'bos-durum hata-durumu');
  d.setAttribute('role', 'alert');
  const ikon = el('span', 'durum-ikonu tehlike');
  ikon.setAttribute('aria-hidden', 'true');
  ikon.innerHTML = `<svg viewBox="0 0 24 24" width="28" height="28" fill="none" stroke="currentColor" stroke-width="1.75" stroke-linecap="round" stroke-linejoin="round"><path d="${BULUT_YOK}"/></svg>`;
  d.append(ikon, el('h3', null, baslik));
  const metin = [aciklama, verilerGuvende ? 'Hiçbir şey kaybolmadı.' : null].filter(Boolean).join(' ');
  if (metin) d.append(el('p', null, metin));
  if (eylem) d.append(eylem);
  if (kod) {
    const ayrinti = el('button', 'durum-ayrinti', 'Ayrıntı');
    ayrinti.type = 'button';
    ayrinti.setAttribute('aria-expanded', 'false');
    ayrinti.addEventListener('click', () => {
      const acik = ayrinti.getAttribute('aria-expanded') === 'true';
      ayrinti.setAttribute('aria-expanded', String(!acik));
      ayrinti.textContent = acik ? 'Ayrıntı' : `Ayrıntı · ${kod}`;
    });
    d.append(ayrinti);
  }
  return d;
}
