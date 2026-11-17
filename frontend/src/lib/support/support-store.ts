import { create } from "zustand";

/** Whether a ClickFix widget is mounted on the page, and its unread count. */
type SupportState = {
  available: boolean;
  unread: number;
  setAvailable: (available: boolean) => void;
  setUnread: (unread: number) => void;
};

export const useSupportStore = create<SupportState>()((set) => ({
  available: false,
  unread: 0,
  setAvailable: (available) =>
    set(available ? { available } : { available, unread: 0 }),
  setUnread: (unread) => set({ unread }),
}));
