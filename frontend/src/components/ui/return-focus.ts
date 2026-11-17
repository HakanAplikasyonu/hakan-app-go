"use client";

import * as React from "react";

/**
 * Diyalog kapanınca odak açan öğeye döner (#455). Radix, odağı yalnızca
 * `DialogTrigger` varsa geri verir; durumla (`open` state) açılan diyalog
 * ve çekmecelerde odak `body`'ye düşüyor, klavye kullanıcısı yerini
 * kaybediyordu. Açılışta (odak içeri taşınmadan önce) odaktaki öğe saklanır,
 * kapanışta (başka bir işleyici engellemediyse) ona dönülür.
 */
export function useReturnFocus({
  onOpenAutoFocus,
  onCloseAutoFocus,
}: {
  onOpenAutoFocus?: (event: Event) => void;
  onCloseAutoFocus?: (event: Event) => void;
}) {
  const opener = React.useRef<HTMLElement | null>(null);
  const onOpen = React.useCallback(
    (event: Event) => {
      const el = document.activeElement;
      opener.current =
        el instanceof HTMLElement && el !== document.body ? el : null;
      onOpenAutoFocus?.(event);
    },
    [onOpenAutoFocus],
  );
  const onClose = React.useCallback(
    (event: Event) => {
      onCloseAutoFocus?.(event);
      const el = opener.current;
      opener.current = null;
      if (event.defaultPrevented) return;
      if (el?.isConnected) {
        event.preventDefault();
        el.focus({ preventScroll: true });
      }
    },
    [onCloseAutoFocus],
  );
  return { onOpenAutoFocus: onOpen, onCloseAutoFocus: onClose };
}
