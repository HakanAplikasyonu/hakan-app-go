import { BosDurum, KristalYukleyici } from '@hakan/mobile-core/hakan';
import { StyleSheet, View } from 'react-native';

import { LoadError } from '@/components/load-error';
import { Button, Chip, Text } from '@/components/ui';
import { FileRow } from '@/features/files/file-row';

import { archiveFile, type useArchiveDocuments } from './api';
import { documentSubtitle, expiryChip } from './format';

/** Arşiv ve İSG sekmelerinin ortak listesi: satırlar, boş / hata / daha fazla. */
export function DocumentList({
  engagement,
  query,
  empty,
  onError,
}: {
  engagement: string;
  query: ReturnType<typeof useArchiveDocuments>;
  empty: { title: string; text: string };
  onError: (message: string | null) => void;
}) {
  const items = query.data?.pages.flatMap((p) => p.items) ?? [];
  if (query.isLoading) {
    return (
      <View style={styles.center}>
        <KristalYukleyici etiket="Evraklar yükleniyor" />
      </View>
    );
  }
  if (query.error && items.length === 0) {
    return <LoadError title="Evraklar alınamadı" error={query.error} retry={query.refetch} retrying={query.isRefetching} />;
  }
  if (items.length === 0) return <BosDurum baslik={empty.title} aciklama={empty.text} />;
  const total = query.data?.pages[0]?.total ?? items.length;
  return (
    <View style={{ gap: 8 }}>
      <Text variant="caption" tone="muted" accessibilityLiveRegion="polite">
        {total} evrak
      </Text>
      {items.map((d) => {
        const chip = expiryChip(d);
        return (
          <FileRow
            key={d.uuid}
            testID="archive-document"
            file={archiveFile(engagement, d)}
            subtitle={[documentSubtitle(d), chip?.label].filter(Boolean).join(' · ')}
            trailing={chip && chip.tone !== 'success' ? <Chip tone={chip.tone} label={chip.tone === 'danger' ? 'Doldu' : 'Yaklaşıyor'} /> : undefined}
            onError={onError}
          />
        );
      })}
      {query.hasNextPage ? (
        <Button
          title="Daha fazla göster"
          variant="secondary"
          loading={query.isFetchingNextPage}
          onPress={() => void query.fetchNextPage()}
        />
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  center: { alignItems: 'center', paddingVertical: 40 },
});
