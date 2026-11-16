/**
 * Görevler: müşavirin istediği eksik belgeler (sunucu) ve telefondaki fişlerden
 * çıkan işler (yerel). Pure: tested with node --test.
 */
import { fieldLabel, fieldsToCheck, kindLabels } from '../receipts/fields.ts';
import type { components } from '../../core/api/schema';
import type { QueueItem, ReceiptKind } from '../receipts/queue.ts';

/** Server task: `GET /v1/taxpayer/document-tasks` (#200). */
export type ServerTask = components['schemas']['DocumentTask'];

export type TaskAction = { type: 'receipt'; id: string } | { type: 'capture'; kind: ReceiptKind | null } | { type: 'task'; uuid: string };

export type Task = {
  id: string;
  source: 'server' | 'local';
  title: string;
  detail: string;
  action: TaskAction;
  /** YYYY-MM-DD */
  due: string | null;
  overdue?: boolean;
};

export function localTasks(items: readonly QueueItem[], taxpayer: string): Task[] {
  const out: Task[] = [];
  for (const i of items) {
    // Task photos are shown on their task, not as tasks of their own.
    if (i.taxpayer !== taxpayer || i.target) continue;
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

const bankTaskTitles: Record<string, string> = {
  undocumented_payment: 'Ödeme için belge',
  uninvoiced_collection: 'Tahsilat için fatura',
};

/** Görev bir banka hareketine mi bağlı? Elle açılan talepte (#240) hareket yer tutucudur. */
export function isBankTask(t: Pick<ServerTask, 'kind'>): boolean {
  return Object.hasOwn(bankTaskTitles, t.kind);
}

/**
 * Görevin başlığı: banka görevlerinde türün adı, müşavirin elle açtığı
 * talepte (`document_request`) istenen belge. Bilinmeyen tür genel başlık alır.
 */
export function taskTitle(t: Pick<ServerTask, 'kind' | 'title'>): string {
  if (isBankTask(t)) return bankTaskTitles[t.kind];
  const title = t.title?.trim();
  return title || 'Eksik belge';
}

/** Turkish money text: "12.500,00 TRY". */
export function moneyText(amount: string, currency: string): string {
  const n = Number(amount);
  if (!Number.isFinite(n)) return `${amount} ${currency}`;
  const text = new Intl.NumberFormat('tr-TR', { minimumFractionDigits: 2, maximumFractionDigits: 2 }).format(Math.abs(n));
  return `${text} ${currency}`;
}

/** One line about the bank transaction the office is asking about. */
export function transactionLine(t: ServerTask): string {
  const b = t.bank_transaction;
  const who = b.counterparty_name || b.description;
  return [moneyText(b.amount, b.currency), who, b.transaction_date].filter(Boolean).join(' · ');
}

export function serverTasks(list: readonly ServerTask[]): Task[] {
  return list
    .filter((t) => t.status === 'open' || t.status === 'submitted')
    .map((t) => ({
      id: `server:${t.uuid}`,
      source: 'server' as const,
      title: taskTitle(t),
      detail: [isBankTask(t) ? transactionLine(t) : '', t.status === 'submitted' ? 'Gönderildi, müşavir kontrol ediyor' : t.note].filter(Boolean).join('\n'),
      action: { type: 'task' as const, uuid: t.uuid },
      due: t.due_at ? t.due_at.slice(0, 10) : null,
      overdue: t.overdue,
    }));
}

/** Server tasks first (by due date, undated last), then local ones. */
export function mergeTasks(server: readonly Task[], local: readonly Task[]): Task[] {
  const byDue = [...server].sort((a, b) => (a.due ?? '9999').localeCompare(b.due ?? '9999'));
  return [...byDue, ...local];
}

const taskStatusLabels: Record<ServerTask['status'], string> = {
  open: 'Bekliyor',
  submitted: 'Gönderildi',
  matched: 'Tamamlandı',
  cancelled: 'Kapatıldı',
};

export function taskStatusLabel(status: ServerTask['status']): string {
  return taskStatusLabels[status] ?? status;
}
