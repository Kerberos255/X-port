#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
INDEX="$ROOT/web/dist/index.html"
DIST="$ROOT/web/dist"
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

# The versioned patch stack is being retired. Keep existing compatibility layers
# while preventing new v12/v13/... files from becoming another permanent layer.
for rel in "${scripts[@]}" "${styles[@]}"; do
  base="$(basename "$rel")"
  if [[ "$base" =~ ^v([1-9][0-9]+)\.(js|css)$ ]] && (( BASH_REMATCH[1] >= 12 )); then
    echo "Do not add new versioned frontend patch layers: $rel" >&2
    exit 1
  fi
done

combined="$(mktemp -t xport-web-classic.XXXXXXXX.js)"
trap 'rm -f "$combined"' EXIT
: > "$combined"

for rel in "${scripts[@]}"; do
  path="$DIST/$rel"
  [[ -f "$path" ]] || { echo "Missing referenced script: $rel" >&2; exit 1; }
  node --check "$path"
  printf '\n;\n' >> "$combined"
  cat "$path" >> "$combined"
done
node --check "$combined"

for rel in "${styles[@]}"; do
  path="$DIST/$rel"
  [[ -f "$path" ]] || { echo "Missing referenced stylesheet: $rel" >&2; exit 1; }
done

# A leftover vN asset after its link/script is removed is easy to forget and
# makes the next refactor ambiguous. Every remaining versioned asset must be live.
mapfile -t versioned_assets < <(find "$DIST" -maxdepth 1 -type f -regextype posix-extended -regex '.*/v[0-9]+\.(js|css)' -printf '%f\n' | sort)
for asset in "${versioned_assets[@]}"; do
  found=false
  for rel in "${scripts[@]}" "${styles[@]}"; do
    if [[ "$(basename "$rel")" == "$asset" ]]; then
      found=true
      break
    fi
  done
  if [[ "$found" != true ]]; then
    echo "Unreferenced versioned frontend asset: $asset" >&2
    exit 1
  fi
done

printf 'Verified %d scripts and %d stylesheets from index.html\n' "${#scripts[@]}" "${#styles[@]}"
