/**
 * Opens the screen a tapped notification points to (#201).
 *
 * A tap can arrive before the session is ready (cold start from a
 * notification): the target waits until auth is `ready`. When the link names
 * another of the user's companies, that company is selected first; a link to a
 * company the user cannot see is dropped.
 */
import { router, type Href } from 'expo-router';
import { useEffect, useRef, useState } from 'react';

import { useAuth } from '../auth/auth-provider';
import { resolveLink, type LinkTarget } from './links';
import { subscribeNotificationOpens } from './push';

export function useNotificationRouting() {
  const { status, taxpayer, taxpayers, selectTaxpayer } = useAuth();
  const pending = useRef<LinkTarget | null>(null);
  const switching = useRef(false);
  // Bumped from the notification callback so the effect below re-runs.
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
    const current = taxpayer?.taxpayer.uuid.toLowerCase();
    if (target.taxpayer && target.taxpayer !== current) {
      if (!taxpayers.some((t) => t.uuid.toLowerCase() === target.taxpayer)) {
        pending.current = null; // not one of this user's companies
        return;
      }
      switching.current = true;
      selectTaxpayer(target.taxpayer)
        .catch(() => {
          pending.current = null;
        })
        .finally(() => {
          switching.current = false;
        });
      return; // runs again once the new company is active
    }
    pending.current = null;
    router.push(target.route as Href);
  }, [tap, status, taxpayer, taxpayers, selectTaxpayer]);
}
