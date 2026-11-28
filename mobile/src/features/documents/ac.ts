import { router } from 'expo-router';

import type { BelgeKaynagi } from './belge';

/** Açılacak belge: kaynak + önbellek adı için önek ve kimlik. */
export type AcilacakBelge = BelgeKaynagi & { id: string; prefix: string };

/**
 * Belgeyi uygulama içi görüntüleyicide açar (#780). Bütün belge listeleri
 * (bordro, önemli ve paylaşılan belgeler, istek yüklemeleri, fatura PDF'i,
 * fiş fotoğrafı) bu yoldan açılır.
 */
export function belgeyiAc(b: AcilacakBelge): void {
  router.push({
    pathname: '/belge',
    params: {
      title: b.title,
      path: b.path ?? '',
      localUri: b.localUri ?? '',
      fileName: b.fileName ?? '',
      contentType: b.contentType ?? '',
      size: b.size != null ? String(b.size) : '',
      id: b.id,
      prefix: b.prefix,
    },
  });
}
