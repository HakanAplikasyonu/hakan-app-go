import { File, Paths } from 'expo-file-system';
import * as Sharing from 'expo-sharing';
import { Linking, Platform } from 'react-native';

import { session } from '@/core/auth/session';
import { env } from '@/core/config/env';

import { cacheFileName } from './name';

export type RemoteFile = {
  /** `/v1/tenant/...` yolu (dosyanın indirme ucu). */
  path: string;
  uuid: string;
  prefix: string;
  title: string;
  fileName?: string | null;
  contentType?: string | null;
};

/**
 * Dosyayı oturum belirteci ve ofis kapsamıyla önbelleğe indirir. Liste isteği
 * belirteci tazelediği için burada geçerli bir erişim belirteci vardır.
 */
async function download(org: string, f: RemoteFile): Promise<File> {
  const token = session.get()?.accessToken;
  const dest = new File(Paths.cache, cacheFileName(f.prefix, f.uuid, f.fileName, f.contentType));
  if (dest.exists) dest.delete();
  return File.downloadFileAsync(`${env.apiUrl.replace(/\/$/, '')}${f.path}`, dest, {
    headers: { 'X-Organization': org, ...(token ? { Authorization: `Bearer ${token}` } : {}) },
  });
}

/**
 * Önizleme: Android'de varsayılan görüntüleyici (PDF okuyucu), iOS'ta sistem
 * önizleme / paylaşım sayfası (Quick Look).
 */
export async function previewFile(org: string, f: RemoteFile): Promise<void> {
  const file = await download(org, f);
  if (Platform.OS === 'android' && file.contentUri) {
    await Linking.openURL(file.contentUri);
    return;
  }
  await shareLocal(file, f);
}

/** Sistem paylaş menüsü (e-posta, WhatsApp, Dosyalar…). */
export async function shareFile(org: string, f: RemoteFile): Promise<void> {
  await shareLocal(await download(org, f), f);
}

async function shareLocal(file: File, f: RemoteFile): Promise<void> {
  if (!(await Sharing.isAvailableAsync())) {
    await Linking.openURL(file.contentUri ?? file.uri);
    return;
  }
  await Sharing.shareAsync(file.uri, {
    dialogTitle: f.title,
    mimeType: f.contentType ?? undefined,
    UTI: f.contentType === 'application/pdf' ? 'com.adobe.pdf' : undefined,
  });
}
