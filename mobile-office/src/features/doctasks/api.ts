import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import { env } from '@/core/config/env';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';
import { session } from '@/core/auth/session';

export type DocTask = Schemas['DocumentTask'];
type Page = { items: DocTask[]; total: number; limit: number; offset: number };

const base = (engagement: string) => `/v1/tenant/engagements/${engagement}/document-tasks`;

/** Engagement'ın görevleri (`?status=all|open|submitted|...`). */
export function useTasks(engagement: string, status: string) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['doctasks', org, engagement, status],
    queryFn: () => api.get<Page>(base(engagement), { organization: org, query: { status, limit: 50 } }),
  });
}

export function useTask(engagement: string, uuid: string) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['doctask', org, uuid],
    queryFn: () => api.get<DocTask>(`${base(engagement)}/${uuid}`, { organization: org }),
  });
}

function useInvalidate(engagement: string) {
  const org = useOrganization();
  const qc = useQueryClient();
  return (task?: DocTask) => {
    void qc.invalidateQueries({ queryKey: ['doctasks', org, engagement] });
    void qc.invalidateQueries({ queryKey: ['engagement', org, engagement] });
    void qc.invalidateQueries({ queryKey: ['dashboard', org] });
    void qc.invalidateQueries({ queryKey: ['clients', org] });
    if (task) qc.setQueryData(['doctask', org, task.uuid], task);
  };
}

/** Belge talebi aç (`POST .../document-tasks`, #240). Mükellefe bildirim gider. */
export function useCreateRequest(engagement: string) {
  const org = useOrganization();
  const done = useInvalidate(engagement);
  return useMutation({
    mutationFn: (body: { title: string; note?: string; due_days?: number; due_date?: string }) =>
      api.post<DocTask>(base(engagement), body, { organization: org }),
    onSuccess: (t) => done(t),
  });
}

export type TaskAction = { kind: 'complete' } | { kind: 'cancel'; reason?: string } | { kind: 'rerequest'; note: string; due_days: number };

export function useTaskAction(engagement: string, uuid: string) {
  const org = useOrganization();
  const done = useInvalidate(engagement);
  return useMutation({
    mutationFn: (a: TaskAction) => {
      const url = `${base(engagement)}/${uuid}/${a.kind}`;
      if (a.kind === 'rerequest') return api.post<DocTask>(url, { note: a.note, due_days: a.due_days }, { organization: org });
      if (a.kind === 'cancel') return api.post<DocTask>(url, a.reason ? { reason: a.reason } : undefined, { organization: org });
      return api.post<DocTask>(url, undefined, { organization: org });
    },
    onSuccess: (t) => done(t),
  });
}

/** Yüklenen dosya (görsel önizleme), oturum tokenıyla. */
export function uploadFileSource(org: string, engagement: string, task: string, upload: string) {
  const token = session.get()?.accessToken;
  return {
    uri: `${env.apiUrl}${base(engagement)}/${task}/uploads/${upload}/file`,
    headers: { 'X-Organization': org, ...(token ? { Authorization: `Bearer ${token}` } : {}) },
    cacheKey: `doctask-upload-${upload}`,
  };
}
