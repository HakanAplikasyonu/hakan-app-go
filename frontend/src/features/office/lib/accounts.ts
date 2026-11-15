/**
 * Tek Düzen Hesap Planı'ndan sık kullanılan hesaplar. Kod değiştirme
 * seçicisi için başlangıç listesi; gerçek liste ofisin hedef hesap planından
 * (#30) gelecek.
 */
export type Account = { code: string; name: string };

export const commonAccounts: Account[] = [
  { code: "100.01", name: "Kasa" },
  { code: "102.01", name: "Bankalar" },
  { code: "120.01", name: "Alıcılar" },
  { code: "153.01", name: "Ticari mallar" },
  { code: "191.01", name: "İndirilecek KDV" },
  { code: "253.01", name: "Tesis, makine ve cihazlar" },
  { code: "254.01", name: "Taşıtlar" },
  { code: "255.01", name: "Demirbaşlar" },
  { code: "320.01", name: "Satıcılar" },
  { code: "360.01", name: "Ödenecek vergi ve fonlar" },
  { code: "391.01", name: "Hesaplanan KDV" },
  { code: "600.01", name: "Yurt içi satışlar" },
  { code: "601.01", name: "Yurt dışı satışlar" },
  { code: "610.01", name: "Satıştan iadeler" },
  { code: "760.01", name: "Pazarlama, satış ve dağıtım giderleri" },
  { code: "770.01.001", name: "Kira giderleri" },
  { code: "770.01.003", name: "Kırtasiye giderleri" },
  { code: "770.01.004", name: "Enerji ve iletişim giderleri" },
  { code: "770.01.005", name: "Taşıt yakıt giderleri" },
  { code: "770.01.006", name: "Temizlik giderleri" },
  { code: "770.01.008", name: "Danışmanlık giderleri" },
  { code: "770.01.010", name: "Yazılım ve lisans giderleri" },
  { code: "780.01", name: "Finansman giderleri" },
];

export function accountName(code: string): string | undefined {
  return commonAccounts.find((a) => a.code === code)?.name;
}
