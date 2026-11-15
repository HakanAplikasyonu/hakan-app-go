/**
 * Görevler: müşavirin istediği eksik belgeler (sunucu) ve telefondaki fişlerden
 * çıkan işler (yerel). Pure: tested with node --test.
 */
import { fieldLabel, fieldsToCheck, kindLabels } from '../receipts/fields.ts';
import type { QueueItem, ReceiptKind } from '../receipts/queue.ts';

/** Server task (proposed `GET /v1/taxpayer/tasks`, see the PR's decision list). */
export type ServerTask = {
  uuid: string;
  kind: 'missing_document' | string;
  title: string;
  description?: string | null;
  due_date?: string | null;
  /** For missing documents: what to photograph. */
  document_kind?: ReceiptKind | null;
};

export type TaskAction = { type: 'receipt'; id: string } | { type: 'capture'; kind: ReceiptKind | null };

export type Task = {
  id: string;
  source: 'server' | 'local';
  title: string;
  detail: string;
  action: TaskAction;
  /** YYYY-MM-DD */
  due: string | null;
};

export function localTasks(items: readonly QueueItem[], taxpayer: string): Task[] {
  const out: Task[] = [];
  for (const i of items) {
    if (i.taxpayer !== taxpayer) continue;
    if (i.status === 'failed') {
      out.push({
        id: `failed:${i.id}`,
        source: 'local',
        title: `${kindLabels[i.kind]} gönderilemedi`,
        detail: i.lastError ? `Sunucu: ${i.lastError}` : 'Tekrar deneyin ya da yeniden çekin.',
        action: { type: 'receipt', id: i.id },
        due: null,
      });
      continue;
    }
    const check = fieldsToCheck(i);
    if (check.length) {
      out.push({
        id: `check:${i.id}`,
        source: 'local',
        title: `${kindLabels[i.kind]}: okunanı kontrol edin`,
        detail: `${check.map(fieldLabel).join(', ')} emin olunamadı.`,
        action: { type: 'receipt', id: i.id },
        due: null,
      });
    }
  }
  return out;
}

export function serverTasks(list: readonly ServerTask[]): Task[] {
  return list.map((t) => ({
    id: `server:${t.uuid}`,
    source: 'server',
    title: t.title,
    detail: t.description ?? '',
    action: { type: 'capture', kind: t.document_kind ?? null },
    due: t.due_date ?? null,
  }));
}

/** Server tasks first (by due date, undated last), then local ones. */
export function mergeTasks(server: readonly Task[], local: readonly Task[]): Task[] {
  const byDue = [...server].sort((a, b) => (a.due ?? '9999').localeCompare(b.due ?? '9999'));
  return [...byDue, ...local];
}
