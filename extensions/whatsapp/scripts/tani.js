// HAKAN WhatsApp uzantısı · tanı betiği (issue #388).
//
// Kullanım: web.whatsapp.com'da bir sohbet açın (içinde bir görsel ve bir PDF
// olsun), F12 → Konsol sekmesine bu dosyanın tamamını yapıştırıp Enter'a
// basın. Çıkan JSON'u (panoya da kopyalanır) geliştiriciye gönderin.
//
// Gizlilik: mesaj metni OKUNMAZ (textContent / innerText / data-pre-plain-text
// kullanılmaz). Yalnızca seçici sayıları, data-testid / data-icon adları ve
// öznitelik iskeleti alınır. Rakamlar 9 ile, uzun onaltılık diziler X ile
// maskelenir. class, style, src, href, title, alt, aria-label ve benzeri
// içerik taşıyabilen öznitelikler hiç yazılmaz. Sohbet kimliğinin yalnızca
// soneki (@c.us / @lid / @g.us) yazılır.
(() => {
  const mask = (v) =>
    String(v)
      .replace(/[0-9A-Fa-f]{8,}/g, (m) => (/[0-9]/.test(m) && /[A-Fa-f]/.test(m) ? "X" : m))
      .replace(/[0-9]/g, "9")
      .slice(0, 80);
  const SKIP_ATTR = new Set([
    "class", "style", "src", "srcset", "href", "title", "alt", "aria-label", "aria-description",
    "aria-valuetext", "data-pre-plain-text", "data-plain-text", "placeholder", "value", "download", "content",
  ]);
  const count = (sel) => {
    try {
      return document.querySelectorAll(sel).length;
    } catch {
      return "hata";
    }
  };
  const uniq = (attr, root) => {
    const out = new Set();
    for (const el of root.querySelectorAll(`[${attr}]`)) out.add(mask(el.getAttribute(attr)));
    return Array.from(out).sort().slice(0, 150);
  };
  const skeleton = (el, depth) => {
    if (!el || depth > 8) return null;
    const attrs = {};
    for (const a of Array.from(el.attributes)) {
      if (SKIP_ATTR.has(a.name) || a.name.startsWith("on")) continue;
      attrs[a.name] = mask(a.value);
    }
    const node = { tag: el.tagName, attrs };
    const kids = Array.from(el.children).slice(0, 12).map((c) => skeleton(c, depth + 1)).filter(Boolean);
    if (kids.length) node.children = kids;
    return node;
  };
  const suffix = (jid) => {
    const s = typeof jid === "string" ? jid : jid && (jid._serialized || (jid.user && jid.server && `${jid.user}@${jid.server}`));
    const m = typeof s === "string" ? /@[a-z.]+$/.exec(s) : null;
    return m ? m[0] : null;
  };
  const chatAccessors = {
    "require(WAWebChatCollection).ChatCollection.getActive": () => window.require("WAWebChatCollection").ChatCollection.getActive(),
    "require(WAWebCollections).Chat.getActive": () => window.require("WAWebCollections").Chat.getActive(),
    "Store.Chat.getActive": () => window.Store.Chat.getActive(),
  };
  const accessors = {};
  for (const [name, fn] of Object.entries(chatAccessors)) {
    try {
      const chat = fn();
      if (!chat) {
        accessors[name] = "çalışıyor, aktif sohbet yok";
        continue;
      }
      const contact = chat.contact || {};
      accessors[name] = {
        id: suffix(chat.id),
        phoneNumber: suffix(contact.phoneNumber) || suffix(chat.phoneNumber),
        hasTitle: typeof (chat.formattedTitle || chat.name) === "string",
      };
    } catch (e) {
      accessors[name] = "yok (" + (e && e.name ? e.name : "hata") + ")";
    }
  }
  const main = document.querySelector("#main");
  const ids = main ? Array.from(main.querySelectorAll("[data-id]")).map((el) => el.getAttribute("data-id")) : [];
  const rows = main ? main.querySelectorAll('[data-testid^="conv-msg-"], [data-id]') : [];
  const out = {
    surum: "tani-1",
    main: count("#main"),
    dataIdSayisi: ids.length,
    dataIdOrnekleri: Array.from(new Set(ids.map(mask))).slice(0, 12),
    secici: Object.fromEntries(
      [
        '#main header [data-testid="conversation-info-header-chat-title"]',
        '#main header [data-testid="conversation-info-header-chat-title-name"]',
        "#main header span[title]",
        '#main header span[dir="auto"]',
        '#main [data-testid^="conv-msg-"]',
        '#main img[src^="blob:"]',
        '#main img[src^="data:image"]',
        '[data-testid="document-thumb"]',
        '[data-icon^="document"]',
        "#main [data-id] span[title]",
        '[data-testid="document-download"]',
        '[data-testid="media-state-download"]',
        '[data-icon*="download"]',
        '[data-testid="media-viewer"]',
      ].map((s) => [s, count(s)]),
    ),
    dataIcon: main ? uniq("data-icon", main) : [],
    dataTestid: main ? uniq("data-testid", main) : [],
    sonSatir: rows.length ? skeleton(rows[rows.length - 1], 0) : null,
    sohbetErisimcisi: accessors,
  };
  const json = JSON.stringify(out, null, 2);
  try {
    if (typeof copy === "function") copy(json);
  } catch {
    // copy() yalnızca geliştirici konsolunda vardır.
  }
  console.log(json);
  return out;
})();
