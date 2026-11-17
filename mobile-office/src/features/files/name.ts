/** Belge indirme yardımcıları: saf modül, node --test ile sınanır. */

/** Önbellek dosya adı: önek + uuid + uzantı (yalnızca güvenli karakterler). */
export function cacheFileName(prefix: string, uuid: string, fileName: string | null | undefined, contentType?: string | null): string {
  const ext = extensionOf(fileName, contentType);
  const safe = `${prefix}-${uuid}`.replace(/[^A-Za-z0-9_-]/g, '');
  return `${safe}${ext}`;
}

function extensionOf(fileName: string | null | undefined, contentType?: string | null): string {
  const m = /\.([A-Za-z0-9]{1,8})$/.exec(fileName ?? '');
  if (m) return `.${m[1].toLowerCase()}`;
  const byType: Record<string, string> = {
    'application/pdf': '.pdf',
    'image/jpeg': '.jpg',
    'image/png': '.png',
    'image/heic': '.heic',
    'text/csv': '.csv',
    'application/vnd.openxmlformats-officedocument.spreadsheetml.sheet': '.xlsx',
  };
  return byType[(contentType ?? '').split(';')[0].trim().toLowerCase()] ?? '';
}

/** "1536" → "1,5 KB" (tr-TR). */
export function fileSize(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes < 0) return '';
  if (bytes < 1024) return `${bytes} B`;
  const units = ['KB', 'MB', 'GB'];
  let v = bytes / 1024;
  let i = 0;
  while (v >= 1024 && i < units.length - 1) {
    v /= 1024;
    i++;
  }
  return `${v.toLocaleString('tr-TR', { maximumFractionDigits: 1 })} ${units[i]}`;
}
