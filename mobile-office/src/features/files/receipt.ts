import { useRouter } from 'expo-router';
import { useState } from 'react';

import { useOrganization } from '@/core/auth/auth-provider';

import { previewFile } from './open';

export type ReceiptRef = { engagement: string; uuid: string; mediaType: string; title?: string };

/**
 * Fiş yüklemesini açar (Oğuz'un canlı geri bildirimi): görsel tam ekran
 * görüntüleyicide (`/gorsel`), PDF sistem görüntüleyicisinde (aynı dosya
 * ucu: `.../receipts/{uuid}/image`).
 */
export function useOpenReceipt() {
  const router = useRouter();
  const org = useOrganization();
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);

  const open = async (r: ReceiptRef) => {
    setError(null);
    if (r.mediaType.startsWith('image/')) {
      router.push({ pathname: '/gorsel', params: { engagement: r.engagement, receipt: r.uuid, title: r.title ?? '' } });
      return;
    }
    setBusy(r.uuid);
    try {
      await previewFile(org, {
        path: `/v1/tenant/engagements/${encodeURIComponent(r.engagement)}/receipts/${encodeURIComponent(r.uuid)}/image`,
        uuid: r.uuid,
        prefix: 'fis',
        title: r.title || 'Fiş',
        contentType: r.mediaType,
      });
    } catch {
      setError('Belge açılamadı. Bağlantınızı kontrol edip yeniden deneyin.');
    } finally {
      setBusy(null);
    }
  };

  return { open: (r: ReceiptRef) => void open(r), busy, error };
}
