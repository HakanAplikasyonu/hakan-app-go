import { useQuery } from '@tanstack/react-query';
import { router } from 'expo-router';
import { useMemo } from 'react';

import { useAuth } from '@/core/auth/auth-provider';

import { fetchTasks } from '../receipts/api';
import { useReceipts } from '../receipts/provider';
import { localTasks, mergeTasks, serverTasks, type Task } from './tasks';

export function useTasks() {
  const { taxpayer } = useAuth();
  const { items } = useReceipts();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  const q = useQuery({ queryKey: ['tasks', uuid], queryFn: () => fetchTasks(uuid), enabled: !!uuid });
  const tasks = useMemo(() => mergeTasks(serverTasks(q.data ?? []), localTasks(items, uuid)), [q.data, items, uuid]);
  return { tasks, serverAvailable: q.data !== null, query: q };
}

export function openTask(t: Task) {
  if (t.action.type === 'receipt') router.push({ pathname: '/receipts/[id]', params: { id: t.action.id } });
  else router.push({ pathname: '/capture', params: t.action.kind ? { kind: t.action.kind } : {} });
}

