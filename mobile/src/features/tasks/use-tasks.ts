import { useQuery } from '@tanstack/react-query';
import { router } from 'expo-router';
import { useMemo } from 'react';

import { useAuth } from '@/core/auth/auth-provider';

import { fetchTask, fetchTasks } from './api';
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

/** One document task with its uploads (#200). */
export function useTask(taskUuid: string) {
  const { taxpayer } = useAuth();
  const uuid = taxpayer?.taxpayer.uuid ?? '';
  return useQuery({ queryKey: ['task', uuid, taskUuid], queryFn: () => fetchTask(uuid, taskUuid), enabled: !!uuid && !!taskUuid });
}

export function openTask(t: Task) {
  if (t.action.type === 'receipt') router.push({ pathname: '/receipts/[id]', params: { id: t.action.id } });
  else if (t.action.type === 'task') router.push({ pathname: '/task/[uuid]', params: { uuid: t.action.uuid } });
  else router.push({ pathname: '/capture', params: t.action.kind ? { kind: t.action.kind } : {} });
}

