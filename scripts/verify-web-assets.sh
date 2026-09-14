#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INDEX="$ROOT/web/dist/index.html"
[[ -f "$INDEX" ]] || { echo "Missing $INDEX" >&2; exit 1; }

mapfile -t scripts < <(grep -oE '<script[^>]+src="[^"]+"' "$INDEX" | sed -E 's/.*src="([^"?]+)(\?[^" ]*)?".*/\1/' | sed 's#^/##')
mapfile -t styles < <(grep -oE '<link[^>]+rel="stylesheet"[^>]+href="[^"]+"' "$INDEX" | sed -E 's/.*href="([^"?]+)(\?[^" ]*)?".*/\1/' | sed 's#^/##')

[[ ${#scripts[@]} -gt 0 ]] || { echo "No scripts referenced by index.html" >&2; exit 1; }
[[ ${#styles[@]} -gt 0 ]] || { echo "No stylesheets referenced by index.html" >&2; exit 1; }

check_unique() {
  local kind="$1"; shift
  local dup
  dup="$(printf '%s\n' "$@" | sort | uniq -d)"
  if [[ -n "$dup" ]]; then
    echo "Duplicate $kind references in index.html:" >&2
    printf '%s\n' "$dup" >&2
    return 1
  fi
}
check_unique script "${scripts[@]}"
check_unique stylesheet "${styles[@]}"

combined="$(mktemp -t xport-web-classic.XXXXXXXX.js)"
trap 'rm -f "$combined"' EXIT
: > "$combined"

for rel in "${scripts[@]}"; do
  path="$ROOT/web/dist/$rel"
  [[ -f "$path" ]] || { echo "Missing referenced script: $rel" >&2; exit 1; }
  node --check "$path"
  printf '\n;\n' >> "$combined"
  cat "$path" >> "$combined"
done
node --check "$combined"

for rel in "${styles[@]}"; do
  path="$ROOT/web/dist/$rel"
  [[ -f "$path" ]] || { echo "Missing referenced stylesheet: $rel" >&2; exit 1; }
  while IFS= read -r imported; do
    [[ -z "$imported" ]] && continue
    imported="${imported%%\?*}"
    imported="${imported#/}"
    for loaded in "${styles[@]}"; do
      if [[ "$imported" == "$loaded" ]]; then
        echo "Stylesheet $rel imports $imported, but index.html already loads it directly" >&2
        exit 1
      fi
    done
  done < <(grep -oE '@import[[:space:]]+(url\()?['"'"']?/?[^'"'"') ;]+' "$path" 2>/dev/null | sed -E 's/^@import[[:space:]]+(url\()?['"'"']?//')
done

printf 'Verified %d scripts and %d stylesheets from index.html\n' "${#scripts[@]}" "${#styles[@]}"
