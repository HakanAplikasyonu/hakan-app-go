import { describe, expect, it } from "vitest";

import {
  buildInit,
  cleanScriptUrl,
  cleanWidgetId,
  compactAttributes,
  DEFAULT_CLICKFIX_SCRIPT_URL,
  resolveWidgetId,
} from "./clickfix";

describe("cleanWidgetId", () => {
  it("boş ya da geçersiz kimlikte null (widget yüklenmez)", () => {
    expect(cleanWidgetId("")).toBeNull();
    expect(cleanWidgetId(undefined)).toBeNull();
    expect(cleanWidgetId("<script>")).toBeNull();
    expect(cleanWidgetId("a b")).toBeNull();
  });
  it("kenar boşluklarını kırpar", () => {
    expect(cleanWidgetId(" ofis_1-a ")).toBe("ofis_1-a");
  });
});

describe("cleanScriptUrl", () => {
  it("yalnız https; yerelde localhost http", () => {
    expect(cleanScriptUrl("")).toBe(DEFAULT_CLICKFIX_SCRIPT_URL);
    expect(cleanScriptUrl("javascript:alert(1)")).toBe(
      DEFAULT_CLICKFIX_SCRIPT_URL,
    );
    expect(cleanScriptUrl("http://evil.example/w.js")).toBe(
      DEFAULT_CLICKFIX_SCRIPT_URL,
    );
    expect(cleanScriptUrl("http://localhost:3001/widget.js")).toBe(
      "http://localhost:3001/widget.js",
    );
    expect(cleanScriptUrl("https://destek.example.com/widget.js")).toBe(
      "https://destek.example.com/widget.js",
    );
  });
});

describe("resolveWidgetId", () => {
  const env = { widgetId: "saglayici", taxpayerWidgetId: "varsayilan" };
  it("ofis paneli platform widget'ını kullanır", () => {
    expect(resolveWidgetId("provider", env, "ofis")).toBe("saglayici");
    expect(resolveWidgetId("provider", {}, "ofis")).toBeNull();
  });
  it("mükellef portalı önce ofisin kimliğini, yoksa varsayılanı kullanır", () => {
    expect(resolveWidgetId("taxpayer", env, "ofis")).toBe("ofis");
    expect(resolveWidgetId("taxpayer", env, "")).toBe("varsayilan");
    expect(resolveWidgetId("taxpayer", { widgetId: "saglayici" }, null)).toBe(
      null,
    );
  });
});

describe("buildInit / compactAttributes", () => {
  it("balon gizli, dil tr/en", () => {
    expect(buildInit("w", "en-US")).toEqual({
      widgetId: "w",
      locale: "en",
      hideLauncher: true,
    });
    expect(buildInit("w", "tr").locale).toBe("tr");
  });
  it("boş öznitelikler gönderilmez", () => {
    expect(compactAttributes({ a: " x ", b: "", c: undefined })).toEqual({
      a: "x",
    });
  });
});
