import { useState } from 'react';
import { Switch, View } from 'react-native';

import { PeriodChips } from '@/components/period-chips';
import { Card, Notice, Text } from '@/components/ui';
import { useColors } from '@/core/theme';

import { useArchiveDocuments } from './api';
import { DocumentList } from './document-list';
import { expiryWarning, isgFilters, isgKindLabels, type ISGKind } from './format';

/**
 * İSG sekmesi (#393): İSG eğitimi, risk değerlendirmesi ve sağlık raporları;
 * geçerlilik bitişi ve yaklaşan bitiş uyarısı (süresi dolan ve 30 gün içinde
 * biten evraklar). "Yalnızca yaklaşanlar" bitişi en yakın olanı önce gösterir.
 */
export function ISGTab({ engagement }: { engagement: string }) {
  const c = useColors();
  const [kind, setKind] = useState<ISGKind | 'all'>('all');
  const [expiring, setExpiring] = useState(false);
  const [fileError, setFileError] = useState<string | null>(null);

  const docs = useArchiveDocuments(engagement, { category: 'isg', isgKind: kind === 'all' ? undefined : kind, expiring });
  // Sayaçlar bütün arşivi kapsar: süzgeç değişse de uyarı aynı kalır.
  const first = docs.data?.pages[0];
  const warning = first ? expiryWarning(first.counts.isg_expired, first.counts.isg_expiring, first.expiring_days) : null;

  return (
    <View style={{ gap: 14 }} testID="isg-tab">
      {warning ? (
        <View testID="isg-warning">
          <Card style={{ borderColor: first && first.counts.isg_expired > 0 ? c.danger : c.warning }}>
            <Text variant="heading" accessibilityRole="header">
              Yaklaşan bitiş uyarısı
            </Text>
            <Text accessibilityLiveRegion="polite">{warning}</Text>
          </Card>
        </View>
      ) : first && first.counts.isg > 0 ? (
        <Text tone="muted">Bütün İSG evrakları geçerli.</Text>
      ) : null}
      <PeriodChips
        periods={isgFilters}
        selected={kind}
        onSelect={(k) => setKind(k as ISGKind | 'all')}
        label={(p) => (p.key === 'all' ? 'Bütün İSG evrakları' : isgKindLabels[p.key as ISGKind])}
        testIDPrefix="isg-kind"
      />
      <View style={{ flexDirection: 'row', alignItems: 'center', gap: 8 }}>
        <Text style={{ flex: 1 }}>Yalnızca süresi dolan ve yaklaşanlar</Text>
        <Switch
          testID="isg-expiring"
          accessibilityLabel="Yalnızca süresi dolan ve yaklaşan evrakları göster"
          value={expiring}
          onValueChange={setExpiring}
          trackColor={{ true: c.brand, false: c.border }}
        />
      </View>
      {fileError ? <Notice>{fileError}</Notice> : null}
      <DocumentList
        engagement={engagement}
        query={docs}
        onError={setFileError}
        empty={
          expiring
            ? { title: 'Yaklaşan bitiş yok', text: 'Süresi dolan ya da 30 gün içinde bitecek İSG evrakı yok.' }
            : { title: 'İSG evrakı yok', text: 'İSG eğitim sertifikaları, risk değerlendirmesi ve sağlık raporları burada görünür.' }
        }
      />
    </View>
  );
}
