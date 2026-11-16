/**
 * Arayüz v1 yazı tipleri (#324): Inter (gövde), Inter Tight (başlık),
 * JetBrains Mono (kod ve sayı etiketi). Uygulamanın kök _layout'u
 * `useFonts(hakanFonts)` ile yükler; adlar tokens.ts `font` ile aynı.
 */
import {
  Inter_400Regular,
  Inter_500Medium,
  Inter_600SemiBold,
  Inter_700Bold,
} from '@expo-google-fonts/inter';
import { InterTight_600SemiBold, InterTight_700Bold, InterTight_800ExtraBold } from '@expo-google-fonts/inter-tight';
import { JetBrainsMono_500Medium, JetBrainsMono_600SemiBold } from '@expo-google-fonts/jetbrains-mono';

export const hakanFonts = {
  Inter_400Regular,
  Inter_500Medium,
  Inter_600SemiBold,
  Inter_700Bold,
  InterTight_600SemiBold,
  InterTight_700Bold,
  InterTight_800ExtraBold,
  JetBrainsMono_500Medium,
  JetBrainsMono_600SemiBold,
};
