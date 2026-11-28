import { describe, expect, it } from "vitest";

import {
  alreadySaved,
  badgeText,
  badgeTone,
  isSettled,
  latestFor,
  phoneOfChat,
  sha256Hex,
  stateFromCapture,
  type CaptureStatus,
} from "../src/badge";

const row = (over: Partial<CaptureStatus> = {}): CaptureStatus => ({
  sha256: "a".repeat(64),
  uuid: "c1",
  target: "expense",
  engagement_uuid: "eng-1",
  taxpayer_title: "Atlas Gıda",
  state: "pending",
  created_at: "2026-11-01T10:00:00Z",
  ...over,
});

describe("badge", () => {
  it("writes the receipt's journey", () => {
    expect(badgeText("reading")).toBe("Okunuyor…");
    expect(badgeText("pending", "Atlas Gıda")).toBe("Atlas Gıda · Kaydedildi · onay bekliyor");
    expect(badgeText("approved")).toBe("Onaylandı ✓");
    expect(badgeText("rejected")).toMatch(/^Reddedildi/);
    expect(badgeTone("approved")).toBe("ok");
    expect(badgeTone("failed")).toBe("bad");
    expect(badgeTone("pending")).toBe("wait");
  });

  it("maps the capture reply like the server does", () => {
    expect(stateFromCapture("expense", "draft")).toBe("pending");
    expect(stateFromCapture("expense", "failed")).toBe("failed");
    expect(stateFromCapture("income", "duplicate")).toBe("duplicate");
    expect(stateFromCapture("document", null)).toBe("saved");
    expect(stateFromCapture("foreign_invoice", "draft")).toBe("saved");
  });

  it("keeps polling only open states", () => {
    expect(isSettled("pending")).toBe(false);
    expect(isSettled("reading")).toBe(false);
    expect(isSettled("approved")).toBe(true);
    expect(isSettled("rejected")).toBe(true);
  });

  it("finds the newest capture of a file and warns on the same target", () => {
    const old = row({ uuid: "old", state: "approved", created_at: "2026-10-01T10:00:00Z" });
    const doc = row({ uuid: "doc", target: "document", state: "saved", created_at: "2026-11-02T10:00:00Z" });
    expect(latestFor([old, doc], "a".repeat(64))?.uuid).toBe("doc");
    expect(latestFor([old], "b".repeat(64))).toBeNull();
    expect(alreadySaved([old, doc], "a".repeat(64), "expense")).toBe("Bu fiş zaten kaydedildi (Atlas Gıda).");
    expect(alreadySaved([old], "a".repeat(64), "income")).toBeNull();
    expect(alreadySaved([doc], "a".repeat(64), "document")).toBe("Bu dosya zaten kaydedildi (Atlas Gıda).");
  });

  it("reads the phone of a person chat only", () => {
    expect(phoneOfChat("905321234567@c.us")).toBe("+905321234567");
    expect(phoneOfChat("120363000000000000@g.us")).toBeNull();
    expect(phoneOfChat("123@lid")).toBeNull();
  });

  it("hashes like the server (sha256 of the bytes)", async () => {
    expect(await sha256Hex(new Blob(["abc"]))).toBe("ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
  });
});
