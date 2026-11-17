#!/usr/bin/env bash
# Tests for scripts/check-migrations.sh (#299). Run: scripts/check-migrations.test.sh
# Works on temp copies and a temp git repo; the repository is not modified.
set -euo pipefail

ROOT=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
CHK=$ROOT/scripts/check-migrations.sh
T=$(mktemp -d)
trap 'rm -rf "$T"' EXIT

pass=0
ok() { pass=$((pass + 1)); echo "ok  - $*"; }
fail() { echo "FAIL - $*" >&2; exit 1; }
# expect <code> <grep pattern> <args...>
expect() {
  local want=$1 pattern=$2 code=0
  shift 2
  "$CHK" "$@" >"$T/out" 2>&1 || code=$?
  [[ $code == "$want" ]] || { cat "$T/out" >&2; fail "expected exit $want, got $code ($*)"; }
  grep -qE -- "$pattern" "$T/out" || { cat "$T/out" >&2; fail "output lacks /$pattern/"; }
}
mig() { # mig <dir> <num_name> [up|down|both]
  local d=$1 n=$2 k=${3:-both}
  mkdir -p "$d"
  [[ $k == down ]] || echo "SELECT 1;" >"$d/$n.up.sql"
  [[ $k == up ]] || echo "SELECT 1;" >"$d/$n.down.sql"
}

expect 0 "check-migrations: ok" --dir "$ROOT/backend/migrations"
ok "passes on the repository"

D=$T/clean
mig "$D" 000001_init; mig "$D" 000002_users; mig "$D" 000003_orgs
expect 0 "ok \(3 migration, en yüksek 000003\)" --dir "$D"
grep -q "uyarı" "$T/out" && fail "no warning expected for contiguous numbers"
ok "contiguous numbers pass without a warning"

D=$T/gap
mig "$D" 000001_init; mig "$D" 000004_x; mig "$D" 000006_y
expect 0 "boşluk var: 000002-000003 000005" --dir "$D"
ok "gaps warn but pass"

D=$T/dup
mig "$D" 000001_init; mig "$D" 000002_invoices; mig "$D" 000002_receipts
expect 1 "000002 numarası iki kez kullanılmış: 000002_invoices ve 000002_receipts" --dir "$D"
ok "a duplicate number fails"

D=$T/nodown
mig "$D" 000001_init; mig "$D" 000002_x up
expect 1 "000002_x.up.sql var ama 000002_x.down.sql yok" --dir "$D"
D=$T/noup
mig "$D" 000001_init; mig "$D" 000002_x down
expect 1 "000002_x.down.sql var ama 000002_x.up.sql yok" --dir "$D"
ok "a missing up or down fails"

D=$T/badname
mig "$D" 000001_init; echo x >"$D/2_Bad-Name.up.sql"
expect 1 "2_Bad-Name.up.sql: ad .* kalıbına uymuyor" --dir "$D"
ok "a badly named file fails"

# Against a branch: main's migrations are committed in a temp repo; the work
# tree holds the branch's migrations (the check reads files + the ref's tree).
G=$T/repo
git init -q -b main "$G"
mkdir -p "$G/scripts"
cp "$CHK" "$G/scripts/"
GCHK=$G/scripts/check-migrations.sh
commit_main() { # commit_main <num_name...>: main's migrations
  rm -rf "$G/backend/migrations"
  for n in "$@"; do mig "$G/backend/migrations" "$n"; done
  git -C "$G" add -A && git -C "$G" -c user.name=t -c user.email=t@t commit -qm main --allow-empty
}
branch_tree() { # branch_tree <num_name...>: the branch's migrations (work tree)
  rm -rf "$G/backend/migrations"
  for n in "$@"; do mig "$G/backend/migrations" "$n"; done
}
against() { # against <code> <pattern>
  local want=$1 pattern=$2 code=0
  "$GCHK" --against main >"$T/out" 2>&1 || code=$?
  [[ $code == "$want" ]] && grep -qE -- "$pattern" "$T/out" || { cat "$T/out" >&2; fail "against main: want $want /$pattern/"; }
}

commit_main 000001_init 000002_users 000003_orgs 000004_from_main
branch_tree 000001_init 000002_users 000003_orgs 000004_from_branch
against 1 "000004 numarası main'da 000004_from_main olarak kullanılıyor; bu dalda 000004_from_branch.*000005"
"$GCHK" >"$T/out" 2>&1 || { cat "$T/out" >&2; fail "without --against the branch alone passes"; }
ok "the same number taken on main fails with --against (alone it passes)"

branch_tree 000001_init 000002_users 000003_orgs 000004_from_main 000005_from_branch
against 0 "check-migrations: ok"
ok "a branch at main's max+1 passes"

branch_tree 000001_init 000002_renamed 000003_orgs 000004_from_main
against 1 "000002 numarası main'da 000002_users"
ok "taking a number main uses under another name fails"

commit_main 000001_init 000002_users 000004_from_main
branch_tree 000001_init 000002_users 000003_late 000004_from_main
against 1 "000003_late main'da yok ve main'ın en yüksek numarası 000004"
ok "a branch-only migration below main's highest number fails"

# İki PR aynı numarayla birleşince main'de çift numara kalır; düzelten dal
# birini yeni numaraya taşır ve diğerini yerinde bırakabilir.
commit_main 000001_init 000002_a_first 000002_b_second
branch_tree 000001_init 000002_a_first 000003_b_second
against 0 "check-migrations: ok"
branch_tree 000001_init 000002_b_second 000003_a_first
against 0 "check-migrations: ok"
branch_tree 000001_init 000002_c_other 000003_b_second
against 1 "000002 numarası main'da 000002_a_first ve 000002_b_second olarak kullanılıyor"
ok "a number duplicated on main accepts either of its names"

code=0; "$GCHK" --against no-such-ref >"$T/out" 2>&1 || code=$?
[[ $code == 0 ]] && grep -q "no-such-ref bulunamadı" "$T/out" || { cat "$T/out" >&2; fail "missing ref must warn and pass"; }
ok "a missing ref warns and skips the comparison"

echo "check-migrations.test: $pass passed"
