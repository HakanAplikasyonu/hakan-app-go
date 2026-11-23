import { View } from 'react-native';

import { Chip, Row } from '@/components/ui';

import type { MonthItem } from './api';
import { amountLabel, approvalLabel, approvalTone, bankMatchLabel, dateLabel, matchStatusTone } from './format';

/** Dönem listesinin satırı (fatura, Z raporu, masraf, banka hareketi); dönem ekranı ve mükellef sekmeleri ortak. */
export function ItemRow({ entry, onPress }: { entry: MonthItem; onPress: () => void }) {
  switch (entry.kind) {
    case 'invoices': {
      const inv = entry.item;
      return (
        <Row
          title={inv.counterparty_name || inv.invoice_number}
          subtitle={`${inv.direction === 'incoming' ? 'Gelen' : 'Giden'} · ${dateLabel(inv.issue_date)} · ${amountLabel(inv.payable_amount, inv.currency)}`}
          trailing={
            <Chips
              items={[
                inv.status === 'cancelled' ? { label: 'İptal', tone: 'danger' as const } : null,
                inv.open_warnings ? { label: `${inv.open_warnings} uyarı`, tone: 'warning' as const } : null,
              ]}
            />
          }
          onPress={onPress}
        />
      );
    }
    case 'zreports': {
      const z = entry.item;
      return (
        <Row
          title={z.z_report_number ? `Z raporu ${z.z_report_number}` : 'Z raporu'}
          subtitle={`${dateLabel(z.sale_date)} · ${amountLabel(z.total_amount, z.currency)}${z.okc_serial_number ? ` · ÖKC ${z.okc_serial_number}` : ''}`}
          trailing={
            <Chips
              items={[
                { label: approvalLabel[z.approval_status], tone: approvalTone[z.approval_status] },
                z.review_fields.length ? { label: 'Kontrol et', tone: 'warning' as const } : null,
              ]}
            />
          }
          onPress={onPress}
        />
      );
    }
    case 'expenses': {
      const e = entry.item;
      return (
        <Row
          title={e.vendor_name || e.description || 'Masraf'}
          subtitle={`${dateLabel(e.expense_date)} · ${amountLabel(e.amount, e.currency)}${e.account_code ? ` · ${e.account_code}` : ''}`}
          trailing={
            <Chips
              items={[
                { label: approvalLabel[e.approval_status], tone: approvalTone[e.approval_status] },
                e.review_fields.length ? { label: 'Kontrol et', tone: 'warning' as const } : null,
              ]}
            />
          }
          onPress={onPress}
        />
      );
    }
    case 'bank': {
      const t = entry.item;
      return (
        <Row
          title={t.counterparty_name || t.description || 'Banka hareketi'}
          subtitle={`${dateLabel(t.transaction_date)} · ${t.direction === 'in' ? '+' : '−'}${amountLabel(t.amount, t.currency)} · ${t.cash_box_name}`}
          trailing={<Chips items={[{ label: bankMatchLabel(t), tone: t.booked_account_code && t.match_status === 'unmatched' ? 'success' : matchStatusTone[t.match_status] }]} />}
          onPress={onPress}
        />
      );
    }
  }
}

type ChipItem = { label: string; tone: 'neutral' | 'brand' | 'success' | 'warning' | 'danger' } | null;

function Chips({ items }: { items: ChipItem[] }) {
  const shown = items.filter((x): x is NonNullable<ChipItem> => !!x);
  if (!shown.length) return null;
  return (
    <View style={{ gap: 4, alignItems: 'flex-end' }}>
      {shown.map((x) => (
        <Chip key={x.label} label={x.label} tone={x.tone} />
      ))}
    </View>
  );
}
