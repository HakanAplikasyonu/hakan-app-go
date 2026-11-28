import { View } from 'react-native';

import { Notice } from '@/components/ui';

import { receiptChecks } from './checks';
import type { ReceiptResult } from './queue';

/**
 * Fiş yapay zekâ kontrolleri: başka biri adına kesilmiş fiş, araçlarda
 * olmayan ya da okunamayan plaka. Önizlemede ve fiş ekranında gösterilir.
 */
export function FisUyarilari({ result }: { result: ReceiptResult | null | undefined }) {
  const checks = receiptChecks(result);
  if (!checks.length) return null;
  return (
    <View style={{ gap: 8 }}>
      {checks.map((c) => (
        <View key={c.code} testID={`receipt-check-${c.code}`}>
          <Notice>{`${c.text}. ${c.hint}`}</Notice>
        </View>
      ))}
    </View>
  );
}
