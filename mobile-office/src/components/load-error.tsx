import { HataDurumu } from '@hakan/mobile-core/hakan';

import { Button } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';

/**
 * Liste ya da sekme hiç yüklenemediğinde (çevrimdışı, sunucu hatası; #454):
 * ne olduğu, Türkçe hata metni ve "Tekrar dene". Boş durumla karışmaz.
 */
export function LoadError({
  title,
  error,
  retry,
  retrying,
}: {
  title: string;
  error: unknown;
  retry: () => unknown;
  retrying?: boolean;
}) {
  return (
    <HataDurumu
      baslik={title}
      aciklama={errorMessage(error)}
      eylem={<Button testID="load-retry" title="Tekrar dene" variant="secondary" loading={retrying} onPress={() => void retry()} />}
    />
  );
}
