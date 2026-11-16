#!/usr/bin/env bash
# Maestro duman akışlarını açık bir Android emülatöründe koşturur (#300).
# .github/workflows/mobile-e2e.yml içinden, android-emulator-runner'ın
# script adımında çağrılır; yerelde de açık bir emülatörle çalışır.
#
#   scripts/maestro-ci.sh <uygulama klasörü> <apk> <çıktı klasörü>
#
# MAESTRO_EMAIL / MAESTRO_PASSWORD ortamdan gelir (demo seed hesabı).
# Çıktı: report.xml (JUnit), debug/ (Maestro komut günlüğü ve hata anı
# ekran görüntüleri), recordings/ (ekran kaydı, 170 sn'lik parçalar),
# logcat.txt. Çıkış kodu Maestro'nunkidir.
set -uo pipefail

app=${1:?uygulama klasörü}
apk=${2:?apk yolu}
out=${3:?çıktı klasörü}
mkdir -p "$out/recordings" "$out/debug"
maestro=${MAESTRO_BIN:-$HOME/.maestro/bin/maestro}

adb wait-for-device
until [ "$(adb shell getprop sys.boot_completed 2>/dev/null | tr -d '\r')" = "1" ]; do sleep 2; done
# Açılış animasyonları akışları yavaşlatır ve zaman aşımına sokar.
adb shell settings put global window_animation_scale 0
adb shell settings put global transition_animation_scale 0
adb shell settings put global animator_duration_scale 0
# Yeni açılan emülatörde sistem uygulamaları (Pixel Launcher) bir süre yanıt
# vermeyebiliyor; "isn't responding" penceresi uygulamanın üstüne çıkıp ilk
# akışı düşürüyordu (#300, ilk koşu). Hata pencerelerini kapat, sistemi
# oturt, açık kalan pencereyi kapat.
adb shell settings put global hide_error_dialogs 1
adb shell input keyevent KEYCODE_HOME
sleep 30
adb shell am broadcast -a android.intent.action.CLOSE_SYSTEM_DIALOGS >/dev/null 2>&1 || true
adb install -r -g "$apk"
adb logcat -c || true

# screenrecord en çok 180 sn kaydeder: arka arkaya parçalar.
(
  i=0
  while :; do
    i=$((i + 1))
    adb shell screenrecord --time-limit 170 --bit-rate 2000000 "/sdcard/maestro-rec-$(printf %02d "$i").mp4" || break
  done
) &
rec=$!

code=0
"$maestro" test \
  --format junit --output "$out/report.xml" \
  --debug-output "$out/debug" --flatten-debug-output \
  -e MAESTRO_EMAIL="${MAESTRO_EMAIL:?}" -e MAESTRO_PASSWORD="${MAESTRO_PASSWORD:?}" \
  "$app/.maestro" || code=$?

kill "$rec" 2>/dev/null || true
adb shell pkill -INT screenrecord 2>/dev/null || true
sleep 3
for f in $(adb shell ls /sdcard/ 2>/dev/null | tr -d '\r' | grep '^maestro-rec-'); do
  adb pull "/sdcard/$f" "$out/recordings/" >/dev/null 2>&1 || true
done
adb logcat -d >"$out/logcat.txt" 2>/dev/null || true

echo "maestro-ci: çıkış $code ($app)"
exit "$code"
