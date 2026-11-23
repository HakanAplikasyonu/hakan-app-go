/**
 * Masaüstü ajanı indirme bağlantıları (Oğuz'un demo geri bildirimi).
 *
 * Paketleri `.github/workflows/desktop-release.yml` `desktop-vX.Y.Z` GitHub
 * Release'ine yükler. Depo herkese açık olduğundan sürüm listesi ve paketler
 * oturumsuz okunur; tarayıcı GitHub API'sini doğrudan çağırır (CSP yalnızca
 * frame-ancestors / base-uri kısıtlar, connect-src yok: next.config.ts).
 */
export const DESKTOP_RELEASES_API =
  "https://api.github.com/repos/HakanAplikasyonu/hakan-app-go/releases?per_page=30";

export const DESKTOP_RELEASES_PAGE =
  "https://github.com/HakanAplikasyonu/hakan-app-go/releases";

type GithubAsset = { name: string; browser_download_url: string; size?: number };

export type GithubRelease = {
  tag_name: string;
  name?: string | null;
  draft?: boolean;
  prerelease?: boolean;
  published_at?: string | null;
  html_url?: string;
  assets?: GithubAsset[];
};

export type DesktopDownload = { name: string; url: string; size?: number };

export type DesktopRelease = {
  tag: string;
  version: string;
  publishedAt?: string;
  pageUrl: string;
  prerelease: boolean;
  mac?: DesktopDownload;
  windows?: DesktopDownload;
};

const TAG_PREFIX = "desktop-v";

function pickAsset(
  assets: GithubAsset[],
  ...exts: string[]
): DesktopDownload | undefined {
  for (const ext of exts) {
    const a = assets.find((x) => x.name.toLowerCase().endsWith(ext));
    if (a) return { name: a.name, url: a.browser_download_url, size: a.size };
  }
  return undefined;
}

/**
 * En yeni `desktop-v*` sürümünü seçer: önce kararlı, kararlı yoksa beta.
 * GitHub listeyi yeniden eskiye verir. Paketi olmayan sürüm atlanır.
 */
export function pickDesktopRelease(
  releases: GithubRelease[],
): DesktopRelease | null {
  const candidates = releases
    .filter((r) => r.tag_name?.startsWith(TAG_PREFIX) && !r.draft)
    .sort((a, b) =>
      (b.published_at ?? "").localeCompare(a.published_at ?? ""),
    )
    .map((r) => {
      const assets = r.assets ?? [];
      return {
        tag: r.tag_name,
        version: r.tag_name.slice(TAG_PREFIX.length),
        publishedAt: r.published_at ?? undefined,
        pageUrl: r.html_url ?? DESKTOP_RELEASES_PAGE,
        prerelease: Boolean(r.prerelease),
        mac: pickAsset(assets, ".dmg"),
        // .msi kurumsal kurulum için; yoksa NSIS kurucusu (-setup.exe).
        windows: pickAsset(assets, ".msi", ".exe"),
      } satisfies DesktopRelease;
    })
    .filter((r) => r.mac || r.windows);
  return candidates.find((r) => !r.prerelease) ?? candidates[0] ?? null;
}

export async function fetchLatestDesktopRelease(): Promise<DesktopRelease | null> {
  const res = await fetch(DESKTOP_RELEASES_API, {
    headers: { Accept: "application/vnd.github+json" },
  });
  if (res.status === 404) return null;
  if (!res.ok) throw new Error(`GitHub ${res.status}`);
  const data = (await res.json()) as unknown;
  return Array.isArray(data) ? pickDesktopRelease(data as GithubRelease[]) : null;
}
