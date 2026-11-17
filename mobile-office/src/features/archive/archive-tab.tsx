import { useEffect, useState } from 'react';
import { View } from 'react-native';

import { PeriodChips } from '@/components/period-chips';
import { Field, Notice } from '@/components/ui';
import { normalizeSearch } from '@/features/clients/format';

import { useArchiveDocuments } from './api';
import { DocumentList } from './document-list';
import { archiveFilters } from './format';

/**
 * Evrak sekmesi (#393): mükellefin geçmiş evrakları (sözleşme, vergi, SGK,
 * ticaret sicil, İSG...), başlık / dosya adı / notta arama ve kategori
 * süzgeci. Satır önizler, düğme paylaşır.
 */
export function ArchiveTab({ engagement }: { engagement: string }) {
  const [text, setText] = useState('');
  const [q, setQ] = useState<string | undefined>();
  const [category, setCategory] = useState('all');
  const [fileError, setFileError] = useState<string | null>(null);

  useEffect(() => {
    const t = setTimeout(() => setQ(normalizeSearch(text)), 300);
    return () => clearTimeout(t);
  }, [text]);

  const docs = useArchiveDocuments(engagement, { category: category === 'all' ? undefined : category, q });

  return (
    <View style={{ gap: 14 }} testID="archive-tab">
      <Field
        label="Evrak ara"
        placeholder="Başlık, dosya adı ya da not"
        value={text}
        onChangeText={setText}
        autoCorrect={false}
        returnKeyType="search"
        testID="archive-search"
      />
      <PeriodChips
        periods={archiveFilters}
        selected={category}
        onSelect={setCategory}
        label={(p) => `${p.label} evrakları`}
        testIDPrefix="archive-category"
      />
      {fileError ? <Notice>{fileError}</Notice> : null}
      <DocumentList
        engagement={engagement}
        query={docs}
        onError={setFileError}
        empty={
          q || category !== 'all'
            ? { title: 'Eşleşen evrak yok', text: 'Aramayı ya da kategoriyi değiştirin.' }
            : { title: 'Arşivde evrak yok', text: 'Ofisin bu mükellef için sakladığı sözleşme, vergi, SGK ve İSG evrakları burada görünür.' }
        }
      />
    </View>
  );
}
