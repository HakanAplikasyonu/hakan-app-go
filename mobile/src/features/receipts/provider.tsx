import { useQueryClient } from '@tanstack/react-query';
import * as Network from 'expo-network';
import { createContext, useCallback, useContext, useEffect, useMemo, useRef, useState, type ReactNode } from 'react';
import { AppState } from 'react-native';

import { errorMessage, isApiError } from '@/core/api/envelope';
import { useAuth } from '@/core/auth/auth-provider';

import { uploadTaskPhoto } from '../tasks/api';
import { sendCorrections, uploadReceipt } from './api';
import {
  addCorrection,
  classify,
  correctionOutcome,
  markCorrections,
  correctionsDue,
  due,
  markFailed,
  markTaskUploaded,
  markUploaded,
  itemPhotos,
  markUploading,
  MAX_PARTS,
  newItem,
  recover,
  retry as retryItem,
  summarize,
  type Correction,
  type QueueItem,
  type QueuePart,
  type QueueSummary,
  type ReceiptKind,
} from './queue';
import { deletePhoto, keepPhoto, loadQueue, saveQueue } from './store';

export type CapturedPhoto = { uri: string; mimeType?: string | null };

type ReceiptsApi = {
  /** This company's items, newest first. */
  items: QueueItem[];
  summary: QueueSummary;
  online: boolean;
  syncing: boolean;
  /**
   * `asParts`: the photos are parça 1, 2, … of one long Z report (one upload).
   * Resolves to the new items' ids (the fiş önizlemesi follows its item).
   */
  enqueue: (kind: ReceiptKind, photos: CapturedPhoto[], asParts?: boolean) => Promise<string[]>;
  /** Queues photos for a document task (#200); uploaded like receipts, also offline. */
  enqueueTask: (taskUuid: string, photos: CapturedPhoto[]) => Promise<void>;
  retry: (id: string) => void;
  remove: (id: string) => void;
  correct: (id: string, field: string, value: string) => void;
  /** Several corrections at once (fiş önizlemesi), sent with one sync. */
  correctMany: (id: string, corrections: Correction[]) => void;
  syncNow: () => void;
};

const Ctx = createContext<ReceiptsApi | null>(null);

const newId = () =>
  `${Date.now().toString(36)}-${Math.random().toString(36).slice(2, 10)}${Math.random().toString(36).slice(2, 6)}`;

/** Fiş kuyruğu: çekilen fotoğrafları saklar, bağlantı geldikçe yükler. */
export function ReceiptsProvider({ children }: { children: ReactNode }) {
  const { taxpayer } = useAuth();
  const queryClient = useQueryClient();
  const taxpayerUuid = taxpayer?.taxpayer.uuid ?? null;
  const [all, setAll] = useState<QueueItem[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [online, setOnline] = useState(true);
  const [syncing, setSyncing] = useState(false);
  const ref = useRef<QueueItem[]>([]);
  const running = useRef(false);

  // Single writer: every change goes through here, then to disk.
  const commit = useCallback((fn: (items: QueueItem[]) => QueueItem[]) => {
    ref.current = fn(ref.current);
    setAll(ref.current);
    void saveQueue(ref.current);
  }, []);
  const replace = useCallback(
    (next: QueueItem) => commit((items) => items.map((i) => (i.id === next.id ? next : i))),
    [commit],
  );

  useEffect(() => {
    loadQueue().then((items) => {
      ref.current = recover(items);
      setAll(ref.current);
      setLoaded(true);
    });
  }, []);

  const sync = useCallback(
    async (force: boolean) => {
      if (!taxpayerUuid || running.current) return;
      running.current = true;
      setSyncing(true);
      try {
        for (const item of due(ref.current, taxpayerUuid, Date.now(), { force })) {
          replace(markUploading(item));
          try {
            const current = () => ref.current.find((i) => i.id === item.id) ?? item;
            if (item.target?.type === 'task') {
              replace(markTaskUploaded(current(), await uploadTaskPhoto(item)));
              void queryClient.invalidateQueries({ queryKey: ['task'] });
              void queryClient.invalidateQueries({ queryKey: ['tasks'] });
            } else {
              replace(markUploaded(current(), await uploadReceipt(item)));
            }
          } catch (e) {
            const status = isApiError(e) ? e.status : 0;
            const current = ref.current.find((i) => i.id === item.id) ?? item;
            replace(markFailed(current, classify(status, isApiError(e) ? e.code : undefined), errorMessage(e), Date.now()));
            if (status === 0) break; // offline: stop, the next network event retries
          }
        }
        for (const item of correctionsDue(ref.current, taxpayerUuid)) {
          try {
            await sendCorrections(item, item.corrections);
            replace(markCorrections(item));
          } catch (e) {
            // Network / server: kept for a later sync. Refused (4xx): shown, not retried.
            if (correctionOutcome(isApiError(e) ? e.status : 0) === 'drop') replace(markCorrections(item, errorMessage(e)));
          }
        }
      } finally {
        running.current = false;
        setSyncing(false);
      }
    },
    [taxpayerUuid, replace, queryClient],
  );

  // Triggers: load / company change, network back, app foreground, a minute timer (backoff).
  useEffect(() => {
    if (!loaded) return;
    void sync(true);
    const net = Network.addNetworkStateListener((s) => {
      const up = s.isConnected !== false && s.isInternetReachable !== false;
      setOnline(up);
      if (up) void sync(true);
    });
    Network.getNetworkStateAsync()
      .then((s) => setOnline(s.isConnected !== false && s.isInternetReachable !== false))
      .catch(() => {});
    const app = AppState.addEventListener('change', (s) => {
      if (s === 'active') void sync(true);
    });
    const timer = setInterval(() => void sync(false), 60_000);
    return () => {
      net.remove();
      app.remove();
      clearInterval(timer);
    };
  }, [loaded, sync]);

  const add = useCallback(
    async (kind: ReceiptKind, photos: CapturedPhoto[], target: QueueItem['target'], asParts = false): Promise<string[]> => {
      if (!taxpayerUuid) return [];
      const now = Date.now();
      const added: QueueItem[] = [];
      let single = photos;
      if (asParts && photos.length > 1) {
        // One upload: parça 1 is the item's photo, the rest go as image_2…
        const id = newId();
        const kept: QueuePart[] = [];
        for (const [n, p] of photos.slice(0, MAX_PARTS).entries()) {
          const mimeType = p.mimeType || 'image/jpeg';
          kept.push({ fileUri: await keepPhoto(p.uri, n ? `${id}-${n + 1}` : id, mimeType), mimeType });
        }
        const [first, ...rest] = kept;
        added.push(newItem({ id, taxpayer: taxpayerUuid, kind, fileUri: first.fileUri, mimeType: first.mimeType, now, target, extraParts: rest }));
        single = [];
      }
      for (const [n, p] of single.entries()) {
        const id = newId();
        const mimeType = p.mimeType || 'image/jpeg';
        const fileUri = await keepPhoto(p.uri, id, mimeType);
        added.push(newItem({ id, taxpayer: taxpayerUuid, kind, fileUri, mimeType, now: now + n, target }));
      }
      commit((items) => [...items, ...added]);
      void sync(true);
      return added.map((i) => i.id);
    },
    [taxpayerUuid, commit, sync],
  );
  const enqueue = useCallback<ReceiptsApi['enqueue']>((kind, photos, asParts) => add(kind, photos, null, asParts), [add]);
  const enqueueTask = useCallback<ReceiptsApi['enqueueTask']>(
    // kind is not used for task photos; the task decides what is expected.
    async (taskUuid, photos) => {
      await add('expense_receipt', photos, { type: 'task', taskUuid });
    },
    [add],
  );

  const retry = useCallback(
    (id: string) => {
      const item = ref.current.find((i) => i.id === id);
      if (!item) return;
      replace(retryItem(item));
      void sync(true);
    },
    [replace, sync],
  );

  const remove = useCallback(
    (id: string) => {
      const item = ref.current.find((i) => i.id === id);
      if (item) itemPhotos(item).forEach(deletePhoto);
      commit((items) => items.filter((i) => i.id !== id));
    },
    [commit],
  );

  const correct = useCallback(
    (id: string, field: string, value: string) => {
      const item = ref.current.find((i) => i.id === id);
      if (!item) return;
      replace(addCorrection(item, { field, value }));
      void sync(true);
    },
    [replace, sync],
  );

  const correctMany = useCallback(
    (id: string, corrections: Correction[]) => {
      const item = ref.current.find((i) => i.id === id);
      if (!item || !corrections.length) return;
      replace(corrections.reduce(addCorrection, item));
      void sync(true);
    },
    [replace, sync],
  );

  const value = useMemo<ReceiptsApi>(() => {
    const items = all.filter((i) => i.taxpayer === taxpayerUuid).sort((a, b) => b.createdAt - a.createdAt);
    return {
      items,
      summary: summarize(all, taxpayerUuid ?? ''),
      online,
      syncing,
      enqueue,
      enqueueTask,
      retry,
      remove,
      correct,
      correctMany,
      syncNow: () => void sync(true),
    };
  }, [all, taxpayerUuid, online, syncing, enqueue, enqueueTask, retry, remove, correct, correctMany, sync]);

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

export function useReceipts(): ReceiptsApi {
  const ctx = useContext(Ctx);
  if (!ctx) throw new Error('useReceipts outside ReceiptsProvider');
  return ctx;
}
