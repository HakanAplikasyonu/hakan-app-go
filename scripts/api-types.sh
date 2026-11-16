#!/usr/bin/env bash
# API tipleri (#276): backend/docs/openapi.yaml'dan üç istemcinin işlenmiş
# tiplerini tek komutla üretir ya da güncel mi diye denetler.
#
#   scripts/api-types.sh           üret (make api-types)
#   scripts/api-types.sh --check   üret ve işlenmiş dosyalardan farklıysa 1 ile çık
#
# Üretici openapi-typescript; sürüm mobile/package.json'daki sabit sürüm. Çıktı
# sürüme bağlı olduğu için mobile-office'in sabit sürümü ve frontend'in kilit
# dosyasındaki çözümlenmiş sürüm aynı olmalı, değilse betik durur.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
cd "$ROOT"
check=0
[[ ${1:-} == --check ]] && check=1

SPEC=backend/docs/openapi.yaml
# hedef dosya | sahibi (sürüm denetimi için)
TARGETS=(
  "frontend/src/generated/api.d.ts|frontend"
  "mobile/src/core/api/schema.d.ts|mobile"
  "mobile-office/src/core/api/schema.d.ts|mobile-office"
)

pin() { node -e 'const p=require(process.argv[1]);const v=(p.devDependencies||{})["openapi-typescript"]||(p.dependencies||{})["openapi-typescript"]||"";process.stdout.write(v)' "$ROOT/$1/package.json"; }

version=$(pin mobile)
[[ $version =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "api-types: mobile/package.json openapi-typescript sabit sürüm olmalı (bulunan: '$version')" >&2; exit 1; }
office=$(pin mobile-office)
[[ $office == "$version" ]] || { echo "api-types: mobile-office openapi-typescript $office, mobile $version: aynı sürüme sabitleyin" >&2; exit 1; }
front=$(sed -n '/^  openapi-typescript@[0-9]/{s/^  openapi-typescript@\([0-9.]*\):.*/\1/p;q}' frontend/pnpm-lock.yaml)
[[ $front == "$version" ]] || { echo "api-types: frontend kilit dosyasında openapi-typescript $front, mobile $version: aynı sürüme getirin" >&2; exit 1; }

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
# Projelerin node_modules'ü gerekmesin: sabit sürümü geçici dizinde çalıştır.
gen="$tmp/gen.d.ts"
(cd "$tmp" && npx --yes "openapi-typescript@$version" "$ROOT/$SPEC" -o "$gen" >/dev/null)

stale=()
for entry in "${TARGETS[@]}"; do
  file=${entry%%|*}
  if ((check)); then
    cmp -s "$gen" "$file" || stale+=("$file")
  else
    cp "$gen" "$file"
  fi
done

if ((check)); then
  if ((${#stale[@]})); then
    echo "api-types: $SPEC ile uyuşmayan tipler (openapi-typescript $version):" >&2
    printf '  %s\n' "${stale[@]}" >&2
    echo "  düzeltme: make api-types (ya da scripts/api-types.sh) ve dosyaları işleyin" >&2
    exit 1
  fi
  echo "api-types: ok (openapi-typescript $version, ${#TARGETS[@]} dosya)"
else
  echo "api-types: ${#TARGETS[@]} dosya üretildi (openapi-typescript $version)"
fi
