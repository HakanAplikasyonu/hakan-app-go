import { useTheme } from "@hakan/mobile-core/ui/theme";
import { useEffect, useState, type ReactNode } from "react";
import { Linking, Pressable, Text as RNText, View } from "react-native";
import Svg, { Path, Polygon } from "react-native-svg";

import { Button, Screen, Text } from "@/components/ui";
import { env } from "@/core/config/env";
import { secure, StorageKeys } from "@/core/storage";

/** Kristal: sivri tepeli dikdörtgen (taç rozetinin parçası). */
function Kristal({ h, renk }: { h: number; renk: string }) {
  return (
    <Svg width={44} height={h} viewBox={`0 0 44 ${h}`}>
      <Polygon points={`0,14 22,0 44,14 44,${h} 0,${h}`} fill={renk} />
    </Svg>
  );
}

/**
 * Mükellef ilk açılış (#332, Giris.dc.html "Mobil · mükellef ilk açılış"):
 * üç kristal anlatımı (Sen · Beyan · Müşavirin) ve KVKK onayı. Onay
 * verilmeden devam edilmez; bir kez görülür (cihazda saklanır).
 */
export function IlkAcilis({ onDevam }: { onDevam: () => void }) {
  const { colors: c } = useTheme();
  const [onay, setOnay] = useState(false);
  const kristal = (h: number, renk: string, etiket: string) => (
    <View style={{ alignItems: "center", gap: 8 }}>
      <Kristal h={h} renk={renk} />
      <Text variant="caption" tone="muted">
        {etiket}
      </Text>
    </View>
  );
  return (
    <Screen
      style={{
        justifyContent: "center",
        gap: 24,
        maxWidth: 480,
        width: "100%",
        alignSelf: "center",
      }}
    >
      <View
        importantForAccessibility="no-hide-descendants"
        style={{
          flexDirection: "row",
          alignItems: "flex-end",
          justifyContent: "center",
          gap: 14,
        }}
      >
        {kristal(96, c.crystal.light, "Sen")}
        {kristal(132, c.crystal.mid, "Beyan")}
        {kristal(96, c.crystal.dark, "Müşavirin")}
      </View>
      <View style={{ gap: 8 }}>
        <Text variant="title">Fişi çek, gerisini düşünme</Text>
        <Text tone="muted">
          Müşavirin seni davet etti. Çektiğin her fiş müşavirine gider,
          onaylanınca beyanına girer.
        </Text>
      </View>
      <Pressable
        accessibilityRole="checkbox"
        accessibilityState={{ checked: onay }}
        onPress={() => setOnay((v) => !v)}
        testID="ilk-acilis-kvkk"
        style={{
          flexDirection: "row",
          gap: 12,
          alignItems: "flex-start",
          minHeight: 44,
          paddingVertical: 4,
        }}
      >
        <View
          style={{
            width: 24,
            height: 24,
            borderRadius: 6,
            borderWidth: 2,
            borderColor: onay ? c.brand : c.lineStrong,
            backgroundColor: onay ? c.brand : c.surface,
            alignItems: "center",
            justifyContent: "center",
          }}
        >
          {onay ? (
            <Svg width={16} height={16} viewBox="0 0 24 24">
              <Path
                d="M4 12.5l5 5L20 6.5"
                stroke={c.onBrand}
                strokeWidth={2.5}
                fill="none"
                strokeLinecap="round"
                strokeLinejoin="round"
              />
            </Svg>
          ) : null}
        </View>
        <RNText
          style={{
            flex: 1,
            fontFamily: "Inter_400Regular",
            fontSize: 15,
            lineHeight: 22,
            color: c.text,
          }}
        >
          KVKK aydınlatma metnini okudum, verilerimin müşavirimle paylaşılmasını
          onaylıyorum.
        </RNText>
      </Pressable>
      {env.privacyUrl ? (
        <Button
          title="Aydınlatma metnini oku"
          variant="ghost"
          onPress={() => void Linking.openURL(env.privacyUrl as string)}
        />
      ) : null}
      <Button title="Devam et" disabled={!onay} onPress={onDevam} />
    </Screen>
  );
}

/**
 * İlk açılışı bir kez gösterir: cihazda işaret yoksa çocuklar yerine
 * IlkAcilis çizilir. İşaret okunana kadar hiçbir şey çizilmez (kısa).
 */
export function IlkAcilisKapisi({ children }: { children: ReactNode }) {
  const [durum, setDurum] = useState<"yukleniyor" | "goster" | "gec">(
    "yukleniyor",
  );
  useEffect(() => {
    let iptal = false;
    secure
      .get(StorageKeys.onboarding)
      .then((v) => !iptal && setDurum(v ? "gec" : "goster"))
      .catch(() => !iptal && setDurum("goster"));
    return () => {
      iptal = true;
    };
  }, []);
  if (durum === "yukleniyor") return null;
  if (durum === "goster") {
    return (
      <IlkAcilis
        onDevam={() => {
          setDurum("gec");
          void secure.set(StorageKeys.onboarding, new Date().toISOString());
        }}
      />
    );
  }
  return <>{children}</>;
}
