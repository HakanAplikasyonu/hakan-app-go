/**
 * Mükellef belge görevleri (#200): ofisin istediği eksik belgeler.
 *
 * `GET /v1/taxpayer/document-tasks` (varsayılan: açık görevler),
 * `GET …/{taskUuid}` (yüklemelerle), `POST …/{taskUuid}/uploads` (multipart
 * `file`: fotoğraf ya da PDF). Yükleme banka hareketine bağlanır ve ofisteki
 * eksik belge uyarısı kendiliğinden kapanır.
 */
import { api } from '@/core/api/client';
import { ApiError } from '@/core/api/envelope';
import type { components } from '@/core/api/schema';

import { postQueuedPhoto } from '../receipts/api';
import type { QueueItem, TaskUploadResult } from '../receipts/queue';
import type { ServerTask } from './tasks';

export const DOCUMENT_TASKS_PATH = '/v1/taxpayer/document-tasks';

type UploadResult = components['schemas']['EnvelopeDocumentTaskUploadResult']['data'];

/** Open and submitted tasks; null when the server has no such endpoint (older version). */
export async function fetchTasks(taxpayer: string): Promise<ServerTask[] | null> {
  try {
    const data = await api.get<{ items: ServerTask[] }>(`${DOCUMENT_TASKS_PATH}?status=all&limit=100`, { taxpayer });
    return data?.items ?? [];
  } catch (e) {
    if (e instanceof ApiError && (e.status === 404 || e.status === 405)) return null;
    throw e;
  }
}

/** One task with its uploads. */
export function fetchTask(taxpayer: string, uuid: string): Promise<ServerTask> {
  return api.get<ServerTask>(`${DOCUMENT_TASKS_PATH}/${uuid}`, { taxpayer });
}

/** Uploads one queued task photo (multipart field `file`). */
export async function uploadTaskPhoto(item: QueueItem): Promise<TaskUploadResult> {
  if (item.target?.type !== 'task') throw new Error('not a task photo');
  const res = await postQueuedPhoto<UploadResult>(item, { path: `${DOCUMENT_TASKS_PATH}/${item.target.taskUuid}/uploads`, field: 'file' });
  return { uploadUuid: res.upload.uuid, matched: res.matched };
}
