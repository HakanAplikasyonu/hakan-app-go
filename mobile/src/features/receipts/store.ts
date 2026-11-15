/**
 * Kuyruğun cihazda saklanması. Fotoğraflar ve kuyruk dosyası uygulamanın belge
 * klasöründe durur (uygulama kapansa da kaybolmaz; OS temizlemez). Web derlemesi
 * (yalnızca geliştirme sırasında ekran denemesi için) localStorage kullanır.
 */
import { Directory, File, Paths } from 'expo-file-system';
import { Platform } from 'react-native';

import type { QueueItem } from './queue';

const WEB_KEY = 'hakan.receipt-queue';

function dir(): Directory {
  const d = new Directory(Paths.document, 'receipts');
  if (!d.exists) d.create({ intermediates: true, idempotent: true });
  return d;
}

function indexFile(): File {
  return new File(dir(), 'queue.json');
}

export async function loadQueue(): Promise<QueueItem[]> {
  try {
    if (Platform.OS === 'web') {
      const raw = globalThis.localStorage?.getItem(WEB_KEY);
      return raw ? (JSON.parse(raw) as QueueItem[]) : [];
    }
    const f = indexFile();
    if (!f.exists) return [];
    return JSON.parse(await f.text()) as QueueItem[];
  } catch {
    return [];
  }
}

export async function saveQueue(items: readonly QueueItem[]): Promise<void> {
  const json = JSON.stringify(items);
  if (Platform.OS === 'web') {
    globalThis.localStorage?.setItem(WEB_KEY, json);
    return;
  }
  const f = indexFile();
  if (!f.exists) f.create();
  f.write(json);
}

/** Copies a picker/camera photo (a temporary cache file) into the queue folder. */
export async function keepPhoto(uri: string, id: string, mimeType: string): Promise<string> {
  if (Platform.OS === 'web') return uri; // data: URI from the web picker
  const ext = mimeType === 'image/png' ? 'png' : mimeType === 'image/webp' ? 'webp' : 'jpg';
  const target = new File(dir(), `${id}.${ext}`);
  if (target.exists) target.delete();
  new File(uri).copy(target);
  return target.uri;
}

export function deletePhoto(uri: string) {
  if (Platform.OS === 'web') return;
  try {
    const f = new File(uri);
    if (f.exists) f.delete();
  } catch {
    // already gone
  }
}
