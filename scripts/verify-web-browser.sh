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
import re
import sys
p = Path(sys.argv[1])
s = p.read_text(encoding='utf-8')
pattern = r'(<script src="/app\.js(?:\?[^\"]*)?" defer></script>)'
if not re.search(pattern, s):
    raise SystemExit('app.js script marker not found')
s = re.sub(pattern, r'<script src="/__mock.js"></script>\1', s, count=1)
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
  const expert = {
    protocol:'vless', method:'raw', security:'tls', acceptProxyProtocol:false,
    fallbacksJson:'[{"path":"/legacy","dest":80,"xver":0}]', httpHeaderJson:'',
    supportsFallbacks:true, supportsHttpHeader:true, supportsReality:false, supportsTls:true, supportsXhttp:false,
    tlsAlpn:['http/1.1'], tlsMinVersion:'', tlsMaxVersion:'', tlsCipherSuites:'',
    tlsRejectUnknownSni:false, tlsCertificatesJson:'[]'
  }
  globalThis.__legacyCloneHits = 0
  globalThis.__fwConfirmCalls = 0
  globalThis.__fwConfirmResult = false
  globalThis.confirm = () => { globalThis.__fwConfirmCalls++; return globalThis.__fwConfirmResult }
  let fwNote = '主节点 · REALITY'
  globalThis.fetch = (input, init = {}) => {
    const u = new URL(typeof input === 'string' ? input : input.url, location.href)
    const path = u.pathname
    const method = String(init.method || 'GET').toUpperCase()
    if (path === '/api/overview') return json({
      system: {hostname:'ci-node', os:'Linux', distribution:'Linux', kernelVersion:'test', systemType:'linux', hostAddress:'127.0.0.1',hostIPv6:'2001:db8::1234',cpuCores:4,load5:0.2,load15:0.3, bootTime:0,
        uptimeSeconds:100, cpuPercent:1, load1:0.1, memoryUsed:1, memoryTotal:2, diskUsed:1, diskTotal:2,
        networkRx:0, networkTx:0, xrayVersion:'test', xrayActive:true},
      accounts: {total:1, enabled:1}, xportVersion:'browser-test'
    })
    if (path === '/api/accounts' && method === 'GET') return json({accounts:[account]})
    if (path === '/api/accounts/1' && method === 'GET') return json(account)
    if (path === '/api/accounts/1/expert' && method === 'GET') return json(expert)
    if (path === '/api/accounts/online') return json({connections:{}})
    if (path === '/api/accounts/port-suggestion') return json({port:23741,min:20000,max:60000})
    if (path === '/api/firewall' && method === 'GET') return json({
      backend:'firewalld',active:true,writable:!globalThis.__fwReadOnly,unsafeReason:globalThis.__fwReadOnly?'独立 INPUT 过滤链需要检查':'',managers:['firewalld'],defaultZone:'public',zones:['public'],services:{public:['ssh']},
      notice:'预览：简单端口规则展示；底层链可能影响结果',rules:[
        {id:'firewalld:public:22/tcp',port:22,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'SSH（常见端口，未核实）',purposeSource:'assumed',note:'远程管理',protected:true,xray:false},
        {id:'firewalld:public:443/tcp',port:443,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'HTTPS',purposeSource:'manual',note:'网站入口',protected:false,xray:false,kind:'port',action:'allow',managed:false,revision:'111122223333444455556666'},
        {id:'firewalld:public:2053/tcp',port:2053,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'Xray · VLESS',purposeSource:'xray',note:fwNote,protected:false,managed:true,xray:true,usage:'used',usageDetails:[{port:2053,protocol:'tcp',pid:1242,name:'xray'}],action:'allow',kind:'port',canToggle:true,accountId:1,accountName:'Alpha inbound',accountEnabled:true},
        {id:'firewalld:public:8443/tcp',port:8443,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:false,access:'区域：public',purpose:'Xray · Trojan',purposeSource:'xray',note:'临时放行，需同步',protected:false,xray:true},
        {id:'firewalld:public:51820/udp',port:51820,protocol:'udp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'未识别',purposeSource:'unknown',note:'WireGuard 站点互联',protected:false,xray:false,usage:'unused',action:'deny'},
        {id:'firewalld:public:22022/tcp',port:22022,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'SSH · 管理端口',purposeSource:'ssh',note:'',protected:true,xray:false,usage:'used',usageDetails:[{port:22022,protocol:'tcp',pid:29635,name:'sshd'}],action:'allow',kind:'port',canToggle:false},
        {id:'firewalld:public:39000-40000/tcp',port:0,rangeStart:39000,rangeEnd:40000,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'FTP 被动模式 · Pure-FTPd（配置匹配）',purposeSource:'ftp-config',note:'',protected:true,xray:false,usage:'unused',action:'allow',kind:'port',canToggle:false},
        {id:'firewalld:public:8444/tcp',port:8444,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'未识别',purposeSource:'unknown',note:'',protected:true,xray:false,usage:'used',usageDetails:[{port:8444,protocol:'tcp',name:'web-svc',container:true}],action:'allow',kind:'port',canToggle:false}
      ]})
    if (path === '/api/firewall/notes' && method === 'PUT') {
      fwNote = JSON.parse(init.body).note
      return json({note:fwNote})
    }
    if (path === '/api/firewall/purpose' && method === 'PUT') return json({purpose:JSON.parse(init.body).purpose})
    if (path === '/api/firewall/raw' && method === 'GET') return json({ruleset:'table inet firewalld {\\n chain INPUT { type filter hook input priority 10; }\\n}'})
    if (path === '/api/firewall/rules' && method === 'POST') { globalThis.__fwMutationCalls=(globalThis.__fwMutationCalls||0)+1; return json({ok:true}) }
    if (path === '/api/firewall/control' && method === 'POST') {
      globalThis.__fwControlCalls=(globalThis.__fwControlCalls||0)+1
      globalThis.__fwLastControl=JSON.parse(init.body||'{}')
      return json({ok:true,pending:false})
    }
    if (path === '/api/firewall/edit' && method === 'POST') {
      globalThis.__fwEditCalls=(globalThis.__fwEditCalls||0)+1
      globalThis.__fwLastEdit=JSON.parse(init.body||'{}')
      return json({ok:true,pending:false})
    }
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
  async function waitForValue(selector, want, timeout=3000) {
    const end = Date.now() + timeout
    while (Date.now() < end) {
      const node = document.querySelector(selector)
      if (node && String(node.value || '') === want) return node
      await sleep(25)
    }
    const got = document.querySelector(selector)?.value || ''
    throw new Error(`timeout waiting for ${selector}=${want}; got=${got}`)
  }
  async function waitForValueContains(selector, part, timeout=3000) {
    const end = Date.now() + timeout
    while (Date.now() < end) {
      const node = document.querySelector(selector)
      if (node && String(node.value || '').includes(part)) return node
      await sleep(25)
    }
    throw new Error(`timeout waiting for ${selector} to contain ${part}`)
  }
  async function waitForTextContains(selector,part,timeout=3500) {
    const end=Date.now()+timeout
    while(Date.now()<end) {
      const node=document.querySelector(selector)
      if(node && String(node.textContent||'').includes(part)) return node
      await sleep(25)
    }
    throw new Error('timeout waiting for text '+selector+' to contain '+part)
  }
  function assertContained(container, node, label) {
    const outer = container.getBoundingClientRect()
    const inner = node.getBoundingClientRect()
    if (inner.left < outer.left - 2 || inner.right > outer.right + 2) {
      throw new Error(`${label} overflows horizontally: ${Math.round(inner.left)}..${Math.round(inner.right)} outside ${Math.round(outer.left)}..${Math.round(outer.right)}`)
    }
  }
  window.addEventListener('load', async () => {
    try {
      await waitFor('#app:not(.hidden)')
      const gauges = document.querySelectorAll('#page-overview .status-gauge-ring')
      if (gauges.length !== 4) throw new Error('expected four overview status gauges, got '+gauges.length)
      if (!document.querySelector('#status-load-value')?.textContent.includes('%')) throw new Error('load gauge did not update')
      const healthLabel = document.querySelector('#page-overview .health-row')
      const healthStatus = document.querySelector('#page-overview .health-row b')
      const serviceSub = document.querySelector('#page-overview .service-row small')
      if (!healthLabel || !healthStatus || !serviceSub) throw new Error('overview readability elements missing')
      for (const [node,minSize] of [[healthLabel,11],[healthStatus,11],[serviceSub,10.5]]) {
        if (parseFloat(getComputedStyle(node).fontSize) < minSize) throw new Error('overview text too small: '+node.className)
      }
      if (!document.querySelector('#server-details .is-ipv6 b')?.textContent.includes('2001:db8::1234')) throw new Error('IPv6 row missing')
      const popup = document.querySelector('body > .status-gauge-floating')
      if (!popup || !popup.hidden) throw new Error('body-level tooltip should start hidden')
      const label = document.querySelector('#page-overview .status-gauge-title')
      label.dispatchEvent(new PointerEvent('pointerenter',{pointerType:'mouse'}))
      if (!popup.hidden) throw new Error('label must not trigger popup')
      gauges[0].dispatchEvent(new PointerEvent('pointerenter',{pointerType:'mouse'}))
      if (popup.hidden || popup.querySelectorAll('.status-gauge-row').length !== 4) throw new Error('ring must trigger compact tooltip')
      if (getComputedStyle(popup).position !== 'fixed') throw new Error('popup is not viewport-level')
      if (Number(getComputedStyle(popup).zIndex) < 60) throw new Error('tooltip must stack above cards')
      const detailNumber = popup.querySelector('.status-gauge-row b')
      const detailStyle = getComputedStyle(detailNumber)
      if (detailNumber.getBoundingClientRect().height > 35 || detailStyle.borderRadius === '50%') throw new Error('legacy ring CSS leaked into popup rows')
      const rect = popup.getBoundingClientRect()
      if (rect.left < 0 || rect.right > innerWidth+1 || rect.top < 0 || rect.bottom > innerHeight+1) throw new Error('tooltip escapes viewport')
      gauges[0].dispatchEvent(new PointerEvent('pointerleave',{pointerType:'mouse'}))
      if (!popup.hidden) throw new Error('hover popup must hide after leaving ring')
      gauges[0].click()
      if (gauges[0].getAttribute('aria-expanded') !== 'true') throw new Error('ring click did not pin details')
      await loadOverview(false,true)
      if (gauges[0].getAttribute('aria-expanded') !== 'true' || popup.hidden) throw new Error('refresh unpinned details')
      document.dispatchEvent(new KeyboardEvent('keydown',{key:'Escape'}))
      if (!popup.hidden) throw new Error('Escape did not close popup')
      gauges[0].click()
      document.querySelector('#page-overview .status-gauge-title').dispatchEvent(new PointerEvent('pointerdown',{bubbles:true}))
      if (!popup.hidden) throw new Error('outside click did not close popup')
      state.overview.system.hostIPv6 = ''
      renderServerDetailsV10()
      if (document.querySelector('#server-details .is-ipv6')) throw new Error('IPv6 row should hide when absent')
      state.overview.system.hostIPv6 = '2001:db8::1234'
      state.overview.system.hostAddress = '2001:db8::1234'
      renderServerDetailsV10()
      if (document.querySelector('#server-details .is-ipv6')) throw new Error('IPv6-only server must not show duplicate address')
      document.querySelector('#nav [data-page="accounts"]').click()

      const accountSubtitle = await waitFor('.account-row .account-name small')
      if (parseFloat(getComputedStyle(accountSubtitle).fontSize) < 11) throw new Error('account subtitle too small')
      // Status belongs in the badge, not in an internal implementation note.
      renderXrayUpdateState(null)
      const xrayNote = document.querySelector('#xray-page-update-note')
      if (!xrayNote || !xrayNote.hidden) throw new Error('Xray checking message must not expose internal note')
      renderXrayUpdateState({current:'25.9.11',latest:'25.9.11',available:false})
      if (!xrayNote.hidden || xrayNote.textContent) throw new Error('Xray success note should be hidden')
      renderXrayUpdateState(null,'Test update failure')
      if (xrayNote.hidden || !xrayNote.textContent.includes('Test update failure')) throw new Error('Xray error message should remain visible')
      renderXrayUpdateState({current:'25.9.11',latest:'25.9.11',available:false})
      const clone = await waitFor('.account-row button[data-act="clone"]')
      // If capture interception fails, the legacy row onclick will call this replacement.
      globalThis.openCloneModal = () => { globalThis.__legacyCloneHits++ }
      clone.click()
      const cloneForm = await waitFor('#clone-form')
      const portInput = await waitForValue('#clone-port', '23741')
      if ((cloneForm.textContent || '').includes('协议与传输参数会复制')) throw new Error('legacy clone explanation is visible')
      if (Number(globalThis.__legacyCloneHits || 0) !== 0) throw new Error(`legacy clone handler hits=${globalThis.__legacyCloneHits}`)
      document.querySelector('#clone-cancel').click()

      const edit = await waitFor('.account-row button[data-act="edit"]')
      edit.click()
      await waitFor('#account-form')
      const advanced = await waitFor('#account-advanced')
      advanced.querySelector('summary').click()
      const fallback = await waitFor('.fallback-section')
      const dest = fallback.querySelector('[data-fallback-key="dest"]')?.value || ''
      if (dest !== '80') throw new Error(`fallback dest=${dest}`)
      const sections = [...document.querySelectorAll('.expert-section')]
      const tls = sections.find(s => String(s.querySelector('.expert-section-title')?.textContent || '').trim().startsWith('TLS'))
      if (!tls || tls.nextElementSibling !== fallback) throw new Error('Fallbacks is not directly after TLS')
      const editorScroll = await waitFor('.account-editor-scroll')
      assertContained(editorScroll, fallback, 'Fallbacks section')
      const visibleControls = [...fallback.querySelectorAll('input,select,textarea,button')].filter(node => node.getClientRects().length > 0)
      for (const [i, node] of visibleControls.entries()) {
        assertContained(editorScroll, node, `Fallbacks control ${i + 1}`)
      }
      if (editorScroll.scrollWidth > editorScroll.clientWidth + 2) {
        throw new Error(`account editor scroll width overflow: ${editorScroll.scrollWidth}>${editorScroll.clientWidth}`)
      }
      document.querySelector('#modal-close').click()

      document.querySelector('#nav [data-page="xray"]').click()
      await waitFor('#xray-global-config-panel')
      const globalEditor = await waitForValueContains('#xray-global-config', '"routing"')
      const globalConfig = JSON.parse(globalEditor.value)
      if (!globalConfig.routing || !Array.isArray(globalConfig.outbounds)) throw new Error('global Xray config editor did not load expected sections')

      document.querySelector('#nav [data-page="firewall"]').click()
      const fwNoteButton=await waitFor('#fw-rows button[data-fw-id="firewalld:public:2053/tcp"][data-fw-key="note"]')
      fwNoteButton.click()
      const fwInput=await waitFor('.fw-edit-input')
      fwInput.value='浏览器回归备注'
      const fwSave=await waitFor('#fw-rows button[data-fw-act="save"]')
      fwSave.click()
      await waitForTextContains('#fw-rows button[data-fw-id="firewalld:public:2053/tcp"][data-fw-key="note"] span','浏览器回归备注')
      if(document.querySelector('#nav [data-page="firewall"]')?.classList.contains('active')!==true) throw new Error('firewall navigation not active')
      const accountLink=document.querySelector('#fw-rows button.fw-account-link[data-fw-account="1"]')
      if(!accountLink||!accountLink.textContent.includes('Alpha inbound')) throw new Error('Xray account linkage is not visible')
      const accountRow=accountLink.closest('tr')
      if(!accountRow.querySelector('.fw-usage.used')||!accountRow.querySelector('.fw-policy.allow')) throw new Error('account usage or allow badge missing')
      const sshRow=document.querySelector('#fw-rows button[data-fw-act="detail"][data-fw-id="firewalld:public:22022/tcp"]')?.closest('tr')
      if(!sshRow?.textContent.includes('sshd')||!sshRow?.querySelector('button.fw-switch[disabled]')) throw new Error('nonstandard SSH process or protected state missing')
      const ftpRow=document.querySelector('#fw-rows button[data-fw-act="detail"][data-fw-id="firewalld:public:39000-40000/tcp"]')?.closest('tr')
      if(!ftpRow?.textContent.includes('39000–40000')||!ftpRow?.textContent.includes('FTP 被动模式')||!ftpRow?.querySelector('button.fw-switch[disabled]')) throw new Error('read-only passive FTP range missing')
      ftpRow.querySelector('button[data-fw-act="configure"]').click()
      const rangeEditor=await waitFor('#fw-policy-dialog[open]')
      if(!rangeEditor.querySelector('#fw-policy-port').disabled||!rangeEditor.querySelector('#fw-policy-save').disabled||
         !rangeEditor.querySelector('#fw-policy-title').textContent.includes('39000–40000')) throw new Error('FTP range editor must be read-only')
      rangeEditor.querySelector('button[value="cancel"]').click()
      const dockerRow=document.querySelector('#fw-rows button[data-fw-act="detail"][data-fw-id="firewalld:public:8444/tcp"]')?.closest('tr')
      if(!dockerRow?.textContent.includes('Docker: web-svc')) throw new Error('Docker published port association missing')
      const denyRow=document.querySelector('#fw-rows button[data-fw-act="detail"][data-fw-id="firewalld:public:51820/udp"]')?.closest('tr')
      if(!denyRow?.querySelector('.fw-policy.deny')) throw new Error('deny strategy is not visible')
      const toggle=accountRow.querySelector('button.fw-switch')
      if(!toggle||toggle.getAttribute('aria-checked')!=='true') throw new Error('rule switch is not labelled')
      const beforeCalls=globalThis.__fwMutationCalls||0
      toggle.click()
      if((globalThis.__fwMutationCalls||0)!==beforeCalls) throw new Error('preview toggle unexpectedly mutated firewall')
      accountRow.querySelector('button[data-fw-act="configure"]').click()
      const policyDialog=await waitFor('#fw-policy-dialog[open]')
      if(policyDialog.querySelector('#fw-policy-port').value!=='2053'||!policyDialog.querySelector('#fw-policy-save').disabled)
        throw new Error('policy editor preview must not allow real writes')
      policyDialog.querySelector('button[value="cancel"]').click()
            const detailButton=document.querySelector('#fw-rows button[data-fw-act="detail"][data-fw-id="firewalld:public:2053/tcp"]')
      if(!detailButton) throw new Error('firewall detail action missing')
      detailButton.click()
      const detail=await waitFor('#fw-detail-dialog[open]')
      if(detail.querySelectorAll('.fw-detail-section').length!==4) throw new Error('firewall detail groups missing')
      if(detail.getBoundingClientRect().width < 440 && innerWidth > 700) throw new Error('firewall detail dialog is too narrow')
      if(!detail.querySelector('.fw-detail-summary')?.textContent.includes('X-port')) throw new Error('detail summary missing')
      if(!detail.textContent.includes('xray')||!detail.textContent.includes('1242'))throw new Error('process/PID details missing')
      detail.querySelector('button[value="cancel"]').click()

      const oldTheme=document.documentElement.dataset.theme
      document.documentElement.dataset.theme='light'
      const chip=document.querySelector('#fw-rows .fw-source')
      const noteButton=document.querySelector('#fw-rows .fw-remark')
      for(const node of [chip,noteButton]){
        if(!node) throw new Error('firewall metadata element missing')
        const computed=getComputedStyle(node)
        if(parseFloat(computed.fontSize)<12) throw new Error('firewall metadata too small')
        const values=computed.color.match(/[0-9]+/g)?.slice(0,3).map(Number)||[]
        if(values.length!==3 || values.reduce((a,b)=>a+b,0)/3>135) throw new Error('light-theme metadata too pale: '+computed.color)
      }
      if(parseFloat(getComputedStyle(document.querySelector('#nav button')).fontSize)<13 && innerWidth>1050) throw new Error('sidebar text too small')
      if(parseFloat(getComputedStyle(document.querySelector('#login-form label')).fontSize)<12) throw new Error('login label too small')
      document.documentElement.dataset.theme=oldTheme||'dark'

      globalThis.__fwReadOnly=true
      document.querySelector('#fw-refresh').click()
      const diagnostic=await waitFor('#fw-diagnostic:not([hidden])')
      if(!diagnostic.textContent.includes('独立 INPUT')) throw new Error('read-only reason not visible near backend')
      if(!document.querySelector('#fw-add').disabled) throw new Error('read-only guard failed')
      globalThis.__fwReadOnly=false
      document.querySelector('#fw-refresh').click()
      await waitFor('#fw-diagnostic[hidden]')
      if(document.querySelector('#fw-add').disabled) throw new Error('write access did not recover')
      const beforeDeny=globalThis.__fwMutationCalls||0
      const beforeControl=globalThis.__fwControlCalls||0
      const beforeConfirm=globalThis.__fwConfirmCalls||0
      document.querySelector('#fw-add').click()
      const create=await waitFor('#fw-add-dialog[open]')
      create.querySelector('#fw-add-port').value='18666'
      create.querySelector('#fw-add-policy').value='deny'
      create.querySelector('#fw-confirm-add').click()
      await sleep(160)
      if(globalThis.__fwConfirmCalls<=beforeConfirm)
        throw new Error('deny option did not require confirmation: closed='+!create.open+' value='+create.returnValue+' policy='+create.querySelector('#fw-add-policy').value+' writable='+!document.querySelector('#fw-add').disabled)
      if((globalThis.__fwControlCalls||0)!==beforeControl) throw new Error('declined deny confirmation sent an API write')
      if((globalThis.__fwMutationCalls||0)!==beforeDeny) throw new Error('deny option unexpectedly sent an allow mutation')
      globalThis.__fwConfirmResult=true
      document.querySelector('#fw-add').click()
      const confirmed=await waitFor('#fw-add-dialog[open]')
      confirmed.querySelector('#fw-add-port').value='18667'
      confirmed.querySelector('#fw-add-policy').value='deny'
      confirmed.querySelector('#fw-confirm-add').click()
      await sleep(160)
      if((globalThis.__fwControlCalls||0)!==beforeControl+1 ||
         globalThis.__fwLastControl?.action!=='add-drop' ||
         globalThis.__fwLastControl?.port!==18667 ||
         globalThis.__fwLastControl?.protocol!=='tcp')
        throw new Error('confirmed deny did not call dedicated DROP policy endpoint')
      if((globalThis.__fwMutationCalls||0)!==beforeDeny) throw new Error('DROP fell back to legacy allow endpoint')
      globalThis.__fwConfirmResult=false
      const activeLink=await waitFor('#fw-rows button.fw-account-link[data-fw-account="1"]')
      activeLink.click()
      const accountEditor=await waitFor('#account-form')
      if(document.querySelector('#nav [data-page="accounts"]')?.classList.contains('active')!==true)
        throw new Error('account link did not switch navigation')
      if(accountEditor.querySelector('input[name="name"]')?.value!=='alpha')
        throw new Error('account link did not open the exact account editor')
      document.querySelector('#modal-close').click()

      document.querySelector('#nav [data-page="firewall"]').click()
      await waitFor('#fw-rows button[data-fw-act="configure"][data-fw-id="firewalld:public:443/tcp"]')
      const editExternal=async (newPort,approve) => {
        document.querySelector('#fw-rows button[data-fw-act="configure"][data-fw-id="firewalld:public:443/tcp"]').click()
        const dialog=await waitFor('#fw-policy-dialog[open]')
        const save=dialog.querySelector('#fw-policy-save')
        if(!save.disabled)throw new Error('external edit allowed without acknowledgment')
        dialog.querySelector('#fw-policy-port').value=String(newPort)
        const ack=dialog.querySelector('#fw-external-confirm')
        if(dialog.querySelector('#fw-external-confirm-label').hidden)throw new Error('external edit consent is hidden')
        ack.checked=true
        ack.dispatchEvent(new Event('change',{bubbles:true}))
        if(save.disabled)throw new Error('safe external edit cannot be submitted after consent')
        globalThis.__fwConfirmResult=approve
        save.click()
        await sleep(180)
      }
      const beforeEdit=globalThis.__fwEditCalls||0
      await editExternal(444,false)
      if((globalThis.__fwEditCalls||0)!==beforeEdit)throw new Error('declined external edit mutated firewall')
      await editExternal(445,true)
      if((globalThis.__fwEditCalls||0)!==beforeEdit+1 ||
         globalThis.__fwLastEdit?.newPort!==445 ||
         globalThis.__fwLastEdit?.newProtocol!=='tcp' ||
         globalThis.__fwLastEdit?.externalAcknowledged!==true ||
         globalThis.__fwLastEdit?.revision!=='111122223333444455556666')
        throw new Error('confirmed external edit did not include revision and explicit consent')
      if((globalThis.__fwMutationCalls||0)!==beforeDeny)throw new Error('external edit fell back to legacy allow path')
      globalThis.__fwConfirmResult=false
      mark('pass', `clonePort=${portInput.value};legacyHits=0;fallbackDest=${dest};layout=contained;globalXray=ok;firewallEdits=guarded`)
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
"$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage --virtual-time-budget=7000 --dump-dom "http://127.0.0.1:$PORT/" > "$DOM" 2>"$TMP/browser.log" || {
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
echo "Browser frontend regression passed"
