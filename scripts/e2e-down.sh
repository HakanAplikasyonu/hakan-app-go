#!/usr/bin/env bash
# scripts/e2e-up.sh ile kurulan ortamı kapatır: API ve frontend süreçleri
# durur, compose birimleriyle birlikte silinir.
set -uo pipefail

ROOT="$(cd "$(dirname "$0")/.." && pwd)"
DIR="${E2E_DIR:-$ROOT/e2e/.run}"

for name in frontend server; do
  if [ -f "$DIR/$name.pid" ]; then
    pid="$(cat "$DIR/$name.pid")"
    # e2e-up süreçleri setsid ile başlatır: grubu (next-server dahil) kapat.
    kill -- "-$pid" 2>/dev/null || kill "$pid" 2>/dev/null
    rm -f "$DIR/$name.pid"
  fi
done

if [ -f "$DIR/compose-args" ]; then
  eval "args=($(cat "$DIR/compose-args"))"
  docker compose "${args[@]}" down -v --remove-orphans
fi
