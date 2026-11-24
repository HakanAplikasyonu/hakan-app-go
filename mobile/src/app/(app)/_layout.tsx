import { Stack } from "expo-router";

import { useColors } from "@/core/theme";
import { IlkAcilisKapisi } from "@/features/onboarding/ilk-acilis";
import { ReceiptsProvider } from "@/features/receipts/provider";

export default function AppLayout() {
  const c = useColors();
  return (
    <IlkAcilisKapisi>
      <ReceiptsProvider>
        <Stack
          screenOptions={{
            headerShown: false,
            contentStyle: { backgroundColor: c.background },
            headerStyle: { backgroundColor: c.background },
            headerTintColor: c.text,
            headerShadowVisible: false,
            headerBackButtonDisplayMode: "minimal",
          }}
        >
          <Stack.Screen name="(tabs)" />
          <Stack.Screen
            name="capture"
            options={{ presentation: "fullScreenModal" }}
          />
          <Stack.Screen
            name="receipts/[id]"
            options={{ headerShown: true, title: "Fiş" }}
          />
          <Stack.Screen
            name="task/[uuid]"
            options={{ headerShown: true, title: "İstek" }}
          />
          <Stack.Screen
            name="notifications"
            options={{ headerShown: true, title: "Bildirimler" }}
          />
          <Stack.Screen
            name="destek"
            options={{ headerShown: true, title: "Destek" }}
          />
          <Stack.Screen
            name="payroll"
            options={{ headerShown: true, title: "Bordro belgeleri" }}
          />
          <Stack.Screen
            name="yabanci-fatura"
            options={{ headerShown: true, title: "Yabancı hizmet faturası" }}
          />
          <Stack.Screen
            name="mesajlar"
            options={{ headerShown: true, title: "Mesajlarım" }}
          />
          <Stack.Screen
            name="araclar"
            options={{ headerShown: true, title: "Araçlarım" }}
          />
        </Stack>
      </ReceiptsProvider>
    </IlkAcilisKapisi>
  );
}
