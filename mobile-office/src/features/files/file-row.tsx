import { Ikon } from '@hakan/mobile-core/ui/ikon';
import { useState } from 'react';
import { ActivityIndicator, Pressable, StyleSheet, View } from 'react-native';

import { Row } from '@/components/ui';
import { useOrganization } from '@/core/auth/auth-provider';
import { useColors } from '@/core/theme';

import { previewFile, shareFile, type RemoteFile } from './open';

/**
 * Belge satırı (#393): dokununca önizleme (PDF görüntüleyici), sağdaki düğme
 * sistem paylaş menüsü. Hata üst bileşene `onError` ile iletilir.
 */
export function FileRow({
  file,
  subtitle,
  trailing,
  onError,
  testID,
}: {
  file: RemoteFile;
  subtitle?: string;
  trailing?: React.ReactNode;
  onError: (message: string | null) => void;
  testID?: string;
}) {
  const org = useOrganization();
  const c = useColors();
  const [busy, setBusy] = useState<'preview' | 'share' | null>(null);

  async function run(kind: 'preview' | 'share') {
    if (busy) return;
    onError(null);
    setBusy(kind);
    try {
      await (kind === 'preview' ? previewFile(org, file) : shareFile(org, file));
    } catch {
      onError('Belge açılamadı. Bağlantınızı kontrol edip yeniden deneyin.');
    } finally {
      setBusy(null);
    }
  }

  return (
    <View testID={testID}>
      <Row
        title={file.title}
        subtitle={subtitle}
        accessibilityLabel={`${file.title}${subtitle ? `, ${subtitle}` : ''}. Önizlemek için dokunun.`}
        onPress={() => void run('preview')}
        leading={busy === 'preview' ? <ActivityIndicator color={c.brand} /> : <Ikon ad="klasor" renk={c.muted} />}
        trailing={
          <View style={styles.trailing}>
            {trailing}
            <Pressable
              accessibilityRole="button"
              accessibilityLabel={`${file.title} belgesini paylaş`}
              hitSlop={10}
              onPress={() => void run('share')}
              style={({ pressed }) => [styles.share, { borderColor: c.border, opacity: pressed ? 0.7 : 1 }]}>
              {busy === 'share' ? <ActivityIndicator color={c.brand} /> : <Ikon ad="yukle" renk={c.brand} />}
            </Pressable>
          </View>
        }
      />
    </View>
  );
}

const styles = StyleSheet.create({
  trailing: { flexDirection: 'row', alignItems: 'center', gap: 8 },
  share: { width: 40, height: 40, borderRadius: 12, borderWidth: 1, alignItems: 'center', justifyContent: 'center' },
});
