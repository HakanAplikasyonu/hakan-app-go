/**
 * Mükellef tarafı fiş ve görev uçları.
 *
 * Henüz backend'de yok (#38 PR'ı "Karar gerekiyor"): ofis tarafındaki
 * `POST /v1/tenant/engagements/{uuid}/receipts` sözleşmesinin mükellef kapsamlı
 * (`X-Taxpayer`) karşılığı olarak önerildi. Uç nokta yokken 404 döner ve fiş
 * kuyrukta `blocked` olarak bekler.
 */
import { Platform } from 'react-native';

import { api, request } from '@/core/api/client';
import { session } from '@/core/auth/session';
import { env } from '@/core/config/env';
import { ApiError, unwrap } from '@/core/api/envelope';

import type { Correction, QueueItem, ReceiptResult } from './queue';
import type { ServerTask } from '../tasks/tasks';

export const RECEIPTS_PATH = '/v1/taxpayer/receipts';
export const TASKS_PATH = '/v1/taxpayer/tasks';

async function formData(item: QueueItem): Promise<FormData> {
  const form = new FormData();
  const name = `${item.id}.${item.mimeType.split('/')[1] ?? 'jpg'}`;
  if (Platform.OS === 'web') {
    const blob = await (await fetch(item.fileUri)).blob();
    form.append('image', blob, name);
  } else {
    // React Native's FormData takes a file descriptor object.
    form.append('image', { uri: item.fileUri, name, type: item.mimeType } as unknown as Blob);
  }
  form.append('kind', item.kind);
  return form;
}

async function postMultipart(item: QueueItem, token: string | null) {
  const headers: Record<string, string> = {
    Accept: 'application/json',
    'X-Taxpayer': item.taxpayer,
    'Idempotency-Key': item.id,
  };
  if (token) headers.Authorization = `Bearer ${token}`;
  try {
    const res = await fetch(`${env.apiUrl}${RECEIPTS_PATH}`, { method: 'POST', headers, body: await formData(item) });
    const payload = await res.json().catch(() => undefined);
    return { status: res.status, payload, requestId: res.headers.get('X-Request-ID') ?? undefined };
  } catch {
    throw new ApiError({ status: 0, code: 'NETWORK_ERROR', message: 'Network request failed' });
  }
}

/** Uploads one queued photo. Refreshes the session once on 401 (via a cheap authed call). */
export async function uploadReceipt(item: QueueItem): Promise<ReceiptResult> {
  let res = await postMultipart(item, session.get()?.accessToken ?? null);
  if (res.status === 401) {
    // Any authed request runs the shared refresh; then retry with the new token.
    await api.get('/v1/auth/me').catch(() => {});
    res = await postMultipart(item, session.get()?.accessToken ?? null);
  }
  return unwrap<ReceiptResult>(res.status, res.payload, res.requestId);
}

export function sendCorrections(item: QueueItem, corrections: Correction[]) {
  return request<unknown>('POST', `${RECEIPTS_PATH}/${item.result?.uuid}/corrections`, {
    taxpayer: item.taxpayer,
    body: { corrections },
  });
}

/** Görevler; uç nokta yoksa null (ekran "henüz yok" der). */
export async function fetchTasks(taxpayer: string): Promise<ServerTask[] | null> {
  try {
    const data = await api.get<{ items: ServerTask[] } | ServerTask[]>(TASKS_PATH, { taxpayer });
    return Array.isArray(data) ? data : (data?.items ?? []);
  } catch (e) {
    if (e instanceof ApiError && (e.status === 404 || e.status === 405)) return null;
    throw e;
  }
}
