#!/usr/bin/env bash
# Enforce minimum per-package statement coverage (default: 100).
# Packages that only contain constants/variables report [no statements] and are skipped.
set -euo pipefail
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"

min="${COVERAGE_MIN:-100}"

fail=0
while IFS= read -r pkg; do
  [[ "$pkg" == *"/integration" ]] && continue
  out="$(go test -cover -covermode=atomic "$pkg" 2>&1)" || true
  if echo "$out" | grep -q 'FAIL'; then
    echo "$out" >&2
    exit 1
  fi
  if echo "$out" | grep -q '\[no statements\]'; then
    continue
  fi
  pct="$(echo "$out" | sed -n 's/.*coverage: \([0-9.]*\)%.*/\1/p')"
  if [[ -z "$pct" ]]; then
    echo "could not parse coverage for $pkg" >&2
    echo "$out" >&2
    exit 1
  fi
  awk -v p="$pct" -v m="$min" 'BEGIN { exit !((p + 0) < (m + 0)) }' && {
    echo "$pkg: ${pct}% (minimum ${min}%)" >&2
    fail=1
  }
done < <(go list ./...)

exit "$fail"
