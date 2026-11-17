import { useInfiniteQuery, useQuery } from '@tanstack/react-query';

import { api } from '@/core/api/client';
import type { Schemas } from '@/core/api/envelope';
import { useOrganization } from '@/core/auth/auth-provider';
import type { RemoteFile } from '@/features/files/open';

export type ArchiveDocument = Schemas['ArchiveDocument'];
export type ArchivePage = Schemas['ArchiveDocumentPage'];

const PAGE = 50;
const base = (engagement: string) => `/v1/tenant/engagements/${encodeURIComponent(engagement)}/archive-documents`;

export type ArchiveFilter = { category?: string; isgKind?: string; q?: string; expiring?: boolean };

/**
 * Belge arşivi (`GET .../archive-documents`, #393): arama, kategori ve
 * yaklaşan bitiş süzgeci; sayfalar 50'şer. Her sayfa arşiv sayaçlarını taşır.
 */
export function useArchiveDocuments(engagement: string, f: ArchiveFilter, enabled = true) {
  const org = useOrganization();
  return useInfiniteQuery({
    queryKey: ['engagement', org, engagement, 'archive', f.category ?? '', f.isgKind ?? '', f.q ?? '', !!f.expiring],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      api.get<ArchivePage>(base(engagement), {
        organization: org,
        query: { category: f.category, isg_kind: f.isgKind, q: f.q, expiring: f.expiring ? 'true' : undefined, limit: PAGE, offset: pageParam },
      }),
    getNextPageParam: (last) => (last.offset + last.items.length < last.total ? last.offset + last.items.length : undefined),
    enabled,
  });
}

export function archiveFile(engagement: string, d: ArchiveDocument): RemoteFile {
  return {
    path: `${base(engagement)}/${encodeURIComponent(d.uuid)}/file`,
    uuid: d.uuid,
    prefix: 'evrak',
    title: d.title,
    fileName: d.file_name,
    contentType: d.content_type,
  };
}

/** Yalnızca arşiv sayaçları (mükellef ayrıntısındaki İSG uyarısı için tek satırlık istek). */
export function useArchiveCounts(engagement: string, enabled: boolean) {
  const org = useOrganization();
  return useQuery({
    queryKey: ['engagement', org, engagement, 'archive-counts'],
    queryFn: async () => {
      const p = await api.get<ArchivePage>(base(engagement), { organization: org, query: { category: 'isg', limit: 1 } });
      return { ...p.counts, expiringDays: p.expiring_days };
    },
    enabled,
    staleTime: 5 * 60_000,
  });
}
