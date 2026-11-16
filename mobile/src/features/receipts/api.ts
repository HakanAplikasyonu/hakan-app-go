/**
 * Mükellef tarafı fiş uçları (#199).
 *
 * `POST /v1/taxpayer/receipts` (multipart `image`, `kind`; `Idempotency-Key`)
 * fotoğrafı şirketin etkin ofisi için okur; aynı anahtarla tekrar gelen istek
 * ilk kaydı 200 ile döner. `POST …/{uuid}/corrections` önerileri ofisin
 * inceleme kuyruğuna gönderir. `GET /v1/taxpayer/receipts` sunucudaki geçmiştir
 * (telefon değişince de görünür).
 */
import { Platform } from 'react-native';

import { api, request } from '@/core/api/client';
import { session } from '@/core/auth/session';
import { env } from '@/core/config/env';
import { ApiError, unwrap } from '@/core/api/envelope';

import type { Correction, QueueItem, ReceiptResult } from './queue';

export const RECEIPTS_PATH = '/v1/taxpayer/receipts';

async function formData(item: QueueItem, field: string, extra: Record<string, string>): Promise<FormData> {
  const form = new FormData();
  const name = `${item.id}.${item.mimeType.split('/')[1] ?? 'jpg'}`;
  if (Platform.OS === 'web') {
    const blob = await (await fetch(item.fileUri)).blob();
    form.append(field, blob, name);
  } else {
    // React Native's FormData takes a file descriptor object.
    form.append(field, { uri: item.fileUri, name, type: item.mimeType } as unknown as Blob);
  }
  for (const [k, v] of Object.entries(extra)) form.append(k, v);
  return form;
}

type Multipart = { path: string; field: string; extra?: Record<string, string> };

async function postMultipart(item: QueueItem, m: Multipart, token: string | null) {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    'X-Taxpayer': item.taxpayer,
    'Idempotency-Key': item.id,
  };
  if (token) headers.Authorization = `Bearer ${token}`;
  try {
    const res = await fetch(`${env.apiUrl}${m.path}`, { method: 'POST', headers, body: await formData(item, m.field, m.extra ?? {}) });
    const payload = await res.json().catch(() => undefined);
    return { status: res.status, payload, requestId: res.headers.get('X-Request-ID') ?? undefined };
  } catch {
    throw new ApiError({ status: 0, code: 'NETWORK_ERROR', message: 'Network request failed' });
  }
}

/** Posts one queued photo as multipart. Refreshes the session once on 401 (via a cheap authed call). */
export async function postQueuedPhoto<T>(item: QueueItem, m: Multipart): Promise<T> {
  let res = await postMultipart(item, m, session.get()?.accessToken ?? null);
  if (res.status === 401) {
    // Any authed request runs the shared refresh; then retry with the new token.
    await api.get('/v1/auth/me').catch(() => {});
    res = await postMultipart(item, m, session.get()?.accessToken ?? null);
  }
  return unwrap<T>(res.status, res.payload, res.requestId);
}

/** Uploads one queued receipt photo. */
export function uploadReceipt(item: QueueItem): Promise<ReceiptResult> {
  return postQueuedPhoto<ReceiptResult>(item, { path: RECEIPTS_PATH, field: 'image', extra: { kind: item.kind } });
}

export function sendCorrections(item: QueueItem, corrections: Correction[]) {
  return request<unknown>('POST', `${RECEIPTS_PATH}/${item.result?.uuid}/corrections`, {
    taxpayer: item.taxpayer,
    body: { corrections },
  });
}

/** A receipt in the server-side history (`GET /v1/taxpayer/receipts`). */
export type ServerReceipt = {
  uuid: string;
  kind?: string;
  status: string;
  media_type: string;
  vendor_name?: string;
  date?: string;
  total_amount?: string;
  error?: string;
  created_at: string;
};

/** The company's receipts on the server, newest first. */
export async function fetchReceiptHistory(taxpayer: string, limit = 50): Promise<ServerReceipt[]> {
  const data = await api.get<{ items: ServerReceipt[] }>(`${RECEIPTS_PATH}?limit=${limit}`, { taxpayer });
  return data?.items ?? [];
}
