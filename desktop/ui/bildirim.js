// Bildirim kuralları (#325, tuval AjanTepsi):
//   - Başarılı gönderimde bildirim yok.
//   - Ağ hatası 30 dk sürerse tek bildirim; bağlantı dönünce sıfırlanır.
//   - Kalıcı hatalar (UBL değil, okunamadı…) gün sonunda tek özet.
// Saf modül: DOM ya da Tauri kullanmaz; app.js her durum yenilemesinde
// `degerlendir` çağırır ve dönen bildirimleri gösterir. Birim testi:
// tests/bildirim.test.mjs.

export const AG_ESIGI_MS = 30 * 60 * 1000;
/** Gün sonu özeti bu saatten sonra (yerel saat) bir kez gönderilir. */
export const OZET_SAATI = 18;

const gun = (d) => `${d.getFullYear()}-${d.getMonth() + 1}-${d.getDate()}`;

export function yeniDurum() {
  return { agSorunuBasladi: null, agBildirildi: false, kalici: {}, ozetGunu: null };
}

/**
 * @param {ReturnType<typeof yeniDurum>} s  değiştirilir
 * @param {{ agSorunu: boolean, kaliciHatalar: { anahtar: string }[] }} g
 * @param {Date} simdi
 * @returns {{ baslik: string, govde: string }[]}
 */
export function degerlendir(s, g, simdi = new Date()) {
  const out = [];
  const t = simdi.getTime();

  if (g.agSorunu) {
    if (s.agSorunuBasladi == null) s.agSorunuBasladi = t;
    if (!s.agBildirildi && t - s.agSorunuBasladi >= AG_ESIGI_MS) {
      s.agBildirildi = true;
      const dk = Math.floor((t - s.agSorunuBasladi) / 60000);
      out.push({
        baslik: `HAKAN · ${dk} dakikadır bağlantı yok`,
        govde: "Dosyalar bilgisayarda bekliyor, kaybolmaz. İnternet gelince gönderilir.",
      });
    }
  } else {
    s.agSorunuBasladi = null;
    s.agBildirildi = false;
  }

  const bugun = gun(simdi);
  for (const h of g.kaliciHatalar) if (!(h.anahtar in s.kalici)) s.kalici[h.anahtar] = bugun;

  if (simdi.getHours() >= OZET_SAATI && s.ozetGunu !== bugun) {
    const yeni = Object.entries(s.kalici).filter(([, d]) => d === bugun).length;
    s.ozetGunu = bugun;
    if (yeni > 0) {
      out.push({
        baslik: `HAKAN · bugün ${yeni} dosya gönderilemedi`,
        govde: "Bu dosyalar e-Fatura olarak okunamadı. Ayrıntılar Sorunlar sekmesinde.",
      });
    }
  }
  // Eski günlerin anahtarları bir hafta sonra unutulur (liste şişmesin).
  const sinir = t - 7 * 86400000;
  for (const [k, d] of Object.entries(s.kalici)) {
    const [y, m, dd] = d.split("-").map(Number);
    if (new Date(y, m - 1, dd).getTime() < sinir) delete s.kalici[k];
  }
  return out;
}
