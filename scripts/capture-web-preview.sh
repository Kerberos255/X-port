#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="$ROOT/web/dist"
OUT="${1:-${XPORT_BROWSER_SCREENSHOT_DIR:-$ROOT/artifacts/ui-preview}}"
TMP="$(mktemp -d -t xport-web-preview.XXXXXXXX)"
PORT="${XPORT_PREVIEW_PORT:-18766}"
SERVER_PID=""
cleanup() {
  if [[ -n "$SERVER_PID" ]]; then kill "$SERVER_PID" 2>/dev/null || true; fi
  rm -rf "$TMP"
}
trap cleanup EXIT

BROWSER=""
for candidate in google-chrome-stable google-chrome chromium chromium-browser; do
  if command -v "$candidate" >/dev/null 2>&1; then BROWSER="$(command -v "$candidate")"; break; fi
done
[[ -n "$BROWSER" ]] || { echo "Chrome/Chromium is required for UI preview screenshots" >&2; exit 1; }

mkdir -p "$OUT"
cp -a "$DIST/." "$TMP/"
python3 - "$TMP/index.html" <<'PY'
from pathlib import Path
import re
import sys
p = Path(sys.argv[1])
s = p.read_text(encoding='utf-8')
pattern = r'(<script src="/app\.js(?:\?[^\"]*)?" defer></script>)'
if not re.search(pattern, s):
    raise SystemExit('app.js script marker not found')
s = re.sub(pattern, r'<script src="/__mock.js"></script>\1', s, count=1)
s = s.replace('</body>', '<script src="/__preview.js" defer></script></body>', 1)
p.write_text(s, encoding='utf-8')
PY

cat > "$TMP/__mock.js" <<'JS'
(() => {
  const json = (body, status = 200) => Promise.resolve(new Response(JSON.stringify(body), {
    status,
    headers: {'Content-Type': 'application/json'}
  }))
  const account = {
    id: 1, name: 'alpha', port: 23456, protocol: 'vless', network: 'tcp', security: 'tls',
    credential: '11111111-1111-4111-8111-111111111111', enabled: true, editable: true,
    monthlyReset: false, upBytes: 1288490188, downBytes: 3221225472, quotaBytes: 10737418240, expiryTime: 0
  }
  const expert = {
    protocol:'vless', method:'raw', security:'tls', acceptProxyProtocol:false,
    fallbacksJson:'[{"name":"web","alpn":"h2","path":"/legacy","dest":80,"xver":0}]', httpHeaderJson:'',
    supportsFallbacks:true, supportsHttpHeader:true, supportsReality:false, supportsTls:true, supportsXhttp:false,
    tlsAlpn:['h2','http/1.1'], tlsMinVersion:'1.2', tlsMaxVersion:'', tlsCipherSuites:'',
    tlsRejectUnknownSni:false, tlsCertificatesJson:'[]'
  }
  globalThis.fetch = (input, init = {}) => {
    const u = new URL(typeof input === 'string' ? input : input.url, location.href)
    const path = u.pathname
    const method = String(init.method || 'GET').toUpperCase()
    if (path === '/api/overview') return json({
      system: {hostname:'github-runner', os:'Linux', distribution:'Ubuntu 24.04', kernelVersion:'6.x', systemType:'linux', hostAddress:'127.0.0.1', bootTime:0,
        uptimeSeconds:345678, cpuPercent:23.4, load1:0.72, memoryUsed:6442450944, memoryTotal:17179869184, diskUsed:68719476736, diskTotal:137438953472,
        networkRx:3145728, networkTx:1572864, xrayVersion:'25.9.11', xrayActive:true},
      accounts: {total:8, enabled:8}, xportVersion:'preview'
    })
    if (path === '/api/accounts' && method === 'GET') return json({accounts:[account]})
    if (path === '/api/accounts/1' && method === 'GET') return json(account)
    if (path === '/api/accounts/1/expert' && method === 'GET') return json(expert)
    if (path === '/api/accounts/online') return json({connections:{'1':2}})
    if (path === '/api/accounts/port-suggestion') return json({port:23741,min:20000,max:60000})
    if (path === '/api/xray/update' && method === 'GET') return json({current:'25.9.11',latest:'25.9.11',available:false})
    if (path === '/api/xray/geodata' && method === 'GET') return json({current:'2026-09-14',latest:'2026-09-14',available:false})
    if (path === '/api/settings') return json({panelListen:'127.0.0.1:8080',panelBasePath:'/',panelCertFile:'',panelKeyFile:'',panelDomain:'panel.example.com',panelTLS:false,xrayApiPort:10085,portMin:20000,portMax:60000,defaultRealitySni:'www.microsoft.com',defaultRealityDest:'www.microsoft.com:443',adminUsername:'admin'})
    if (path === '/api/backups') return json({backups:[]})
    if (path === '/api/logs') return json({logs:'Sep 14 17:20:00 github-runner xport[1000]: preview service healthy'})
    if (path === '/api/xray/config') return json({config:{routing:{rules:[]},dns:{servers:['1.1.1.1']},outbounds:[{protocol:'freedom',tag:'direct'}],policy:{}}})
    return json({ok:true})
  }
})()
JS

cat > "$TMP/__preview.js" <<'JS'
(() => {
  const sleep = ms => new Promise(r => setTimeout(r, ms))
  async function waitFor(selector, timeout=4000) {
    const end = Date.now() + timeout
    while (Date.now() < end) {
      const node = document.querySelector(selector)
      if (node) return node
      await sleep(25)
    }
    throw new Error(`timeout waiting for ${selector}`)
  }
  async function waitForValue(selector, expected, timeout=4000) {
    const node = await waitFor(selector, timeout)
    const end = Date.now() + timeout
    while (Date.now() < end) {
      if (String(node.value) === String(expected)) return node
      await sleep(25)
    }
    throw new Error(`timeout waiting for ${selector}=${expected}`)
  }
  window.addEventListener('load', async () => {
    try {
      await waitFor('#app:not(.hidden)')
      const preview = new URLSearchParams(location.search).get('preview') || 'overview'
      if (preview === 'accounts') {
        document.querySelector('#nav [data-page="accounts"]').click()
        await waitFor('.account-row')
      } else if (preview === 'clone') {
        document.querySelector('#nav [data-page="accounts"]').click()
        const clone = await waitFor('.account-row button[data-act="clone"]')
        clone.click()
        await waitFor('#clone-form')
        await waitForValue('#clone-port', 23741)
      } else if (preview === 'account') {
        document.querySelector('#nav [data-page="accounts"]').click()
        const edit = await waitFor('.account-row button[data-act="edit"]')
        edit.click()
        await waitFor('#account-form')
        const advanced = await waitFor('#account-advanced')
        if (!advanced.open) advanced.querySelector('summary').click()
        const fallback = await waitFor('.fallback-section')
        fallback.scrollIntoView({block:'center', inline:'nearest'})
      } else if (preview === 'xray') {
        document.querySelector('#nav [data-page="xray"]').click()
        const panel = await waitFor('#xray-global-config-panel')
        panel.scrollIntoView({block:'center', inline:'nearest'})
      }
      await sleep(700)
      document.documentElement.dataset.previewReady = preview
    } catch (err) {
      document.documentElement.dataset.previewError = String(err && err.message || err)
    }
  }, {once:true})
})()
JS

(
  cd "$TMP"
  python3 -m http.server "$PORT" --bind 127.0.0.1 >"$TMP/http.log" 2>&1
) &
SERVER_PID=$!
for _ in $(seq 1 50); do
  if curl -fsS "http://127.0.0.1:$PORT/" >/dev/null; then break; fi
  sleep 0.1
done
curl -fsS "http://127.0.0.1:$PORT/" >/dev/null

capture() {
  local name="$1" query="$2"
  "$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
    --window-size=1440,1000 --force-device-scale-factor=1 --virtual-time-budget=6000 \
    --screenshot="$OUT/$name.png" "http://127.0.0.1:$PORT/?preview=$query" >/dev/null 2>"$TMP/$name.log"
  [[ -s "$OUT/$name.png" ]] || { cat "$TMP/$name.log" >&2; echo "Screenshot missing: $name" >&2; exit 1; }
}

capture overview overview
capture accounts accounts
capture clone-account clone
capture account-advanced account
capture xray-global xray
printf 'GitHub runner UI screenshots:\n'
ls -lh "$OUT"/*.png
