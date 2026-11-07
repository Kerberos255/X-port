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
s = s.replace('__XPORT_VERSION__', 'browser-test')
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
  let fwNote = '主节点 · REALITY'
  globalThis.fetch = (input, init = {}) => {
    const u = new URL(typeof input === 'string' ? input : input.url, location.href)
    const path = u.pathname
    const method = String(init.method || 'GET').toUpperCase()
    if (path === '/api/overview') return json({
      system: {hostname:'github-runner', os:'Linux', distribution:'Ubuntu 24.04', kernelVersion:'6.x', systemType:'linux', hostAddress:'127.0.0.1', hostIPv6:'2001:db8:85a3::8a2e:370:7334', cpuCores:2, load5:0.48, load15:0.35, bootTime:0,
        uptimeSeconds:345678, cpuPercent:23.4, load1:0.72, memoryUsed:6442450944, memoryTotal:17179869184, diskUsed:68719476736, diskTotal:137438953472,
        networkRx:3145728, networkTx:1572864, xrayVersion:'25.9.11', xrayActive:true},
      accounts: {total:8, enabled:8}, xportVersion:'preview'
    })
    if (path === '/api/accounts' && method === 'GET') return json({accounts:[account]})
    if (path === '/api/accounts/1' && method === 'GET') return json(account)
    if (path === '/api/accounts/1/expert' && method === 'GET') return json(expert)
    if (path === '/api/accounts/online') return json({connections:{'1':2}})
    if (path === '/api/accounts/port-suggestion') return json({port:23741,min:20000,max:60000})
    if (path === '/api/firewall' && method === 'GET') return json({
      backend:'firewalld',active:true,writable:true,managers:['firewalld'],defaultZone:'public',zones:['public'],services:{public:['ssh']},
      notice:'预览：简单端口规则展示；底层链可能影响结果',rules:[
        {id:'firewalld:public:22/tcp',port:22,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'未识别',purposeSource:'unknown',note:'预留防火墙规则',protected:true,xray:false,usage:'unused',action:'allow'},
        {id:'firewalld:public:443/tcp',port:443,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'HTTPS · Nginx',purposeSource:'manual',note:'网站服务入口',protected:true,xray:false,usage:'used',action:'allow'},
        {id:'firewalld:public:2053/tcp',port:2053,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'Xray · VLESS',purposeSource:'xray',note:fwNote,protected:false,managed:true,xray:true,accountId:1,accountName:'主节点 · REALITY',accountEnabled:true,usage:'used',action:'allow'},
        {id:'firewalld:public:8443/tcp',port:8443,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:false,access:'区域：public',purpose:'Xray · Trojan',purposeSource:'xray',note:'待同步的运行时规则',protected:false,xray:true,usage:'unused',action:'allow'},
        {id:'firewalld:public:10401/tcp',port:10401,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'Xray · VLESS',purposeSource:'xray',note:'边缘入口',protected:false,managed:true,xray:true,usage:'used',action:'allow',accountId:2,accountName:'Edge · 东京节点',accountEnabled:true},
        {id:'firewalld:public:15432/tcp',port:15432,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'PostgreSQL',purposeSource:'manual',note:'限制应用服务访问',protected:false,xray:false,usage:'unused',action:'deny'},
        {id:'firewalld:public:rich:443/udp:deny:ipv4',port:443,protocol:'udp',family:'ipv4',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'未识别',protected:true,xray:false,usage:'unused',action:'deny',kind:'rich'},
        {id:'firewalld:public:rich:443/udp:deny:ipv6',port:443,protocol:'udp',family:'ipv6',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'未识别',protected:true,xray:false,usage:'unused',action:'deny',kind:'rich'},
        {id:'firewalld:public:51820/udp',port:51820,protocol:'udp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'WireGuard',purposeSource:'manual',note:'站点互联',protected:false,xray:false,usage:'used',action:'allow'},
        {id:'firewalld:public:39000-40000/tcp',port:0,rangeStart:39000,rangeEnd:40000,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'FTP 被动模式（配置匹配）',purposeSource:'ftp-config',note:'',protected:true,xray:false,usage:'unused',action:'allow',kind:'port'}
      ]})
    if (path === '/api/firewall/notes' && method === 'PUT') {
      fwNote = JSON.parse(init.body).note
      return json({note:fwNote})
    }
    if (path === '/api/firewall/purpose' && method === 'PUT') return json({purpose:JSON.parse(init.body).purpose})
    if (path === '/api/firewall/raw' && method === 'GET') return json({ruleset:'table inet firewalld {\\n chain INPUT { type filter hook input priority 10; }\\n}'})
    if (path === '/api/firewall/rules' && method === 'POST') return json({ok:true})
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
      if (preview.endsWith('-light')) applyTheme('light')
      if (preview === 'overview-tooltip') {
        const ring = await waitFor('#page-overview .status-gauge-ring')
        ring.dispatchEvent(new PointerEvent('pointerenter',{pointerType:'mouse'}))
        const popup = await waitFor('body > .status-gauge-floating')
        if (popup.hidden) throw new Error('expected open tooltip in screenshot')
        if (!document.querySelector('#server-details .is-ipv6')) throw new Error('expected IPv6 in screenshot')
      } else if (preview === 'overview-ipv6') {
        const address=await waitFor('#server-details .is-ipv6 b')
        if (!address.textContent.includes('2001:db8:')) throw new Error('IPv6 is missing from screenshot preview')
      } else if (preview === 'xray-update-light') {
        document.querySelector('#nav [data-page="xray"]').click()
        const note = await waitFor('#xray-page-update-note')
        if (!note.hidden) throw new Error('internal update note should be hidden')
        const status=await waitFor('#xray-page-update-state')
        if (!(status.textContent||'').trim()) throw new Error('Xray update state missing')
      } else if (preview === 'firewall' || preview === 'firewall-light' || preview === 'firewall-detail-light' || preview === 'firewall-editor-light' || preview === 'firewall-create-light' || preview === 'firewall-long-ipv6' || preview === 'firewall-long-ipv6-light' || preview === 'firewall-long-range') {
        document.querySelector('#nav [data-page="firewall"]').click()
        await waitFor('#fw-rows tr button[data-fw-act="edit"]')
        if (preview.startsWith('firewall-long-ipv6')) {
          const ip='2001:0db8:abcd:1234:5678:9abc:def0:1234/64'
          const label=document.querySelector('#fw-rows button[data-fw-id="firewalld:public:rich:443/udp:deny:ipv6"][data-fw-act="detail"]')?.closest('tr')?.querySelector('.fw-ip-cell span')
          if (!label) throw new Error('IPv6 firewall row unavailable in screenshot fixture')
          label.textContent=ip
          label.title=ip
          if (getComputedStyle(label).textOverflow!=='ellipsis' ||
              getComputedStyle(label).whiteSpace!=='nowrap' ||
              label.scrollWidth<=label.clientWidth)
            throw new Error('long IPv6 is not truncated to a single line')
          // Screenshot only: place the target row in the first visible
          // position; headless Chromium does not reliably repaint after scroll.
          const target=label.closest('tr')
          target.parentElement.prepend(target)
        }
        if (preview === 'firewall-long-range') {
          const row=document.querySelector('#fw-rows button[data-fw-id="firewalld:public:39000-40000/tcp"][data-fw-act="detail"]')?.closest('tr')
          const number=row?.querySelector('.fw-port-cell strong')
          const protocol=row?.querySelector('.fw-port-cell small')
          if (!number || !protocol || number.textContent!=='39000–40000' ||
              protocol.textContent.trim()!=='TCP' ||
              getComputedStyle(number).whiteSpace!=='nowrap' ||
              number.scrollWidth>number.clientWidth+1 ||
              number.getBoundingClientRect().bottom>protocol.getBoundingClientRect().top+1)
            throw new Error('long port range and protocol did not fit on separate lines')
          row.parentElement.prepend(row)
        }
        if (preview === 'firewall-detail-light') {
          document.querySelector('#fw-rows button[data-fw-act="detail"][data-fw-id="firewalld:public:2053/tcp"]').click()
          await waitFor('#fw-detail-dialog[open]')
        }
        if (preview === 'firewall-editor-light') {
          document.querySelector('#fw-rows button[data-fw-act="configure"][data-fw-id="firewalld:public:10401/tcp"]').click()
          await waitFor('#fw-policy-dialog[open]')
        }
        if (preview === 'firewall-create-light') {
          document.querySelector('#fw-add').click()
          await waitFor('#fw-add-dialog[open]')
          document.querySelector('#fw-add-port').value=18666
          document.querySelector('#fw-add-policy').value='deny'
        }
      } else if (preview === 'accounts' || preview === 'accounts-light') {
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
      } else if (preview === 'account-centered' || preview === 'account-centered-light' || preview === 'clone-centered') {
        document.querySelector('#nav [data-page="accounts"]').click()
        if(preview === 'clone-centered'){
          const clone=await waitFor('.account-row button[data-act="clone"]')
          clone.click()
          await waitFor('#clone-form')
        }else{
          const edit=await waitFor('.account-row button[data-act="edit"]')
          edit.click()
          await waitFor('#account-form')
        }
        const box=document.querySelector('#modal .modal-card')?.getBoundingClientRect()
        if(!box || Math.abs((box.top+box.bottom)/2-innerHeight/2)>Math.max(32,innerHeight*.09) ||
           Math.abs((box.left+box.right)/2-innerWidth/2)>Math.max(32,innerWidth*.09) ||
           box.left<0 || box.right>innerWidth)
          throw new Error('account dialog is not visually centered')
        if(innerWidth>700 && box.width<500)
          throw new Error('desktop account dialog is too narrow')
      } else if (preview === 'confirm-account' || preview === 'confirm-firewall' || preview === 'confirm-account-light') {
        const isFirewall=preview==='confirm-firewall'
        document.querySelector('#nav [data-page="'+(isFirewall?'firewall':'accounts')+'"]').click()
        if(isFirewall)await waitFor('#fw-rows tr')
        else await waitFor('.account-row')
        void XportOperation.ask({
          title:isFirewall?'确认删除放行规则':'确认删除账号',
          detail:isFirewall?'将删除 39000/TCP 的防火墙规则。':'删除测试账号？\\n对应 inbound 将从 Xray 配置中移除。',
          confirmText:'确认删除',danger:true
        })
        const dialog=await waitFor('#operation-confirm[open]')
        const rect=dialog.getBoundingClientRect()
        if(Math.abs((rect.top+rect.bottom)/2-innerHeight/2)>Math.max(28,innerHeight*.08) ||
           Math.abs((rect.left+rect.right)/2-innerWidth/2)>Math.max(28,innerWidth*.08))
          throw new Error('confirmation is not centered in viewport')
        if(innerWidth<700 && rect.width>innerWidth-18)throw new Error('mobile confirmation clipped')
      } else if (preview === 'operation-account' || preview === 'operation-firewall' || preview === 'operation-account-light') {
        const isFirewall=preview==='operation-firewall'
        document.querySelector('#nav [data-page="'+(isFirewall?'firewall':'accounts')+'"]').click()
        if(isFirewall)await waitFor('#fw-rows tr')
        else await waitFor('.account-row')
        void XportOperation.run({
          title:isFirewall?'正在添加防火墙规则':'正在创建账号',
          detail:isFirewall?'39000 / TCP · public':'测试账号 · 2053 / VLESS',
          perform:()=>new Promise(()=>{})
        })
        const dialog=await waitFor('#operation-progress[open]')
        const bounds=dialog.getBoundingClientRect()
        if(Math.abs((bounds.top+bounds.bottom)/2-innerHeight/2)>Math.max(28,innerHeight*.08)||
           Math.abs((bounds.left+bounds.right)/2-innerWidth/2)>Math.max(28,innerWidth*.08))
          throw new Error('progress modal not centered in viewport')
        if(innerWidth<700 && bounds.width>innerWidth-18)throw new Error('mobile progress modal clips viewport')
      } else if (preview === 'xray') {
        document.querySelector('#nav [data-page="xray"]').click()
        const panel = await waitFor('#xray-global-config-panel')
        panel.scrollIntoView({block:'center', inline:'nearest'})
      }
      await sleep(700)
      if (preview === 'firewall' && innerWidth > 700 && innerWidth <= 1050) {
        const selected=document.querySelector('#nav button.active')
        const label=document.querySelector('.sidebar .brand > div')
        if (selected && parseFloat(getComputedStyle(selected).fontSize) > 1)
          throw new Error('compact sidebar active label is visible and may wrap')
        if (label && getComputedStyle(label).display !== 'none')
          throw new Error('compact sidebar brand text not hidden')
        if (!selected?.title) throw new Error('compact nav is missing accessible hover label')
      }
      if (preview === 'firewall' && innerWidth >= 1300){
        const wrap=document.querySelector('#page-firewall .fw-table-wrap')
        const table=document.querySelector('#page-firewall .fw-table')
        if(!wrap||!table||table.scrollWidth>wrap.clientWidth+2)
          throw new Error('wide desktop must show all firewall columns without clipping')
        if(table.querySelectorAll('thead th').length!==9)
          throw new Error('IP family column not rendered in desktop table')
      }
      if (preview === 'firewall' && innerWidth > 700 && innerWidth < 1150){
        const hint=document.querySelector('#page-firewall .fw-scroll-hint')
        if(!hint||getComputedStyle(hint).display==='none')
          throw new Error('tablet needs an explicit scroll hint')
      }
      if ((preview === 'firewall' || preview.startsWith('firewall-long-ipv6') || preview === 'firewall-long-range') &&
          document.documentElement.scrollWidth > innerWidth + 2) {
        throw new Error('mobile horizontal overflow: '+document.documentElement.scrollWidth+' > '+innerWidth)
      }
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
    --window-size=1440,1000 --force-device-scale-factor=1 --virtual-time-budget=6500 \
    --screenshot="$OUT/$name.png" "http://127.0.0.1:$PORT/?preview=$query" >/dev/null 2>"$TMP/$name.log"
  [[ -s "$OUT/$name.png" ]] || { cat "$TMP/$name.log" >&2; echo "Screenshot missing: $name" >&2; exit 1; }
}
capture overview overview
capture overview-light overview-light
capture accounts-light accounts-light
capture xray-update-light xray-update-light
capture overview-ipv6 overview-ipv6
capture overview-tooltip overview-tooltip
capture accounts accounts
capture clone-account clone
capture account-advanced account
capture account-centered account-centered
capture account-centered-light account-centered-light
capture clone-centered clone-centered
capture operation-account operation-account
capture operation-account-light operation-account-light
capture operation-firewall operation-firewall
capture confirm-account confirm-account
capture confirm-account-light confirm-account-light
capture confirm-firewall confirm-firewall
for preview in account-centered account-centered-light clone-centered operation-account operation-account-light operation-firewall confirm-account confirm-account-light confirm-firewall; do
  "$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
    --window-size=1440,1000 --force-device-scale-factor=1 --virtual-time-budget=6500 \
    --dump-dom "http://127.0.0.1:$PORT/?preview=$preview" \
    >"$TMP/$preview-dom.html" 2>"$TMP/$preview-dom.log"
  grep -Fq "data-preview-ready=\"$preview\"" "$TMP/$preview-dom.html" || {
    grep -o 'data-preview-error="[^"]*"' "$TMP/$preview-dom.html" || true
    echo "Modal desktop preview failed: $preview" >&2
    exit 1
  }
done
capture xray-global xray
capture firewall-desktop firewall
capture firewall-light firewall-light
capture firewall-long-ipv6 firewall-long-ipv6
capture firewall-long-ipv6-light firewall-long-ipv6-light
capture firewall-long-range firewall-long-range
# Screenshots can be written even if injected preview assertions fail;
# require the Chromium DOM success sentinel for both long-address themes.
for preview in firewall-long-ipv6 firewall-long-ipv6-light firewall-long-range; do
  "$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
    --window-size=1440,1000 --force-device-scale-factor=1 --virtual-time-budget=6500 \
    --dump-dom "http://127.0.0.1:$PORT/?preview=$preview" \
    >"$TMP/$preview-dom.html" 2>"$TMP/$preview-dom.log"
  if ! grep -Fq "data-preview-ready=\"$preview\"" "$TMP/$preview-dom.html"; then
    grep -o 'data-preview-error="[^"]*"' "$TMP/$preview-dom.html" || true
    echo "Long IPv6 preview failed: $preview" >&2
    exit 1
  fi
done
capture firewall-detail-light firewall-detail-light
capture firewall-editor-light firewall-editor-light
capture firewall-create-light firewall-create-light
# The integrated firewall screen also needs narrow-viewport validation.
"$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
  --window-size=390,844 --force-device-scale-factor=1 --virtual-time-budget=6500 \
  --screenshot="$OUT/firewall-mobile.png" "http://127.0.0.1:$PORT/?preview=firewall" >/dev/null 2>"$TMP/firewall-mobile.log"
[[ -s "$OUT/firewall-mobile.png" ]] || { cat "$TMP/firewall-mobile.log" >&2; exit 1; }
"$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
  --window-size=390,844 --force-device-scale-factor=1 --virtual-time-budget=6500 \
  --screenshot="$OUT/firewall-long-ipv6-mobile.png" "http://127.0.0.1:$PORT/?preview=firewall-long-ipv6" \
  >/dev/null 2>"$TMP/firewall-long-ipv6-mobile.log"
[[ -s "$OUT/firewall-long-ipv6-mobile.png" ]] || {
  cat "$TMP/firewall-long-ipv6-mobile.log" >&2
  echo "Long IPv6 mobile screenshot missing" >&2
  exit 1
}
"$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
  --window-size=390,844 --force-device-scale-factor=1 --virtual-time-budget=6500 \
  --screenshot="$OUT/firewall-long-range-mobile.png" "http://127.0.0.1:$PORT/?preview=firewall-long-range" \
  >/dev/null 2>"$TMP/firewall-long-range-mobile.log"
[[ -s "$OUT/firewall-long-range-mobile.png" ]] || {
  cat "$TMP/firewall-long-range-mobile.log" >&2
  echo "Long port-range mobile screenshot missing" >&2
  exit 1
}
"$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
  --window-size=390,844 --force-device-scale-factor=1 --virtual-time-budget=6500 \
  --dump-dom "http://127.0.0.1:$PORT/?preview=firewall-long-ipv6" \
  >"$TMP/firewall-long-ipv6-mobile.html" 2>"$TMP/firewall-long-ipv6-mobile-dom.log"
if ! grep -Fq 'data-preview-ready="firewall-long-ipv6"' "$TMP/firewall-long-ipv6-mobile.html"; then
  grep -o 'data-preview-error="[^"]*"' "$TMP/firewall-long-ipv6-mobile.html" || true
  echo "Long IPv6 mobile layout test failed" >&2
  exit 1
fi
# Check real mobile dialogs, centered with no viewport clipping.
for preview in account-centered operation-account operation-firewall confirm-account confirm-firewall; do
  "$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
    --window-size=390,844 --force-device-scale-factor=1 --virtual-time-budget=6500 \
    --screenshot="$OUT/$preview-mobile.png" "http://127.0.0.1:$PORT/?preview=$preview" \
    >/dev/null 2>"$TMP/$preview-mobile.log"
  [[ -s "$OUT/$preview-mobile.png" ]] || {
    cat "$TMP/$preview-mobile.log" >&2
    exit 1
  }
  "$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
    --window-size=390,844 --force-device-scale-factor=1 --virtual-time-budget=6500 \
    --dump-dom "http://127.0.0.1:$PORT/?preview=$preview" \
    >"$TMP/$preview-mobile-dom.html" 2>"$TMP/$preview-mobile-dom.log"
  grep -Fq "data-preview-ready=\"$preview\"" "$TMP/$preview-mobile-dom.html" || {
    grep -o 'data-preview-error="[^"]*"' "$TMP/$preview-mobile-dom.html" || true
    echo "Modal mobile preview failed: $preview" >&2
    exit 1
  }
done
# A screenshot alone cannot detect horizontal overflow or failed JS.
"$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
  --window-size=390,844 --force-device-scale-factor=1 --virtual-time-budget=6500 \
  --dump-dom "http://127.0.0.1:$PORT/?preview=firewall" >"$TMP/firewall-mobile-dom.html" 2>"$TMP/firewall-mobile-dom.log"
grep -Fq 'data-preview-ready="firewall"' "$TMP/firewall-mobile-dom.html" || {
  grep -o 'data-preview-error="[^"]*"' "$TMP/firewall-mobile-dom.html" || true
  cat "$TMP/firewall-mobile-dom.log" >&2
  echo "Mobile preview did not pass layout checks" >&2
  exit 1
}
# Verify both compact and tablet layouts are scroll-safe. The full rule
# table may scroll within its container, but the document must not overflow.
for size in 360x780 768x1024; do
  width="${size%x*}"
  height="${size#*x}"
  "$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
    --window-size="$width,$height" --force-device-scale-factor=1 --virtual-time-budget=6500 \
    --screenshot="$OUT/firewall-$size.png" "http://127.0.0.1:$PORT/?preview=firewall" \
    >/dev/null 2>"$TMP/firewall-$size.log"
  [[ -s "$OUT/firewall-$size.png" ]] || { cat "$TMP/firewall-$size.log" >&2; exit 1; }
  "$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage \
    --window-size="$width,$height" --force-device-scale-factor=1 --virtual-time-budget=6500 \
    --dump-dom "http://127.0.0.1:$PORT/?preview=firewall" \
    >"$TMP/firewall-$size.html" 2>"$TMP/firewall-$size-dom.log"
  grep -Fq 'data-preview-ready="firewall"' "$TMP/firewall-$size.html" || {
    grep -o 'data-preview-error="[^"]*"' "$TMP/firewall-$size.html" || true
    echo "Viewport regression failed: $size" >&2
    exit 1
  }
done
printf 'GitHub runner UI screenshots:\n'
ls -lh "$OUT"/*.png
