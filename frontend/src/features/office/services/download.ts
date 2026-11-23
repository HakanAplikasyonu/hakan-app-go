import { apiConfig } from "@/config/api";
import { ORGANIZATION_HEADER } from "@/lib/api/tenant-request";

/**
 * Downloads a file endpoint (Excel exports, #384 / #382 / #395) and saves it
 * with the server's Content-Disposition name, or fallbackName. A failed
 * request throws an error carrying `status`.
 */
export async function downloadFile(
  slug: string,
  path: string,
  query: Record<string, string | undefined>,
  fallbackName: string,
): Promise<void> {
  await downloadFileWithHeaders(slug, path, query, fallbackName);
}

/** downloadFile that also returns the response headers (export counts). */
export async function downloadFileWithHeaders(
  slug: string,
  path: string,
  query: Record<string, string | undefined>,
  fallbackName: string,
): Promise<Headers> {
  const root = apiConfig.baseUrl.replace(/\/$/, "");
  const params = new URLSearchParams();
  for (const [k, v] of Object.entries(query)) if (v) params.set(k, v);
  const qs = params.toString();
  const response = await fetch(`${root}${path}${qs ? `?${qs}` : ""}`, {
    credentials: "include",
    headers: { [ORGANIZATION_HEADER]: slug },
  });
  return save(response, fallbackName);
}

/**
 * POSTs body as JSON to a file endpoint that records the export (Luca
 * masraf dosyası, #780), saves the file and returns the response headers.
 */
export async function downloadPost(
  slug: string,
  path: string,
  body: unknown,
  fallbackName: string,
): Promise<Headers> {
  const root = apiConfig.baseUrl.replace(/\/$/, "");
  const response = await fetch(`${root}${path}`, {
    method: "POST",
    credentials: "include",
    headers: {
      [ORGANIZATION_HEADER]: slug,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(body),
  });
  return save(response, fallbackName);
}

async function save(response: Response, fallbackName: string) {
  if (!response.ok)
    throw Object.assign(new Error(`download ${response.status}`), {
      status: response.status,
    });
  const name =
    /filename="([^"]+)"/.exec(
      response.headers.get("Content-Disposition") ?? "",
    )?.[1] ?? fallbackName;
  const blob = await response.blob();
  const url = URL.createObjectURL(blob);
  const a = document.createElement("a");
  a.href = url;
  a.download = name;
  document.body.appendChild(a);
  a.click();
  a.remove();
  setTimeout(() => URL.revokeObjectURL(url), 10_000);
  return response.headers;
}

/**
 * Opens a file endpoint (a photo or PDF) in a new tab. The tab is opened
 * before the request so the browser does not block it as a popup; when the
 * request fails the tab is closed and the error is thrown.
 */
export async function openFile(slug: string, path: string) {
  const tab = window.open("", "_blank");
  try {
    const root = apiConfig.baseUrl.replace(/\/$/, "");
    const response = await fetch(`${root}${path}`, {
      credentials: "include",
      headers: { [ORGANIZATION_HEADER]: slug },
    });
    if (!response.ok) throw new Error(`open ${response.status}`);
    const url = URL.createObjectURL(await response.blob());
    if (tab) tab.location.href = url;
    else window.location.assign(url);
    setTimeout(() => URL.revokeObjectURL(url), 60_000);
  } catch (err) {
    tab?.close();
    throw err;
  }
}
