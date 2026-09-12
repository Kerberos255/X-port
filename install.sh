#!/usr/bin/env bash
set -Eeuo pipefail

REPO="Kerberos255/X-port"
GITHUB="https://github.com/${REPO}"
RAW="https://raw.githubusercontent.com/${REPO}"

if [[ ${EUID:-$(id -u)} -ne 0 ]]; then
  echo "X-port installer must run as root. Try: curl -fsSL ${RAW}/main/install.sh | sudo bash" >&2
  exit 1
fi

for cmd in curl sha256sum awk mktemp uname bash; do
  command -v "$cmd" >/dev/null 2>&1 || { echo "Required command not found: $cmd" >&2; exit 1; }
done

case "$(uname -m)" in
  x86_64|amd64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
  *) echo "Unsupported architecture: $(uname -m). X-port publishes amd64 and arm64 binaries." >&2; exit 1 ;;
esac

if [[ -n "${XPORT_VERSION:-}" ]]; then
  TAG="${XPORT_VERSION}"
  [[ "$TAG" == v* ]] || TAG="v${TAG}"
else
  LATEST_URL="$(curl --proto '=https' --tlsv1.2 -fsSI --retry 3 \
    -o /dev/null -w '%{redirect_url}' "${GITHUB}/releases/latest")"
  TAG="${LATEST_URL##*/}"
fi

if [[ ! "$TAG" =~ ^v[0-9A-Za-z][0-9A-Za-z._-]*$ ]]; then
  echo "Could not determine a safe X-port release tag (got: ${TAG:-empty})." >&2
  exit 1
fi

ASSET="xport-linux-${ARCH}"
TMP="$(mktemp -d -t xport-install.XXXXXXXX)"
trap 'rm -rf "$TMP"' EXIT
mkdir -p "$TMP/scripts"

echo "Installing X-port ${TAG} for linux/${ARCH}..."

curl --proto '=https' --tlsv1.2 -fL --retry 3 \
  -o "$TMP/$ASSET" "${GITHUB}/releases/download/${TAG}/${ASSET}"
curl --proto '=https' --tlsv1.2 -fL --retry 3 \
  -o "$TMP/SHA256SUMS" "${GITHUB}/releases/download/${TAG}/SHA256SUMS"

EXPECTED="$(awk -v asset="$ASSET" '
  {
    file=$2
    sub(/^\*/, "", file)
    sub(/^.*\//, "", file)
    if (file == asset) { print $1; exit }
  }
' "$TMP/SHA256SUMS")"

if [[ ! "$EXPECTED" =~ ^[0-9a-fA-F]{64}$ ]]; then
  echo "No valid SHA-256 entry for ${ASSET} in ${TAG}/SHA256SUMS." >&2
  exit 1
fi

ACTUAL="$(sha256sum "$TMP/$ASSET" | awk '{print $1}')"
if [[ "${ACTUAL,,}" != "${EXPECTED,,}" ]]; then
  echo "SHA-256 verification failed for ${ASSET}." >&2
  echo "Expected: $EXPECTED" >&2
  echo "Actual:   $ACTUAL" >&2
  exit 1
fi

echo "Verified ${ASSET}: ${ACTUAL}"

for helper in install.sh migrate-xpanel.sh; do
  curl --proto '=https' --tlsv1.2 -fL --retry 3 \
    -o "$TMP/scripts/$helper" "${RAW}/${TAG}/scripts/${helper}"
done

bash -n "$TMP/scripts/install.sh" "$TMP/scripts/migrate-xpanel.sh"
chmod 0755 "$TMP/$ASSET" "$TMP/scripts/install.sh" "$TMP/scripts/migrate-xpanel.sh"

export XPORT_BINARY="$TMP/$ASSET"
bash "$TMP/scripts/install.sh"

echo "X-port ${TAG} installation completed."
