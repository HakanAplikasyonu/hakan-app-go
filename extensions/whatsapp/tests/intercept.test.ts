import { describe, expect, it, vi } from "vitest";

import { captureDownload, installInterceptor } from "../src/intercept";

// jsdom'da postMessage kaynağı (source) null gelir; testte gerçek tarayıcıdaki
// gibi window olarak işaretlenir.
function patchPostMessage() {
  const orig = window.postMessage.bind(window);
  vi.spyOn(window, "postMessage").mockImplementation((data: unknown) => {
    setTimeout(() => window.dispatchEvent(new MessageEvent("message", { data, source: window })), 0);
    void orig;
  });
}

describe("download interceptor", () => {
  it("hands an armed download to the content script without saving it", async () => {
    patchPostMessage();
    installInterceptor(window);
    const blob = new Blob(["%PDF-1.4"], { type: "application/pdf" });
    const fetchSpy = vi.spyOn(window, "fetch").mockResolvedValue({ blob: async () => blob } as Response);
    const realClick = vi.fn();
    const a = document.createElement("a");
    a.href = "blob:https://web.whatsapp.com/abc";
    a.download = "Ekim kira faturası.pdf";
    a.addEventListener("click", realClick);

    const got = await captureDownload(window, () => a.click(), 2000);
    expect(got.name).toBe("Ekim kira faturası.pdf");
    expect(await got.blob.text()).toBe("%PDF-1.4");
    expect(realClick).not.toHaveBeenCalled(); // the browser download did not run
    expect(fetchSpy).toHaveBeenCalledWith("blob:https://web.whatsapp.com/abc");

    // Not armed: the user's own downloads are untouched.
    a.click();
    expect(realClick).toHaveBeenCalledTimes(1);
    vi.restoreAllMocks();
  });

  it("times out when WhatsApp offers no file", async () => {
    patchPostMessage();
    installInterceptor(window);
    await expect(captureDownload(window, () => {}, 50)).rejects.toThrow("timeout");
    vi.restoreAllMocks();
  });
});
