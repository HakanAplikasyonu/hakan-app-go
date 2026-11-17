const ROW_CLICK_IGNORE_SELECTOR = [
  "a",
  "button",
  "input",
  "textarea",
  "select",
  "label",
  "[role='button']",
  "[role='checkbox']",
  "[role='menuitem']",
  "[role='option']",
  "[role='switch']",
  "[data-no-row-click]",
].join(", ");

export function isRowClickIgnored(event: {
  defaultPrevented: boolean;
  target: EventTarget | null;
}): boolean {
  if (event.defaultPrevented) return true;
  if (!(event.target instanceof Element)) return true;
  if (event.target.closest(ROW_CLICK_IGNORE_SELECTOR)) return true;

  const selection = window.getSelection();
  if (selection && !selection.isCollapsed && selection.toString().trim()) {
    return true;
  }

  return false;
}

/**
 * Tıklanabilir satırın klavye etkinleştirmesi: satırın kendisi odaktayken
 * Enter veya Boşluk. İçteki denetimlerden (düğme, bağlantı, giriş) gelen
 * tuşlar satırı açmaz.
 */
export function isRowKeyActivation(event: {
  key: string;
  target: EventTarget | null;
  currentTarget: EventTarget | null;
  defaultPrevented?: boolean;
}): boolean {
  if (event.defaultPrevented) return false;
  if (event.key !== "Enter" && event.key !== " ") return false;
  return event.target === event.currentTarget;
}
