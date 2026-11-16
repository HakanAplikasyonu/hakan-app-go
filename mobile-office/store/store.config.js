// App Store meta verisi (EAS Metadata, `npx eas-cli metadata:push`), #242.
// Kalıp: mobile/store/store.config.js (#203).
//
// Metin buradadır; mağazaya özgü dış bilgiler (adresler, inceleme hesabı)
// ortamdan gelir. Ayarlanmamış değer yer tutucu kalır ve `missingStoreValues()`
// listesinde görünür; gönderimden önce hepsi dolmalı (store/README.md, #207).
const PLACEHOLDER = 'TBD';

// [env adı, yer tutucu]. Mükellef uygulamasından ayrı adlar: inceleme hesabı
// bir ofis kullanıcısıdır.
const external = {
  marketingUrl: ['STORE_OFFICE_MARKETING_URL', 'https://example.com/hakan-ofis'],
  supportUrl: ['STORE_OFFICE_SUPPORT_URL', 'https://example.com/hakan/destek'],
  privacyPolicyUrl: ['STORE_OFFICE_PRIVACY_URL', 'https://example.com/hakan/gizlilik'],
  reviewFirstName: ['STORE_OFFICE_REVIEW_FIRST_NAME', PLACEHOLDER],
  reviewLastName: ['STORE_OFFICE_REVIEW_LAST_NAME', PLACEHOLDER],
  reviewEmail: ['STORE_OFFICE_REVIEW_EMAIL', 'review@example.com'],
  reviewPhone: ['STORE_OFFICE_REVIEW_PHONE', '+90 000 000 00 00'],
  demoUsername: ['STORE_OFFICE_DEMO_USERNAME', 'review-ofis@example.com'],
  demoPassword: ['STORE_OFFICE_DEMO_PASSWORD', PLACEHOLDER],
};

function value(key) {
  const [name, fallback] = external[key];
  const v = process.env[name];
  return v && v.trim() ? v.trim() : fallback;
}

/** Hâlâ yer tutucuda olan env adları (example.com / TBD). */
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
        title: 'Hakan Ofis',
        subtitle: 'Mali müşavir ofisi, cebinizde',
        description:
          'Hakan Ofis, Hakan platformunu kullanan mali müşavirlik ofislerinin müşavir ve çalışanları için mobil uygulamadır.\n\n' +
          '• Pano: İnceleme kuyruğu, açık belge görevleri, aktif uyarılar ve bugün gelen yüklemeler tek ekranda.\n' +
          '• İnceleme kuyruğu: Önerilen hesap kodunu belgesiyle ve gerekçesiyle görün; onaylayın, düzeltin ya da reddedin. Kaydırarak ya da toplu onay.\n' +
          '• Mükellefler: Arama ve süzgeçle mükellef listesi; iletişim, dönem durumu, son fiş ve faturalar.\n' +
          '• Belge talepleri: Mükelleften eksik belgeyi isteyin, yüklenince inceleyin, gerekirse yeniden isteyin.\n' +
          '• Bildirimler: Mükellef belge yüklediğinde ve kritik uyarı açıldığında haberiniz olsun.\n\n' +
          'Uygulamayı kullanmak için ofisinizin Hakan hesabında üye olmanız gerekir. Mükellef hesapları için “Hakan” uygulamasını kullanın.',
        keywords: ['mali müşavir', 'muhasebe', 'SMMM', 'mükellef', 'hesap kodu', 'beyanname'],
        releaseNotes: 'İlk sürüm: pano, inceleme kuyruğu, mükellefler, belge talepleri ve bildirimler.',
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
      notes: 'İnceleme hesabı canlı ortamda demo ofisin üyesi olarak açılır (bkz. store/README.md).',
    },
  },
};

module.exports = config;
module.exports.missingStoreValues = missingStoreValues;
module.exports.storeEnv = Object.fromEntries(Object.entries(external).map(([k, [name]]) => [k, name]));
