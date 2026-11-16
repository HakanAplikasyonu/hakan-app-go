#!/usr/bin/env bash
# Migration numaralarını denetler (#299). golang-migrate sürümü dosya adının
# önekidir (000085_ad.up.sql); iki PR aynı numarayı ayrı ayrı yeşil geçip
# main'e girebiliyordu (000077, 000080).
#
# Hata (çıkış 1):
#   - adı NNNNNN_ad.(up|down).sql kalıbına uymayan dosya
#   - aynı numarayla birden çok migration (farklı adlar)
#   - up'ı olup down'ı olmayan ya da down'ı olup up'ı olmayan migration
#   - --against REF verildiyse: REF'te aynı numarayla başka bir migration var
#     ya da bu dalın REF'te olmayan migration'ı REF'in en yüksek numarasından
#     küçük (veritabanı ileri sürümdeyse hiç uygulanmaz)
# Uyarı (çıkış 0): numaralar arasında boşluk.
#
# Kullanım:
#   scripts/check-migrations.sh [--dir backend/migrations] [--against origin/main]
#   make check-migrations
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
dir=$ROOT/backend/migrations
against=""
while [[ $# -gt 0 ]]; do
  case $1 in
    --dir) dir=$2; shift 2 ;;
    --against) against=$2; shift 2 ;;
    -h|--help) sed -n '2,19p' "${BASH_SOURCE[0]}"; exit 0 ;;
    *) echo "check-migrations: bilinmeyen argüman: $1" >&2; exit 2 ;;
  esac
done
[[ -d $dir ]] || { echo "check-migrations: klasör yok: $dir" >&2; exit 2; }

errors=0
err() { echo "check-migrations: HATA: $*" >&2; errors=$((errors + 1)); }
warn() { echo "check-migrations: uyarı: $*" >&2; }

# Bu daldaki migration'lar: "numara ad" (her migration bir kez).
declare -A name_of=() has_up=() has_down=()
shopt -s nullglob
for f in "$dir"/*; do
  base=$(basename "$f")
  [[ -f $f ]] || continue
  if [[ ! $base =~ ^([0-9]{6})_([a-z0-9_]+)\.(up|down)\.sql$ ]]; then
    err "$base: ad NNNNNN_ad.up.sql / NNNNNN_ad.down.sql kalıbına uymuyor"
    continue
  fi
  num=${BASH_REMATCH[1]} name=${BASH_REMATCH[2]} kind=${BASH_REMATCH[3]}
  key="${num}_${name}"
  if [[ -n ${name_of[$num]:-} && ${name_of[$num]} != "$name" ]]; then
    # Aynı numara, farklı ad: her çakışan çift bir kez raporlanır.
    if [[ $kind == up ]]; then
      err "$num numarası iki kez kullanılmış: ${num}_${name_of[$num]} ve $key. Yeni olanı en yüksek numara +1 yapın."
    fi
  else
    name_of[$num]=$name
  fi
  if [[ $kind == up ]]; then has_up[$key]=1; else has_down[$key]=1; fi
done

for key in "${!has_up[@]}"; do
  [[ -n ${has_down[$key]:-} ]] || err "$key.up.sql var ama $key.down.sql yok"
done
for key in "${!has_down[@]}"; do
  [[ -n ${has_up[$key]:-} ]] || err "$key.down.sql var ama $key.up.sql yok"
done

# Boşluklar: yalnızca uyarı (geçmişte silinip birleştirilen numaralar olabilir).
mapfile -t nums < <(printf '%s\n' "${!name_of[@]}" | sort -n)
gaps=()
prev=0
for n in "${nums[@]}"; do
  v=$((10#$n))
  if ((v > prev + 1)); then
    if ((v == prev + 2)); then gaps+=("$(printf '%06d' $((prev + 1)))"); else gaps+=("$(printf '%06d-%06d' $((prev + 1)) $((v - 1)))"); fi
  fi
  prev=$v
done
((${#gaps[@]} == 0)) || warn "numaralarda boşluk var: ${gaps[*]}"

# Hedef dalla (main) karşılaştırma: aynı numara başka adla orada mı, ya da bu
# dalın yeni migration'ı main'in en yüksek numarasının altında mı kalmış?
if [[ -n $against ]]; then
  rel=${dir#"$ROOT"/}
  if ! git -C "$ROOT" rev-parse --verify --quiet "$against^{commit}" >/dev/null; then
    warn "$against bulunamadı; dal karşılaştırması atlandı (CI'da: git fetch origin main)"
  else
    declare -A ref_name=()
    ref_max=0
    while IFS= read -r path; do
      base=${path##*/}
      [[ $base =~ ^([0-9]{6})_([a-z0-9_]+)\.up\.sql$ ]] || continue
      ref_name[${BASH_REMATCH[1]}]=${BASH_REMATCH[2]}
      v=$((10#${BASH_REMATCH[1]}))
      ((v > ref_max)) && ref_max=$v
    done < <(git -C "$ROOT" ls-tree --name-only "$against" -- "$rel/")
    for n in "${nums[@]}"; do
      here=${name_of[$n]}
      there=${ref_name[$n]:-}
      if [[ -n $there && $there != "$here" ]]; then
        err "$n numarası $against'da ${n}_${there} olarak kullanılıyor; bu dalda ${n}_${here}. $against'ı çekin ve numarayı $(printf '%06d' $((ref_max + 1))) ya da üstü yapın."
      elif [[ -z $there ]] && ((10#$n < ref_max)); then
        err "${n}_${here} $against'da yok ve $against'ın en yüksek numarası $(printf '%06d' "$ref_max"); yeni migration en yüksek numara +1 olmalı ($(printf '%06d' $((ref_max + 1))))."
      fi
    done
  fi
fi

if ((errors > 0)); then
  echo "check-migrations: $errors hata. Kural: migration eklemeden önce main'i çekin, en yüksek numara +1 kullanın." >&2
  exit 1
fi
echo "check-migrations: ok (${#nums[@]} migration, en yüksek ${nums[-1]:-yok})"
