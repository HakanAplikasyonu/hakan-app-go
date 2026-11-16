#!/bin/sh
# Egress vekilini başlatır (issue #256): EGRESS_PROXY_HOSTS'taki hostlardan
# izin listesini yazar ve squid'i ön planda çalıştırır.
#   EGRESS_PROXY_HOSTS  virgül ya da boşlukla ayrılmış host adları
#                       (".ornek.com.tr" alt alan adlarını da kapsar)
set -eu
hosts=$(printf '%s' "${EGRESS_PROXY_HOSTS:-}" | tr ',' ' ')
list=${SQUID_ALLOWED_HOSTS_FILE:-/etc/squid/allowed_hosts}
: >"$list"
for h in $hosts; do
  case $h in
    *[!a-zA-Z0-9.-]* | "" | .) echo "egress-proxy: invalid host '$h' in EGRESS_PROXY_HOSTS" >&2; exit 1 ;;
  esac
  printf '%s\n' "$h" >>"$list"
done
[ -s "$list" ] || { echo "egress-proxy: EGRESS_PROXY_HOSTS is empty" >&2; exit 1; }
echo "egress-proxy: allowing $(tr '\n' ' ' <"$list")"
logs=/var/log/squid
mkdir -p "$logs"
touch "$logs/access.log" "$logs/cache.log"
chown -R proxy:proxy "$logs" 2>/dev/null || true
tail -F -n 0 "$logs/access.log" "$logs/cache.log" &
exec squid -N -f "${SQUID_CONF:-/etc/squid/hakan.conf}"
