# mobile/

Yer tutucu. Mükellef mobil uygulaması burada olacak (D3).

- Yığın: **Expo** (React Native, expo-router). Başlangıç noktası `otopoly-go/mobile` (G10).
- API istemci tipleri `backend/docs/openapi.yaml` dosyasından üretilir ve repoya işlenir (`src/core/api/schema.d.ts`).
- CI'daki `mobile` işi `mobile/package.json` oluşana kadar yeşil olarak atlanır.
