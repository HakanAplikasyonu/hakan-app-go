/**
 * Dokunulan bildirimin gösterdiği ekranı açar (#241).
 * Kalıp: mobile/src/core/push/use-notification-routing.ts (#201); mükellef
 * yerine ofis (slug) değiştirilir.
 *
 * Dokunuş oturum hazır olmadan gelebilir (bildirimden soğuk açılış): hedef
 * `ready` durumunu bekler. Bağlantı kullanıcının başka bir ofisine aitse önce
 * o ofis seçilir; kullanıcının görmediği bir ofise giden bağlantı atılır.
 */
import { router, type Href } from 'expo-router';
import { useEffect, useRef, useState } from 'react';

import { useAuth } from '../auth/auth-provider';
import { hrefOf, resolveLink, type LinkTarget } from './links';
import { subscribeNotificationOpens } from './push';

export function useNotificationRouting() {
  const { status, officeSlug, offices, selectOffice } = useAuth();
  const pending = useRef<LinkTarget | null>(null);
  const switching = useRef(false);
  // Bildirim geri çağrısından artırılır; aşağıdaki etki yeniden çalışır.
  const [tap, setTap] = useState(0);

  useEffect(
    () =>
      subscribeNotificationOpens((url) => {
        const target = resolveLink(url);
        if (!target) return;
        pending.current = target;
        setTap((n) => n + 1);
      }),
    [],
  );

  useEffect(() => {
    const target = pending.current;
    if (!target || status !== 'ready' || switching.current) return;
    const current = officeSlug?.toLowerCase();
    if (target.office && target.office !== current) {
      if (!offices.some((o) => o.slug.toLowerCase() === target.office)) {
        pending.current = null; // bu kullanıcının ofisi değil
        return;
      }
      switching.current = true;
      selectOffice(target.office)
        .catch(() => {
          pending.current = null;
        })
        .finally(() => {
          switching.current = false;
        });
      return; // yeni ofis açılınca yeniden çalışır
    }
    pending.current = null;
    router.push(hrefOf(target) as Href);
  }, [tap, status, officeSlug, offices, selectOffice]);
}
