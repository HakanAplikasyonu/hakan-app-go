// App Store meta verisi (EAS Metadata, `npx eas-cli metadata:push`), #183 / #203.
//
// Metin buradadır; mağazaya özgü dış bilgiler (adresler, inceleme hesabı)
// ortamdan gelir. Ayarlanmamış bir değer yer tutucu olarak kalır ve
// `missingStoreValues()` listesinde görünür; gönderimden önce hepsi dolmalı.
// Liste: store/README.md, "Eksik dış bilgiler".
const PLACEHOLDER = 'TBD';

// [env adı, yer tutucu]
const external = {
  marketingUrl: ['STORE_MARKETING_URL', 'https://example.com/hakan'],
  supportUrl: ['STORE_SUPPORT_URL', 'https://example.com/hakan/destek'],
  privacyPolicyUrl: ['STORE_PRIVACY_URL', 'https://example.com/hakan/gizlilik'],
  reviewFirstName: ['STORE_REVIEW_FIRST_NAME', PLACEHOLDER],
  reviewLastName: ['STORE_REVIEW_LAST_NAME', PLACEHOLDER],
  reviewEmail: ['STORE_REVIEW_EMAIL', 'review@example.com'],
  reviewPhone: ['STORE_REVIEW_PHONE', '+90 000 000 00 00'],
  demoUsername: ['STORE_DEMO_USERNAME', 'review@example.com'],
  demoPassword: ['STORE_DEMO_PASSWORD', PLACEHOLDER],
};

function value(key) {
  const [name, fallback] = external[key];
  const v = process.env[name];
  return v && v.trim() ? v.trim() : fallback;
}

/** Env names still on their placeholder (example.com / TBD). */
function missingStoreValues() {
  return Object.keys(external)
    .filter((k) => value(k) === external[k][1])
    .map((k) => external[k][0]);
}

const config = {
  configVersion: 0,
  apple: {
    copyright: '2026 HAKAN',
    categories: ['FINANCE', 'BUSINESS'],
    info: {
      'tr-TR': {
        title: 'Hakan Mükellef',
        subtitle: 'Mali müşavirinizle bağlantı',
        description:
          'Hakan Mükellef, mali müşavirinizle çalışan şirketler için mobil uygulamadır.\n\n' +
          '• Fiş ve Z raporu: Kamerayla fotoğraf çekin ya da galeriden seçin; belge müşavirinizin ofisine gider ve inceleme durumunu buradan izlersiniz.\n' +
          '• Görevler: Müşavirinizin sizden beklediği belgeleri ve son tarihleri tek listede görün.\n' +
          '• Birden çok şirket: Yetkili olduğunuz şirketler arasında geçiş yapın.\n' +
          '• Bildirimler: Yeni görev ve belge durumu değişikliklerinden haberdar olun.\n\n' +
          'Uygulamayı kullanmak için mali müşavirinizin Hakan platformunu kullanması ve sizi şirketinizin mükellef paneline davet etmesi gerekir.',
        keywords: ['mali müşavir', 'fiş', 'Z raporu', 'muhasebe', 'fatura', 'mükellef'],
        releaseNotes: 'İlk sürüm: fiş ve Z raporu gönderme, görevler, bildirimler.',
        marketingUrl: value('marketingUrl'),
        supportUrl: value('supportUrl'),
        privacyPolicyUrl: value('privacyPolicyUrl'),
      },
    },
    review: {
      firstName: value('reviewFirstName'),
      lastName: value('reviewLastName'),
      email: value('reviewEmail'),
      phone: value('reviewPhone'),
      demoUsername: value('demoUsername'),
      demoPassword: value('demoPassword'),
      demoRequired: true,
      notes: 'İnceleme hesabı canlı ortamda açılır (bkz. store/README.md).',
    },
  },
};

module.exports = config;
module.exports.missingStoreValues = missingStoreValues;
module.exports.storeEnv = Object.fromEntries(Object.entries(external).map(([k, [name]]) => [k, name]));
