import { Alert } from 'react-native';

import { Button } from '@/components/ui';
import { errorMessage } from '@/core/api/envelope';

import { useRemindTask } from './api';
import { channelsText, SOON_MESSAGE, type Channel } from './channels';

/**
 * Bekleyen belge isteği için "Hatırlat": kanal sorulur, sonuç kısa bir
 * uyarıyla bildirilir. Gönderim ucu henüz yoksa "Bu özellik yakında".
 */
export function RemindButton({ engagement, task, title }: { engagement: string; task: string; title: string }) {
  const remind = useRemindTask(engagement, task);

  async function send(channels: Channel[]) {
    try {
      const r = await remind.mutateAsync(channels);
      if (r === 'soon') Alert.alert('Hatırlatma', SOON_MESSAGE);
      else Alert.alert('Hatırlatma gönderildi', `"${title}" için ${channelsText(channels)} ile hatırlatıldı.`);
    } catch (e) {
      Alert.alert('Hatırlatılamadı', errorMessage(e));
    }
  }

  return (
    <Button
      title="Hatırlat"
      variant="secondary"
      testID="task-remind"
      loading={remind.isPending}
      onPress={() =>
        Alert.alert('Nasıl hatırlatalım?', title, [
          { text: 'Vazgeç', style: 'cancel' },
          { text: 'Uygulama bildirimi', onPress: () => void send(['push']) },
          { text: 'Bildirim, SMS ve e-posta', onPress: () => void send(['push', 'sms', 'email']) },
        ])
      }
    />
  );
}
