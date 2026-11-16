import { Notice } from '@/components/ui';

import { useReceipts } from './provider';

/** Kuyruğun durumu: bağlantı yok, bekleyen, şimdilik gönderilemeyen fişler. */
export function QueueBanner() {
  const { online, summary } = useReceipts();
  if (!online && summary.waiting + summary.blocked > 0) {
    return <Notice tone="info">Bağlantı yok. {summary.waiting + summary.blocked} fiş telefonda saklanıyor, bağlantı gelince gönderilecek.</Notice>;
  }
  if (!online) return <Notice tone="info">Bağlantı yok. Çektiğiniz fişler telefonda saklanır, bağlantı gelince gönderilir.</Notice>;
  if (summary.blocked > 0) {
    return (
      <Notice tone="info">
        {summary.blocked} fiş telefonda bekliyor: şirketinizin bağlı bir müşavirlik ofisi yok ya da sunucu şu an fiş kabul etmiyor. Ofis bağlanınca kendiliğinden gönderilecek.
      </Notice>
    );
  }
  if (summary.waiting > 0) return <Notice tone="info">{summary.waiting} fiş gönderiliyor.</Notice>;
  return null;
}
