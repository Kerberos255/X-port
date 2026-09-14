#!/usr/bin/env bash
set -Eeuo pipefail

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
DIST="$ROOT/web/dist"
TMP="$(mktemp -d -t xport-web-browser.XXXXXXXX)"
PORT="${XPORT_BROWSER_TEST_PORT:-18765}"
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
[[ -n "$BROWSER" ]] || { echo "Chrome/Chromium is required for browser regression checks" >&2; exit 1; }

cp -a "$DIST/." "$TMP/"
python3 - "$TMP/index.html" <<'PY'
from pathlib import Path
import sys
p = Path(sys.argv[1])
s = p.read_text(encoding='utf-8')
needle = '<script src="/app.js?v=0.1.16" defer></script>'
if needle not in s:
    raise SystemExit('app.js script marker not found')
s = s.replace(needle, '<script src="/__mock.js"></script>' + needle, 1)
s = s.replace('</body>', '<script src="/__driver.js" defer></script></body>', 1)
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
    monthlyReset: false, upBytes: 0, downBytes: 0, quotaBytes: 0, expiryTime: 0
  }
  globalThis.__legacyCloneHits = 0
  globalThis.fetch = (input, init = {}) => {
    const u = new URL(typeof input === 'string' ? input : input.url, location.href)
    const path = u.pathname
    const method = String(init.method || 'GET').toUpperCase()
    if (path === '/api/overview') return json({
      system: {hostname:'ci-node', os:'Linux', distribution:'Linux', kernelVersion:'test', systemType:'linux', hostAddress:'127.0.0.1', bootTime:0,
        uptimeSeconds:100, cpuPercent:1, load1:0.1, memoryUsed:1, memoryTotal:2, diskUsed:1, diskTotal:2,
        networkRx:0, networkTx:0, xrayVersion:'test', xrayActive:true},
      accounts: {total:1, enabled:1}, xportVersion:'browser-test'
    })
    if (path === '/api/accounts' && method === 'GET') return json({accounts:[account]})
    if (path === '/api/accounts/port-suggestion') return json({port:23741,min:20000,max:60000})
    if (path === '/api/xray/update' && method === 'GET') return json({current:'test',latest:'test',available:false})
    if (path === '/api/xray/geodata' && method === 'GET') return json({current:'test',latest:'test',available:false})
    if (path === '/api/settings') return json({panelListen:'127.0.0.1:8080',panelBasePath:'/',panelCertFile:'',panelKeyFile:'',panelDomain:'',panelTLS:false,xrayApiPort:10085,portMin:20000,portMax:60000,defaultRealitySni:'',defaultRealityDest:'',adminUsername:'admin'})
    if (path === '/api/backups') return json({backups:[]})
    if (path === '/api/logs') return json({logs:''})
    if (path === '/api/xray/config') return json({config:{routing:{},dns:{},outbounds:[],policy:{}}})
    return json({ok:true})
  }
})()
JS

cat > "$TMP/__driver.js" <<'JS'
(() => {
  const mark = (status, detail='') => {
    document.documentElement.dataset.browserRegression = status
    document.documentElement.dataset.browserRegressionDetail = detail
  }
  const sleep = ms => new Promise(r => setTimeout(r, ms))
  async function waitFor(selector, timeout=3000) {
    const end = Date.now() + timeout
    while (Date.now() < end) {
      const node = document.querySelector(selector)
      if (node) return node
      await sleep(25)
    }
    throw new Error(`timeout waiting for ${selector}`)
  }
  window.addEventListener('load', async () => {
    try {
      await waitFor('#app:not(.hidden)')
      const accountsNav = document.querySelector('#nav [data-page="accounts"]')
      accountsNav.click()
      const clone = await waitFor('.account-row button[data-act="clone"]')
      // If capture interception fails, the legacy row onclick will call this replacement.
      globalThis.openCloneModal = () => { globalThis.__legacyCloneHits++ }
      clone.click()
      const form = await waitFor('#clone-form')
      const port = form.querySelector('#clone-port')?.value || ''
      const text = form.textContent || ''
      if (port !== '23741') throw new Error(`clone port=${port}`)
      if (text.includes('协议与传输参数会复制')) throw new Error('legacy clone explanation is visible')
      if (Number(globalThis.__legacyCloneHits || 0) !== 0) throw new Error(`legacy clone handler hits=${globalThis.__legacyCloneHits}`)
      mark('pass', `port=${port};legacyHits=0`)
    } catch (err) {
      mark('fail', String(err && err.message || err))
    }
  }, {once:true})
})()
JS

(
  cd "$TMP"
  python3 -m http.server "$PORT" --bind 127.0.0.1 >/tmp/xport-browser-http.log 2>&1
) &
SERVER_PID=$!
for _ in $(seq 1 50); do
  if curl -fsS "http://127.0.0.1:$PORT/" >/dev/null; then break; fi
  sleep 0.1
done
curl -fsS "http://127.0.0.1:$PORT/" >/dev/null

DOM="$TMP/dom.html"
"$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage --virtual-time-budget=5000 --dump-dom "http://127.0.0.1:$PORT/" > "$DOM" 2>"$TMP/browser.log" || {
  cat "$TMP/browser.log" >&2
  exit 1
}
if ! grep -q 'data-browser-regression="pass"' "$DOM"; then
  echo "Browser regression failed" >&2
  grep -o 'data-browser-regression[^>]*' "$DOM" >&2 || true
  cat "$TMP/browser.log" >&2
  exit 1
fi
grep -o 'data-browser-regression="pass"[^>]*' "$DOM" | head -1
echo "Browser clone regression passed"
