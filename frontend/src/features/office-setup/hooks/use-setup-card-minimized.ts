"use client";

import { useCallback, useSyncExternalStore } from "react";

const CHANGE_EVENT = "app:setup-card-change";
const key = (slug: string) => `app.office-setup.card-minimized.${slug}`;

function read(slug: string): boolean {
  try {
    return window.localStorage.getItem(key(slug)) === "1";
  } catch {
    return false;
  }
}

function subscribe(onStoreChange: () => void) {
  window.addEventListener("storage", onStoreChange);
  window.addEventListener(CHANGE_EVENT, onStoreChange);
  return () => {
    window.removeEventListener("storage", onStoreChange);
    window.removeEventListener(CHANGE_EVENT, onStoreChange);
  };
}

/**
 * Office home setup reminder minimized to a single link (#710), per office
 * and browser. Without storage (private mode) the toggle simply does not
 * persist.
 */
export function useSetupCardMinimized(slug: string) {
  const minimized = useSyncExternalStore(
    subscribe,
    () => read(slug),
    () => false,
  );
  const setMinimized = useCallback(
    (next: boolean) => {
      try {
        if (next) window.localStorage.setItem(key(slug), "1");
        else window.localStorage.removeItem(key(slug));
      } catch {
        // Storage unavailable: no-op.
      }
      window.dispatchEvent(new Event(CHANGE_EVENT));
    },
    [slug],
  );
  return [minimized, setMinimized] as const;
}
