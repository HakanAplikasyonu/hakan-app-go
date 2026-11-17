import { YetkiYok } from '@hakan/mobile-core/hakan';
import { Segment } from '@hakan/mobile-core/ui/parcalar';
import { Stack, useLocalSearchParams } from 'expo-router';
import { useState } from 'react';

import { Screen, Text } from '@/components/ui';
import { useAuth } from '@/core/auth/auth-provider';
import { visibleTabs, type DocumentTab } from '@/features/documents/tabs';
import { MizanTab } from '@/features/mizan/mizan-tab';
import { PayrollTab } from '@/features/payroll/payroll-tab';

/**
 * Mükellef belgeleri (#393): sekmelerde bordro ve mizan; her sekme kendi okuma
 * iznine bağlı. Belgeler önizlenir ve sistem paylaş menüsüyle paylaşılır.
 */
export default function ClientDocumentsScreen() {
  const { engagement, title, tab } = useLocalSearchParams<{ engagement: string; title?: string; tab?: string }>();
  const { can } = useAuth();
  const initial = visibleTabs(can, tab);
  const [picked, setPicked] = useState<DocumentTab | undefined>(initial.selected);
  const selected = initial.tabs.some((t) => t.key === picked) ? picked : initial.selected;

  return (
    <Screen>
      <Stack.Screen options={{ title: 'Belgeler' }} />
      {title ? (
        <Text variant="caption" tone="muted" numberOfLines={1}>
          {title}
        </Text>
      ) : null}
      {!selected ? (
        <YetkiYok />
      ) : (
        <>
          {initial.tabs.length > 1 ? (
            <Segment secenekler={initial.tabs} secili={selected} onSec={setPicked} testIDOnek="documents-tab" />
          ) : null}
          {selected === 'payroll' ? <PayrollTab engagement={engagement} /> : null}
          {selected === 'mizan' ? <MizanTab engagement={engagement} title={title} /> : null}
        </>
      )}
    </Screen>
  );
}
