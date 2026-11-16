#!/usr/bin/env bash
# Yeni ofis açma (#273): platform yöneticisi bir mali müşavirlik ofisi açar ve
# ofisin sahibini e-postayla davet eder. Sahip daveti kabul edip ilk kez
# girdiğinde panelde kurulum sihirbazı (/t/<slug>/setup) karşılar: ofis
# bilgileri, ekip, Excel'den mükellef listesi, Luca / Zirve, entegratör
# bağlantıları ve mükelleflere toplu davet.
#
#   scripts/new-office.sh --name "Örnek Mali Müşavirlik" --slug ornek-smmm \
#                         --owner-email sahip@ornek.test
#
# Ortam:
#   API_URL         Go API kökü (varsayılan http://127.0.0.1:8080)
#   ADMIN_EMAIL     platform yöneticisinin e-postası (yoksa sorulur)
#   ADMIN_PASSWORD  parolası (yoksa gizli olarak sorulur; komut satırına
#                   yazılmaz, kabuk geçmişine düşmesin)
#   ADMIN_TOTP      yöneticide doğrulayıcı uygulaması açıksa altı haneli kod
#                   (yoksa ve sunucu isterse sorulur)
#
# Gerekenler: curl, jq. Çıktı: ofisin uuid'i ve slug'ı, davet edilen e-posta.
# Yönetici yeni ofisin ilk sahibi olarak kalır (POST /v1/platform/organizations
# varsayılanı); sahip katıldıktan sonra üyeler sayfasından çıkarılabilir.
set -euo pipefail

die() { echo "new-office: $*" >&2; exit 1; }
usage() { sed -n '2,24p' "$0" | sed 's/^# \{0,1\}//'; exit "${1:-0}"; }

name= slug= owner=
while (($#)); do
  case $1 in
    --name) name=${2:-}; shift 2 ;;
    --slug) slug=${2:-}; shift 2 ;;
    --owner-email) owner=${2:-}; shift 2 ;;
    -h|--help) usage 0 ;;
    *) echo "new-office: bilinmeyen seçenek: $1" >&2; usage 2 >&2 ;;
  esac
done
[[ -n $name ]] || die "--name gerekli"
[[ -n $owner ]] || die "--owner-email gerekli"
[[ $owner =~ ^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$ ]] || die "--owner-email geçerli bir e-posta değil: $owner"
if [[ -n $slug && ! $slug =~ ^[a-z0-9]([a-z0-9-]{0,62}[a-z0-9])?$ ]]; then
  die "--slug küçük harf, rakam ve tire olmalı (örnek: ornek-smmm)"
fi
command -v curl >/dev/null || die "curl bulunamadı"
command -v jq >/dev/null || die "jq bulunamadı"

API=${API_URL:-http://127.0.0.1:8080}
API=${API%/}
email=${ADMIN_EMAIL:-}
password=${ADMIN_PASSWORD:-}
if [[ -z $email ]]; then read -r -p "Platform yöneticisi e-postası: " email; fi
if [[ -z $password ]]; then read -r -s -p "Parola: " password; echo; fi
[[ -n $email && -n $password ]] || die "yönetici e-postası ve parolası gerekli"

# call METHOD PATH [JSON] [extra curl args...]: prints "status body".
call() {
  local method=$1 path=$2 data=${3:-}
  shift $(( $# < 3 ? $# : 3 ))
  local args=(-sS -X "$method" -H 'Accept: application/json' -w '\n%{http_code}' "$@")
  [[ -n $data ]] && args+=(-H 'Content-Type: application/json' --data-binary @-)
  local out
  out=$(printf '%s' "$data" | curl "${args[@]}" "$API$path") || die "$API ulaşılamadı"
  printf '%s %s' "${out##*$'\n'}" "${out%$'\n'*}"
}
code_of() { jq -r '.error.code // empty' <<<"$1" 2>/dev/null; }

login_body=$(jq -n --arg e "$email" --arg p "$password" --arg t "${ADMIN_TOTP:-}" \
  '{email: $e, password: $p} + (if $t == "" then {} else {totp_code: $t} end)')
read -r status body <<<"$(call POST /v1/auth/login "$login_body")" || true
body=${body:-}
if [[ $status == 403 && $(code_of "$body") == MFA_REQUIRED && -z ${ADMIN_TOTP:-} ]]; then
  read -r -p "Doğrulayıcı kodu: " totp
  login_body=$(jq -c --arg t "$totp" '. + {totp_code: $t}' <<<"$login_body")
  read -r status body <<<"$(call POST /v1/auth/login "$login_body")" || true
fi
[[ $status == 200 ]] || die "giriş başarısız ($status $(code_of "$body"))"
token=$(jq -r '.data.access_token // empty' <<<"$body")
[[ -n $token ]] || die "giriş yanıtında erişim belirteci yok"
auth=(-H "Authorization: Bearer $token")

org_body=$(jq -n --arg n "$name" --arg s "$slug" '{name: $n} + (if $s == "" then {} else {slug: $s} end)')
read -r status body <<<"$(call POST /v1/platform/organizations "$org_body" "${auth[@]}")" || true
case $status in
  201) ;;
  403) die "ofis açılamadı: bu hesap platform yöneticisi değil ($(code_of "$body"))" ;;
  409) die "ofis açılamadı: '$slug' slug'ı kullanılıyor" ;;
  *) die "ofis açılamadı ($status $(code_of "$body"))" ;;
esac
org_uuid=$(jq -r '.data.uuid' <<<"$body")
org_slug=$(jq -r '.data.slug' <<<"$body")

invite_body=$(jq -n --arg e "$owner" '{email: $e, role_key: "owner"}')
read -r status body <<<"$(call POST /v1/tenant/invitations "$invite_body" "${auth[@]}" -H "X-Organization: $org_slug")" || true
[[ $status == 201 || $status == 200 ]] || die "ofis açıldı ($org_slug, $org_uuid) ama sahip davet edilemedi ($status $(code_of "$body")); üyeler sayfasından davet edin"

echo "Ofis açıldı: $name"
echo "  slug:  $org_slug"
echo "  uuid:  $org_uuid"
echo "  sahip: $owner (davet e-postası gönderildi; kabul edince kurulum sihirbazı /t/$org_slug/setup açılır)"
