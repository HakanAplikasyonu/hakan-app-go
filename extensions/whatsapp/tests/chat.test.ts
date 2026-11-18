import { afterEach, describe, expect, it, vi } from "vitest";

import { chatFromModel, installChatResponder, readActiveChat, requestChat, serializeWid } from "../src/chat";
import { currentChat } from "../src/dom";

type W = Window & typeof globalThis;
const win = window as W;
const slots = window as unknown as Record<string, unknown>;

// jsdom'da postMessage kaynağı (source) ve kökeni (origin) gelmez; testte
// gerçek tarayıcıdaki gibi doldurulur.
function patchPostMessage() {
  vi.spyOn(window, "postMessage").mockImplementation((data: unknown) => {
    setTimeout(() => window.dispatchEvent(new MessageEvent("message", { data, source: window, origin: window.location.origin })), 0);
  });
}

const wid = (s: string) => ({ _serialized: s, user: s.split("@")[0], server: s.split("@")[1] });

function fakeRequire(modules: Record<string, unknown>) {
  slots.require = (name: string) => {
    if (!(name in modules)) throw new Error(`Requiring unknown module "${name}"`);
    return modules[name];
  };
}

afterEach(() => {
  delete slots.require;
  delete slots.Store;
  delete slots.webpackChunkwhatsapp_web_client;
  vi.restoreAllMocks();
});

describe("serializeWid", () => {
  it.each([
    [wid("905551112233@c.us"), "905551112233@c.us"],
    [{ user: "120363041234567890", server: "g.us" }, "120363041234567890@g.us"],
    ["98765432101234@lid", "98765432101234@lid"],
  ])("%j → %s", (v, out) => expect(serializeWid(v)).toBe(out));

  it.each([null, undefined, "", "status@broadcast", { _serialized: 5 }, "a b@c.us"])("rejects %j", (v) => {
    expect(serializeWid(v)).toBeNull();
  });
});

describe("readActiveChat (MAIN world)", () => {
  it("reads a contact chat from WAWebChatCollection", () => {
    fakeRequire({
      WAWebChatCollection: { ChatCollection: { getActive: () => ({ id: wid("905551112233@c.us"), formattedTitle: "Deniz Yel" }) } },
    });
    expect(readActiveChat(win)).toEqual({ jid: "905551112233@c.us", title: "Deniz Yel" });
  });

  it("prefers the phone number over a @lid id", () => {
    fakeRequire({
      WAWebChatCollection: {
        ChatCollection: {
          getActive: () => ({ id: wid("98765432101234@lid"), name: "Deniz", contact: { phoneNumber: wid("905551112233@c.us") } }),
        },
      },
    });
    expect(readActiveChat(win)).toEqual({ jid: "905551112233@c.us", title: "Deniz" });
  });

  it("keeps @lid when no phone number is known", () => {
    fakeRequire({ WAWebChatCollection: { ChatCollection: { getActive: () => ({ id: wid("98765432101234@lid"), contact: {} }) } } });
    expect(readActiveChat(win)).toEqual({ jid: "98765432101234@lid", title: "" });
  });

  it("falls back to WAWebCollections for groups", () => {
    fakeRequire({
      WAWebCollections: {
        Chat: { getActive: () => ({ id: wid("120363041234567890@g.us"), formattedTitle: "Muhasebe", contact: { phoneNumber: wid("905551112233@c.us") } }) },
      },
    });
    // Groups keep the group id even if a phone-like field exists.
    expect(readActiveChat(win)).toEqual({ jid: "120363041234567890@g.us", title: "Muhasebe" });
  });

  it("uses the old webpack Store path from the module cache only", () => {
    const Chat = { get: () => null, getActive: () => ({ id: wid("905551112233@c.us"), name: "Eski" }) };
    const run = vi.fn();
    const chunks: unknown[] = [];
    chunks.push = (item: unknown) => {
      (item as [unknown, unknown, (r: unknown) => void])[2]({ c: { a: { exports: { x: 1 } }, b: { exports: { default: { Chat } } } }, m: { run } });
      return 0;
    };
    slots.webpackChunkwhatsapp_web_client = chunks;
    expect(readActiveChat(win)).toEqual({ jid: "905551112233@c.us", title: "Eski" });
    expect(run).not.toHaveBeenCalled();
  });

  it("returns null without any accessor, or with no active chat", () => {
    expect(readActiveChat(win)).toBeNull();
    fakeRequire({ WAWebChatCollection: { ChatCollection: { getActive: () => undefined } } });
    expect(readActiveChat(win)).toBeNull();
  });

  it("reads only the active chat's id and title", () => {
    const touched = new Set<string>();
    const chat = new Proxy(
      { id: wid("905551112233@c.us"), formattedTitle: "Deniz", msgs: { models: ["GİZLİ"] }, lastReceivedKey: "GİZLİ" },
      { get: (t, k) => (touched.add(String(k)), (t as Record<string | symbol, unknown>)[k]) },
    );
    fakeRequire({ WAWebChatCollection: { ChatCollection: { getActive: () => chat } } });
    readActiveChat(win);
    expect(touched.has("msgs")).toBe(false);
    expect(touched.has("lastReceivedKey")).toBe(false);
  });
});

describe("chat request / reply", () => {
  it("content script receives the active chat from the MAIN world", async () => {
    patchPostMessage();
    fakeRequire({ WAWebChatCollection: { ChatCollection: { getActive: () => ({ id: wid("905551112233@c.us"), formattedTitle: "Deniz Yel" }) } } });
    installChatResponder(win);
    expect(await requestChat(window, 500)).toEqual({ jid: "905551112233@c.us", title: "Deniz Yel" });
  });

  it("without window.require the old data-id path still works", async () => {
    patchPostMessage();
    installChatResponder(win); // already installed: no second listener
    document.body.innerHTML = `<div id="main"><header><span title="Eski Sohbet"></span></header>
      <div data-id="false_905321234567@c.us_3EB0IMAGE001"></div></div>`;
    expect(await requestChat(window, 500)).toBeNull();
    expect(await currentChat(document)).toEqual({ jid: "905321234567@c.us", title: "Eski Sohbet" });
  });

  it("ignores replies from another origin or source", async () => {
    vi.spyOn(window, "postMessage").mockImplementation((data: unknown) => {
      const d = data as { type: string; token: string };
      setTimeout(() => {
        const reply = { type: "hakan-wa:chat", token: d.token, jid: "905551112233@c.us", title: "x" };
        window.dispatchEvent(new MessageEvent("message", { data: reply, source: window, origin: "https://evil.example" }));
        window.dispatchEvent(new MessageEvent("message", { data: reply, source: null, origin: window.location.origin }));
      }, 0);
    });
    expect(await requestChat(window, 50)).toBeNull();
  });

  it("chatFromModel ignores non-objects", () => {
    expect(chatFromModel(null)).toBeNull();
    expect(chatFromModel({ id: "status@broadcast" })).toBeNull();
  });
});
