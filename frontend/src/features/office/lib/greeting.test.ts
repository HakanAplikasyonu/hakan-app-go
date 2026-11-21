import { describe, expect, it } from "vitest";

import { greetingKey } from "./greeting";

// Türkiye UTC+3 (yaz saati yok): İstanbul saati = UTC saati + 3.
const at = (istanbul: string) => new Date(`2026-11-21T${istanbul}:00+03:00`);

describe("greetingKey: Türkiye saatine göre selamlama", () => {
  it.each([
    ["04:30", "office.home.greeting_night"],
    ["04:59", "office.home.greeting_night"],
    ["05:00", "office.home.greeting_morning"],
    ["11:59", "office.home.greeting_morning"],
    ["12:00", "office.home.greeting_day"],
    ["17:59", "office.home.greeting_day"],
    ["18:00", "office.home.greeting_evening"],
    ["21:59", "office.home.greeting_evening"],
    ["22:00", "office.home.greeting_night"],
    ["00:00", "office.home.greeting_night"],
  ])("%s → %s", (time, key) => {
    expect(greetingKey(at(time))).toBe(key);
  });

  it("cihaz saati ne olursa olsun İstanbul saatini kullanır", () => {
    // 01:30 UTC = 04:30 İstanbul; cihaz UTC-5'te olsa akşam görünürdü.
    expect(greetingKey(new Date("2026-11-21T01:30:00Z"))).toBe(
      "office.home.greeting_night",
    );
  });
});
