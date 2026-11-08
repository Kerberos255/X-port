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
# Standalone browser tests serve static files, so emulate the Go HTML stamp.
s = s.replace('__XPORT_VERSION__', 'browser-test')
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
  globalThis.__mockAccount = account
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
  globalThis.__fwWriteHold = false
  globalThis.__fwReleaseWrite = null
  globalThis.__accountToggleHold = false
  globalThis.__accountReleaseWrite = null
  globalThis.__accountToggleCalls = 0
  globalThis.__accountDeleteCalls = 0
  globalThis.__accountDeleteHold = false
  globalThis.__accountDeleteRelease = null
  globalThis.__mockAccountDeleted = false
  globalThis.__mockCreatedAccount = null
  globalThis.__mockCreateCalls = 0
  globalThis.__mockCreateMode = 'success'
  globalThis.__mockCloneCalls = 0
  globalThis.__mockAccountResetCalls = 0
  globalThis.__mockBackupCalls = 0
  globalThis.__mockBackups = []
  globalThis.__mockXrayActive = true
  globalThis.__mockXrayVersion = 'test'
  globalThis.__mockXrayLatest = 'test-next'
  globalThis.__mockXrayAvailable = false
  globalThis.__mockXrayRestartCalls = 0
  globalThis.__mockGeodataCurrent = 'test'
  globalThis.__mockGeodataLatest = 'test-next'
  globalThis.__mockGeodataAvailable = false
  globalThis.__mockGlobalConfig = {routing:{},dns:{},outbounds:[],policy:{}}
  globalThis.__mockSettings = {panelListen:'127.0.0.1:8080',panelBasePath:'/',panelCertFile:'',
    panelKeyFile:'',panelDomain:'',panelTLS:false,xrayApiPort:10085,portMin:20000,
    portMax:60000,defaultRealitySni:'',defaultRealityDest:'',adminUsername:'admin'}
  globalThis.__fwExtraRules = []
  globalThis.__fwControlPending = false
  globalThis.__pendingConfirmSeenWhileBusy = null
  globalThis.__xportServerVersion = 'browser-test'
  globalThis.__xportPending = false
  globalThis.confirm = message => {
    globalThis.__fwConfirmCalls++
    if(globalThis.__fwControlPending && String(message).includes('规则已临时修改')){
      globalThis.__pendingConfirmSeenWhileBusy=Boolean(document.querySelector('#operation-progress')?.open)
      return false
    }
    return globalThis.__fwConfirmResult
  }
  let fwNote = '主节点 · REALITY'
  let fwPurpose = 'Xray · VLESS'
  let fwExternalPort = 443
  let fwExternalProto = 'tcp'
  globalThis.fetch = (input, init = {}) => {
    const u = new URL(typeof input === 'string' ? input : input.url, location.href)
    const path = u.pathname
    const method = String(init.method || 'GET').toUpperCase()
    if (path === '/api/overview') return json({
      system: {hostname:'ci-node', os:'Linux', distribution:'Linux', kernelVersion:'test', systemType:'linux', hostAddress:'127.0.0.1',hostIPv6:'2001:db8::1234',cpuCores:4,load5:0.2,load15:0.3, bootTime:0,
        uptimeSeconds:100, cpuPercent:1, load1:0.1, memoryUsed:1, memoryTotal:2, diskUsed:1, diskTotal:2,
        networkRx:0, networkTx:0, xrayVersion:globalThis.__mockXrayVersion, xrayActive:globalThis.__mockXrayActive},
      accounts: {total:1, enabled:1}, xportVersion:'browser-test'
    })
    if (path === '/api/accounts' && method === 'GET') return json({accounts:[
      ...(globalThis.__mockAccountDeleted?[]:[account]),
      ...(globalThis.__mockCreatedAccount?[globalThis.__mockCreatedAccount]:[])
    ]})
    if (path === '/api/accounts' && method === 'POST') {
      globalThis.__mockCreateCalls++
      const body=JSON.parse(init.body||'{}')
      if(globalThis.__mockCreateMode==='conflict-recovered'){
        globalThis.__mockCreatedAccount={id:2,name:body.name,port:body.port||23741,
          protocol:body.protocol,enabled:Boolean(body.enabled),editable:true,upBytes:0,downBytes:0,
          quotaBytes:Number(body.quotaBytes||0),expiryTime:Number(body.expiryTime||0),
          monthlyReset:Boolean(body.monthlyReset)}
        return json({error:'port '+(body.port||23741)+' is already used by "'+body.name+'"'},400)
      }
      if(globalThis.__mockCreateMode==='real-conflict')
        return json({error:'port '+(body.port||23741)+' is already used by "existing"'},400)
      globalThis.__mockCreatedAccount={id:2,name:body.name,port:body.port||23741,
        protocol:body.protocol,enabled:Boolean(body.enabled),editable:true,upBytes:0,downBytes:0,
          quotaBytes:Number(body.quotaBytes||0),expiryTime:Number(body.expiryTime||0),
          monthlyReset:Boolean(body.monthlyReset)}
      return json(globalThis.__mockCreatedAccount,201)
    }
    if (path === '/api/accounts/1/clone' && method === 'POST') {
      globalThis.__mockCloneCalls++
      const input=JSON.parse(init.body||'{}')
      globalThis.__mockCreatedAccount={id:2,name:input.name,port:input.port||23741,
        protocol:account.protocol,enabled:true,editable:true,upBytes:0,downBytes:0}
      return json(globalThis.__mockCreatedAccount,201)
    }
    if (path === '/api/accounts/1/traffic/reset' && method === 'POST') {
      globalThis.__mockAccountResetCalls++
      account.upBytes=0;account.downBytes=0
      return json({ok:true})
    }
    if (path === '/api/accounts/1' && method === 'DELETE') {
      globalThis.__accountDeleteCalls++
      if(globalThis.__accountDeleteHold)
        return new Promise(resolve=>{globalThis.__accountDeleteRelease=()=>{globalThis.__mockAccountDeleted=true;resolve(json({ok:true}))}})
      globalThis.__mockAccountDeleted=true
      return json({ok:true})
    }
    if (path === '/api/accounts/1' && method === 'PUT') {
      const body=JSON.parse(init.body||'{}')
      Object.assign(account,{name:body.name,port:body.port,enabled:Boolean(body.enabled),
        monthlyReset:Boolean(body.monthlyReset),quotaBytes:Number(body.quotaBytes||0),
        expiryTime:Number(body.expiryTime||0)})
      return json(account)
    }
    if (path === '/api/accounts/1' && method === 'GET') return json(account)
    if (path === '/api/accounts/1/enabled' && method === 'PATCH') {
      globalThis.__accountToggleCalls++
      const next=Boolean(JSON.parse(init.body||'{}').enabled)
      if(globalThis.__accountToggleHold)
        return new Promise(resolve=>{globalThis.__accountReleaseWrite=()=>{account.enabled=next;resolve(json({ok:true}))}})
      account.enabled=next
      return json({ok:true})
    }
    if (path === '/api/accounts/1/expert' && method === 'GET') return json(expert)
    if (path === '/api/accounts/online') return json({connections:{}})
    if (path === '/api/accounts/port-suggestion') return json({port:23741,min:20000,max:60000})
    if (path === '/api/firewall' && method === 'GET') return json({
      backend:'firewalld',active:true,writable:!globalThis.__fwReadOnly,unsafeReason:globalThis.__fwReadOnly?'独立 INPUT 过滤链需要检查':'',managers:['firewalld'],defaultZone:'public',zones:['public'],services:{public:['ssh']},
      notice:'预览：简单端口规则展示；底层链可能影响结果',rules:[
        {id:'firewalld:public:22/tcp',port:22,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'未识别',purposeSource:'unknown',usage:'unused',note:'保留规则',protected:true,xray:false},
        {id:'firewalld:public:'+fwExternalPort+'/'+fwExternalProto,port:fwExternalPort,protocol:fwExternalProto,zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'HTTPS',purposeSource:'manual',note:'网站入口',protected:false,xray:false,kind:'port',action:'allow',managed:false,revision:'111122223333444455556666'},
        {id:'firewalld:public:2053/tcp',port:2053,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:fwPurpose,purposeSource:'xray',note:fwNote,protected:false,managed:true,xray:true,usage:'used',usageDetails:[{port:2053,protocol:'tcp',pid:1242,name:'xray'}],action:'allow',kind:'port',canToggle:true,accountId:1,accountName:'Alpha inbound',accountEnabled:true},
        {id:'firewalld:public:8443/tcp',port:8443,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:false,access:'区域：public',purpose:'Xray · Trojan',purposeSource:'xray',note:'临时放行，需同步',protected:false,xray:true},
        {id:'firewalld:public:rich:443/udp:deny:ipv4',port:443,protocol:'udp',family:'ipv4',zone:'public',backend:'firewalld',runtime:true,permanent:true,action:'deny',kind:'rich',access:'区域：public',purpose:'未识别',protected:true,usage:'unused'},
        {id:'firewalld:public:rich:443/udp:deny:ipv6',port:443,protocol:'udp',family:'ipv6',zone:'public',backend:'firewalld',runtime:true,permanent:true,action:'deny',kind:'rich',access:'区域：public',purpose:'未识别',protected:true,usage:'unused'},
        {id:'firewalld:public:51820/udp',port:51820,protocol:'udp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'未识别',purposeSource:'unknown',note:'WireGuard 站点互联',protected:false,xray:false,usage:'unused',action:'deny'},
        {id:'firewalld:public:22022/tcp',port:22022,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'SSH · 管理端口',purposeSource:'ssh',note:'',protected:true,xray:false,usage:'used',usageDetails:[{port:22022,protocol:'tcp',pid:12345,name:'sshd'}],action:'allow',kind:'port',canToggle:false},
        {id:'firewalld:public:39000-40000/tcp',port:0,rangeStart:39000,rangeEnd:40000,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'FTP 被动模式 · Pure-FTPd（配置匹配）',purposeSource:'ftp-config',note:'',protected:true,xray:false,usage:'unused',action:'allow',kind:'port',canToggle:false},
        {id:'firewalld:public:8444/tcp',port:8444,protocol:'tcp',zone:'public',backend:'firewalld',runtime:true,permanent:true,access:'区域：public',purpose:'未识别',purposeSource:'unknown',note:'',protected:true,xray:false,usage:'used',usageDetails:[{port:8444,protocol:'tcp',name:'web-svc',container:true}],action:'allow',kind:'port',canToggle:false}
        ,...globalThis.__fwExtraRules
      ]})
    if (path === '/api/firewall/notes' && method === 'PUT') {
      fwNote = JSON.parse(init.body).note
      return json({note:fwNote})
    }
    if (path === '/api/firewall/purpose' && method === 'PUT') {
      fwPurpose=JSON.parse(init.body).purpose
      return json({purpose:fwPurpose})
    }
    if (path === '/api/firewall/raw' && method === 'GET') return json({ruleset:'table inet firewalld {\\n chain INPUT { type filter hook input priority 10; }\\n}'})
    if (path === '/api/firewall/rules' && method === 'POST') {
      globalThis.__fwMutationCalls=(globalThis.__fwMutationCalls||0)+1
      const input=JSON.parse(init.body||'{}')
      const commit=()=>{
        if(input.action==='add')
          globalThis.__fwExtraRules.push({
            id:'firewalld:'+input.zone+':'+input.port+'/'+input.protocol,
            port:input.port,protocol:input.protocol,zone:input.zone,
            backend:'firewalld',runtime:true,permanent:true,managed:true,
            protected:false,kind:'port',action:'allow',purpose:'未识别',note:'',usage:'unused'
          })
        if(input.action==='remove')
          globalThis.__fwExtraRules=globalThis.__fwExtraRules.filter(
            r=>r.port!==input.port||r.protocol!==input.protocol||r.zone!==input.zone)
        return json({ok:true,pending:false})
      }
      if(globalThis.__fwWriteHold)
        return new Promise(resolve=>{globalThis.__fwReleaseWrite=()=>resolve(commit())})
      return commit()
    }
    if (path === '/api/firewall/control' && method === 'POST') {
      globalThis.__fwControlCalls=(globalThis.__fwControlCalls||0)+1
      globalThis.__fwLastControl=JSON.parse(init.body||'{}')
      return json({ok:true,pending:globalThis.__fwControlPending,id:'test-protected',token:'test-token'})
    }
    if (path === '/api/firewall/edit' && method === 'POST') {
      globalThis.__fwEditCalls=(globalThis.__fwEditCalls||0)+1
      globalThis.__fwLastEdit=JSON.parse(init.body||'{}')
      fwExternalPort=Number(globalThis.__fwLastEdit.newPort)
      fwExternalProto=String(globalThis.__fwLastEdit.newProtocol)
      return json({ok:true,pending:false})
    }
    if (path === '/api/xport/version' && method === 'GET')
      return json({version:globalThis.__xportServerVersion})
    if (path === '/api/xport/update' && method === 'GET')
      return json({current:globalThis.__xportServerVersion,latest:'browser-test-next',available:false})
    if (path === '/api/xport/update-state' && method === 'GET')
      return json({firewallPending:globalThis.__xportPending})
    if (path === '/api/xray/restart' && method === 'POST') {
      globalThis.__mockXrayRestartCalls++
      return json({ok:true})
    }
    if (path === '/api/xray/update' && method === 'GET') return json({
      current:globalThis.__mockXrayVersion,latest:globalThis.__mockXrayLatest,
      available:globalThis.__mockXrayAvailable
    })
    if (path === '/api/xray/update' && method === 'POST') {
      globalThis.__mockXrayVersion=globalThis.__mockXrayLatest
      globalThis.__mockXrayAvailable=false
      return json({current:globalThis.__mockXrayVersion})
    }
    if (path === '/api/xray/rollback' && method === 'GET') return json({current:globalThis.__mockXrayVersion,previous:'test',available:true})
    if (path === '/api/xray/rollback' && method === 'POST') {
      globalThis.__mockXrayVersion='test'
      return json({current:'test'})
    }
    if (path === '/api/xray/geodata' && method === 'GET') return json({
      current:globalThis.__mockGeodataCurrent,latest:globalThis.__mockGeodataLatest,
      available:globalThis.__mockGeodataAvailable
    })
    if (path === '/api/xray/geodata' && method === 'POST') {
      globalThis.__mockGeodataCurrent=globalThis.__mockGeodataLatest
      globalThis.__mockGeodataAvailable=false
      return json({current:globalThis.__mockGeodataCurrent})
    }
    if (path === '/api/settings' && method === 'PUT') {
      Object.assign(globalThis.__mockSettings,JSON.parse(init.body||'{}'))
      return json({restartRequired:false})
    }
    if (path === '/api/settings') return json(globalThis.__mockSettings)
    if (path === '/api/backups/import' && method === 'POST') {
      globalThis.__mockBackupCalls++
      const entry={name:'browser-import-'+globalThis.__mockBackupCalls+'.json.gz',createdAt:Date.now(),size:1024}
      globalThis.__mockBackups.push(entry)
      return json(entry,201)
    }
    if (path === '/api/backups' && method === 'POST') {
      globalThis.__mockBackupCalls++
      const entry={name:'browser-backup-'+globalThis.__mockBackupCalls+'.json.gz',createdAt:Date.now(),size:1024}
      globalThis.__mockBackups.push(entry)
      return json(entry,201)
    }
    if (path === '/api/backups' && method === 'GET') return json({backups:globalThis.__mockBackups})
    if (path.startsWith('/api/backups/') && method === 'DELETE') {
      const name=decodeURIComponent(path.split('/').at(-1))
      globalThis.__mockBackups=globalThis.__mockBackups.filter(x=>x.name!==name)
      return json({ok:true})
    }
    if (path === '/api/logs') return json({logs:'browser-log-ok'})
    if (path === '/api/xray/config' && method === 'PUT') {
      globalThis.__mockGlobalConfig=JSON.parse(init.body||'{}').config
      return json({config:globalThis.__mockGlobalConfig})
    }
    if (path === '/api/xray/config') return json({config:globalThis.__mockGlobalConfig})
    return json({ok:true})
  }
  // Act on the real custom confirmation rather than a browser-native
  // confirm(). This also proves the dialog was actually opened.
  const confirmDialog=document.querySelector('#operation-confirm')
  const nativeShow=confirmDialog.showModal.bind(confirmDialog)
  confirmDialog.showModal=()=>{
    nativeShow()
    globalThis.__fwConfirmCalls++
    const text=confirmDialog.querySelector('#operation-confirm-detail').textContent
    const pending=globalThis.__fwControlPending&&text.includes('120 秒')
    if(pending)globalThis.__pendingConfirmSeenWhileBusy=Boolean(document.querySelector('#operation-progress')?.open)
    const agree=pending?false:Boolean(globalThis.__fwConfirmResult)
    queueMicrotask(()=>confirmDialog.querySelector(agree?'#operation-confirm-apply':'#operation-confirm-cancel').click())
  }
})()
JS

cat > "$TMP/__driver.js" <<'JS'
(() => {
  const mark = (status, detail='') => {
    document.documentElement.dataset.browserRegression = status
    document.documentElement.dataset.browserRegressionDetail = detail
  }
  mark('running','driver initialized')
  window.addEventListener('error',e=>mark('error',String(e.message||'script error')))
  window.addEventListener('unhandledrejection',e=>mark('rejection',String(e.reason||'unhandled rejection')))
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
      // Execute the real clone submit twice and verify one POST + new ID.
      document.querySelector('.account-row button[data-act="clone"]').click()
      const liveClone=await waitFor('#clone-form')
      await waitForValue('#clone-port','23741')
      liveClone.querySelector('input[name="name"]').value='cloned-browser'
      const priorClones=globalThis.__mockCloneCalls
      const cloneEvent={preventDefault(){},currentTarget:liveClone}
      await Promise.all([liveClone.onsubmit(cloneEvent),liveClone.onsubmit(cloneEvent)])
      if(globalThis.__mockCloneCalls!==priorClones+1)
        throw new Error('clone must send exactly one write despite simultaneous submits')
      if(!state.accounts.some(x=>Number(x.id)===2&&x.name==='cloned-browser'&&Number(x.port)===23741))
        throw new Error('clone did not confirm newly created ID, name and port')
      if(XportOperation.busy||!document.querySelector('#modal').classList.contains('hidden'))
        throw new Error('clone operation did not finish and close its editor')
      globalThis.__mockCreatedAccount=null
      await loadAccounts()

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
      // An edit with a changed quota must persist all configured account fields.
      const currentEdit=document.querySelector('.account-row button[data-act="edit"]')
      currentEdit.click()
      const editForm=await waitFor('#account-form')
      editForm.querySelector('input[name="quota"]').value='1.25'
      await editForm.onsubmit({preventDefault(){},currentTarget:editForm})
      if(Number(state.accounts.find(x=>x.id===1)?.quotaBytes)!==Math.round(1.25*1073741824))
        throw new Error('account edit quota not persisted in authoritative readback')
      if(XportOperation.busy||!document.querySelector('#modal').classList.contains('hidden'))
        throw new Error('account edit did not release progress and close')

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
      const purposeButton=document.querySelector('#fw-rows button[data-fw-id="firewalld:public:2053/tcp"][data-fw-key="purpose"]')
      purposeButton.click()
      const purposeInput=await waitFor('.fw-edit-input')
      purposeInput.value='浏览器回归用途'
      document.querySelector('#fw-rows button[data-fw-act="save"]').click()
      await waitForTextContains('#fw-rows button[data-fw-id="firewalld:public:2053/tcp"][data-fw-key="purpose"] span','浏览器回归用途')
      if(document.querySelector('#nav [data-page="firewall"]')?.classList.contains('active')!==true) throw new Error('firewall navigation not active')
      const accountLink=document.querySelector('#fw-rows button.fw-account-link[data-fw-account="1"]')
      if(!accountLink||!accountLink.textContent.includes('Alpha inbound')) throw new Error('Xray account linkage is not visible')
      const accountRow=accountLink.closest('tr')
      if(!accountRow.querySelector('.fw-usage.used')||!accountRow.querySelector('.fw-policy.allow')) throw new Error('account usage or allow badge missing')
      const fwTable=document.querySelector('#page-firewall .fw-table')
      if(!fwTable||fwTable.querySelectorAll('thead th').length!==9)throw new Error('firewall source IP column missing')
      const fwFirstRow=fwTable.querySelector('#fw-rows tr')
      if(fwFirstRow?.querySelectorAll('td').length!==9)throw new Error('firewall rows must have 9 aligned cells')
      const v4row=document.querySelector('#fw-rows button[data-fw-id="firewalld:public:rich:443/udp:deny:ipv4"][data-fw-act="detail"]')?.closest('tr')
      const v6row=document.querySelector('#fw-rows button[data-fw-id="firewalld:public:rich:443/udp:deny:ipv6"][data-fw-act="detail"]')?.closest('tr')
      if(!v4row||!v6row||v4row.querySelector('.fw-ip-cell')?.textContent.trim()!=='0.0.0.0/0'||
         v6row.querySelector('.fw-ip-cell')?.textContent.trim()!=='::/0'||
         v4row.querySelector('.fw-policy.conflict')||v6row.querySelector('.fw-policy.conflict'))
        throw new Error('IPv4/IPv6 identical ports must have distinct visible IPs without conflict')
      const defaultSSH=document.querySelector('#fw-rows button[data-fw-id="firewalld:public:22/tcp"][data-fw-act="detail"]')?.closest('tr')
      if(!defaultSSH||defaultSSH.textContent.includes('SSH · 管理端口'))
        throw new Error('unused default SSH port mislabeled as actual management listener')
      const bothIP=document.querySelector('#fw-rows button[data-fw-id="firewalld:public:443/tcp"][data-fw-act="detail"]')?.closest('tr')?.querySelector('.fw-ip-cell')
      if(!bothIP?.textContent.includes('0.0.0.0/0')||!bothIP?.textContent.includes('::/0'))
        throw new Error('dual-family port missing either wildcard address')
      const monitor=window.XportVersionMonitor
      if(!monitor||monitor.loaded!=='browser-test')throw new Error('loaded-version stamp unavailable')
      if(monitor.decision({version:'browser-next',firewallPending:true},false)!=='pending'||
         monitor.decision({version:'browser-next',firewallPending:false},true)!=='editing'||
         monitor.decision({version:'browser-next',firewallPending:false},false)!=='reload')
        throw new Error('version monitor must defer reload during firewall pending or editing')
      globalThis.__xportServerVersion='browser-next'
      globalThis.__xportPending=true
      await monitor.check(true)
      const warning=await waitFor('#xport-version-banner:not([hidden])')
      if(!warning.textContent.includes('防火墙'))throw new Error('pending operation banner missing')
      globalThis.__xportServerVersion='browser-test'
      globalThis.__xportPending=false
      await monitor.check(true)
      if(!warning.hidden)throw new Error('outdated version notice not cleared')
      const sshRow=document.querySelector('#fw-rows button[data-fw-act="detail"][data-fw-id="firewalld:public:22022/tcp"]')?.closest('tr')
      if(!sshRow?.textContent.includes('sshd')||!sshRow?.querySelector('button.fw-switch[disabled]')) throw new Error('nonstandard SSH process or protected state missing')
      const ftpRow=document.querySelector('#fw-rows button[data-fw-act="detail"][data-fw-id="firewalld:public:39000-40000/tcp"]')?.closest('tr')
      if(!ftpRow?.textContent.includes('39000–40000')||!ftpRow?.textContent.includes('FTP 被动模式')||!ftpRow?.querySelector('button.fw-switch[disabled]')) throw new Error('read-only passive FTP range missing')
      // All port labels use a consistent stacked layout. Ranges must remain
      // on ONE line and protocols must sit beneath them, not wrap unpredictably.
      for(const row of [accountRow,ftpRow,v4row]){
        const cell=row?.querySelector('.fw-port-cell')
        const number=cell?.querySelector('strong')
        const protocol=cell?.querySelector('small')
        if(!number||!protocol||getComputedStyle(cell).flexDirection!=='column'||
           !['TCP','UDP'].includes(protocol.textContent.trim())||
           getComputedStyle(number).whiteSpace!=='nowrap')
          throw new Error('port/protocol must be a predictable two-line stack')
        if(number.getBoundingClientRect().bottom>protocol.getBoundingClientRect().top+1)
          throw new Error('port and protocol overlap instead of stacking')
        if(number.scrollWidth>number.clientWidth+1)
          throw new Error('port range clipped at desktop width: '+number.textContent)
      }
      const longIPv6='2001:0db8:abcd:1234:5678:9abc:def0:1234/64'
      const ipv6Cell=v6row.querySelector('.fw-ip-cell')
      const ipv6Label=ipv6Cell.querySelector('span')
      if(ipv6Label.title!=='::/0')throw new Error('IP cell must expose full address in title')
      const originalIPv6=ipv6Label.textContent
      ipv6Label.textContent=longIPv6
      ipv6Label.title=longIPv6
      const ipv6CSS=getComputedStyle(ipv6Label)
      if(ipv6CSS.whiteSpace!=='nowrap'||ipv6CSS.overflow!=='hidden'||
         ipv6CSS.textOverflow!=='ellipsis'||ipv6Label.scrollWidth<=ipv6Label.clientWidth)
        throw new Error('long IPv6 must be truncated to one visual line')
      if(ipv6Cell.getBoundingClientRect().right>v6row.querySelectorAll('td')[1].getBoundingClientRect().right+1)
        throw new Error('long IPv6 overflows IP column')
      if(ipv6Label.title!==longIPv6)throw new Error('full IPv6 hover address missing')
      ipv6Label.textContent=originalIPv6
      ipv6Label.title=originalIPv6
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

      // Shared progress is truly centered, modal and non-dismissible.
      // A deferred Xray toggle exercises the real API write path.
      const progress=document.querySelector('#operation-progress')
      if(!progress||!window.XportOperation)throw new Error('shared operation progress missing')
      document.querySelector('#nav [data-page="accounts"]').click()
      const accountToggle=await waitFor('.account-row .account-toggle')
      globalThis.__accountToggleHold=true
      const accountPromise=toggleAccountV3(1,false,accountToggle)
      await waitFor('#operation-progress[open]')
      if(!progress.querySelector('#operation-progress-title').textContent.includes('停用账号'))
        throw new Error('account toggle missing operation-specific title')
      const bounds=progress.getBoundingClientRect()
      if(Math.abs((bounds.top+bounds.bottom)/2-innerHeight/2)>Math.max(28,innerHeight*.08)||
         Math.abs((bounds.left+bounds.right)/2-innerWidth/2)>Math.max(28,innerWidth*.08))
        throw new Error('operation progress must be centered in viewport')
      if(getComputedStyle(progress).position!=='fixed' || !progress.open)
        throw new Error('operation progress must be modal')
      const canceled=new Event('cancel',{cancelable:true})
      progress.dispatchEvent(canceled)
      if(!canceled.defaultPrevented||!progress.open)throw new Error('progress dialog must block Escape dismissal')
      const oldWrites=globalThis.__accountToggleCalls
      await XportOperation.run({
        title:'不应执行',perform:async()=>{globalThis.__accountToggleCalls++}
      }).then(()=>{throw new Error('parallel operation unexpectedly succeeded')},
        err=>{if(!/正在执行/.test(err.message))throw err})
      if(globalThis.__accountToggleCalls!==oldWrites)
        throw new Error('duplicate account write was not blocked')
      globalThis.__accountReleaseWrite()
      await accountPromise
      globalThis.__accountToggleHold=false
      if(progress.open||XportOperation.busy)
        throw new Error('progress did not close after refreshed account list')
      // Traffic reset must confirm the account's actual zeroed counters.
      accountToggle.disabled=false
      globalThis.__mockAccount.upBytes=800
      globalThis.__mockAccount.downBytes=700
      await loadAccounts()
      globalThis.__fwConfirmResult=true
      const resetBefore=globalThis.__mockAccountResetCalls
      await resetTrafficV3(1)
      globalThis.__fwConfirmResult=false
      if(globalThis.__mockAccountResetCalls!==resetBefore+1||
         state.accounts.find(x=>x.id===1)?.upBytes!==0||
         state.accounts.find(x=>x.id===1)?.downBytes!==0)
        throw new Error('account traffic reset did not confirm zero counters from the server')
      // Ordinary validation and success notices share the centered visual style,
      // but never trap focus or block the page.
      const notice=document.querySelector('#toast')
      toast('端口必须是 1–65535',true)
      if(notice.classList.contains('hidden')||notice.getAttribute('role')!=='alert')
        throw new Error('validation error notice is not accessible')
      const noticeRect=notice.getBoundingClientRect()
      if(Math.abs((noticeRect.top+noticeRect.bottom)/2-innerHeight/2)>Math.max(25,innerHeight*.06) ||
         Math.abs((noticeRect.left+noticeRect.right)/2-innerWidth/2)>Math.max(25,innerWidth*.06))
        throw new Error('validation notice is not centered')
      if(getComputedStyle(notice).pointerEvents!=='none' || parseFloat(getComputedStyle(notice).fontSize)<13)
        throw new Error('notice must be legible and nonblocking')
      const oldNoticeTheme=document.documentElement.dataset.theme
      document.documentElement.dataset.theme='light'
      const lightNotice=getComputedStyle(notice)
      if(lightNotice.color!=='rgb(146, 45, 64)')
        throw new Error('light warning notice has insufficient theme contrast: '+lightNotice.color)
      document.documentElement.dataset.theme=oldNoticeTheme
      toast('账号已创建')
      if(notice.getAttribute('role')!=='status'||notice.classList.contains('bad'))
        throw new Error('success notice state was not reset')
      notice.classList.add('hidden')
      // Input prompts use the same centered confirmation surface.
      globalThis.__fwConfirmResult=true
      const entered=await XportOperation.input({
        title:'批量导出账号',detail:'输入域名',
        defaultValue:'example.com',confirmText:'开始导出'
      })
      globalThis.__fwConfirmResult=false
      if(entered!=='example.com'||document.querySelector('.operation-confirm-input'))
        throw new Error('centered input did not return its value or clean up')
      // Account modals must be centered and usable, not small/top-edge sheets.
      await openAccountModalV3(state.accounts.find(a=>a.id===1))
      const accountModal=document.querySelector('#modal .modal-card')
      if(!accountModal||!document.querySelector('#account-form'))
        throw new Error('account form modal missing')
      const accountRect=accountModal.getBoundingClientRect()
      if(Math.abs((accountRect.top+accountRect.bottom)/2-innerHeight/2)>Math.max(32,innerHeight*.08)||
         accountRect.width<600||accountRect.right>innerWidth)
        throw new Error('desktop account editor must be centered and spacious')
      document.querySelector('#modal-close').click()
      // Exercise the actual firewall add flow with a held write response.
      document.querySelector('#nav [data-page="firewall"]').click()
      globalThis.__fwConfirmResult=true
      globalThis.__fwWriteHold=true
      const priorRules=globalThis.__fwMutationCalls||0
      document.querySelector('#fw-add').click()
      const firewallAdd=await waitFor('#fw-add-dialog[open]')
      firewallAdd.querySelector('#fw-add-port').value='53341'
      firewallAdd.querySelector('#fw-add-policy').value='allow'
      firewallAdd.querySelector('#fw-confirm-add').click()
      await waitFor('#operation-progress[open]')
      if(!progress.querySelector('#operation-progress-title').textContent.includes('添加防火墙规则')||
         (globalThis.__fwMutationCalls||0)!==priorRules+1)
        throw new Error('firewall add must display modal progress once')
      if(!globalThis.__fwReleaseWrite)throw new Error('held firewall write did not start')
      globalThis.__fwReleaseWrite()
      globalThis.__fwWriteHold=false
      await sleep(75)
      if(progress.open||XportOperation.busy)
        throw new Error('firewall add progress did not close after status refresh')
      globalThis.__fwConfirmResult=false

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
      // Native recovery confirmation must be offered after the
      // non-dismissible operation progress overlay is released.
      globalThis.__fwControlPending=true
      globalThis.__fwConfirmResult=true
      const activeToggle=document.querySelector('#fw-rows button.fw-switch[data-fw-id="firewalld:public:2053/tcp"]')
      if(!activeToggle)throw new Error('test rule toggle missing')
      activeToggle.click()
      await sleep(120)
      if(globalThis.__pendingConfirmSeenWhileBusy!==false)
        throw new Error('firewall recovery confirmation appeared while operation progress was modal')
      if(document.querySelector('#operation-progress')?.open)
        throw new Error('firewall recovery confirmation left progress locked')
      globalThis.__fwControlPending=false
      globalThis.__fwConfirmResult=false
      document.querySelector('#nav [data-page="accounts"]').click()
      const deleteButton=await waitFor('.account-row button[data-act="delete"]')
      const prevDelete=globalThis.__accountDeleteCalls
      deleteButton.click()
      await sleep(45)
      if(globalThis.__accountDeleteCalls!==prevDelete)
        throw new Error('canceled centered account delete confirmation sent a write')
      globalThis.__accountDeleteHold=true
      globalThis.__fwConfirmResult=true
      deleteButton.click()
      await waitFor('#operation-progress[open]')
      if(!progress.querySelector('#operation-progress-title').textContent.includes('删除账号')||
         globalThis.__accountDeleteCalls!==prevDelete+1)
        throw new Error('confirmed delete must show centered progress and send exactly once')
      globalThis.__accountDeleteRelease()
      globalThis.__accountDeleteHold=false
      await sleep(100)
      if(progress.open||XportOperation.busy||document.querySelector('.account-row'))
        throw new Error('account delete did not finish and refresh after its single write')
      // Real browser submit: a persisted account and a port-conflict response
      // must be reconciled by its new ID without sending POST twice.
      globalThis.__mockCreateMode='conflict-recovered'
      document.querySelector('#add-account').click()
      const createForm=await waitFor('#account-form')
      createForm.querySelector('input[name="name"]').value='fresh-browser'
      const submitEvent={preventDefault(){},currentTarget:createForm}
      const countBefore=globalThis.__mockCreateCalls
      const createRequest=createForm.onsubmit(submitEvent)
      const doubleSubmit=createForm.onsubmit(submitEvent)
      await Promise.all([createRequest,doubleSubmit])
      if(globalThis.__mockCreateCalls!==countBefore+1)
        throw new Error('account create sent multiple POST requests')
      if(!state.accounts.some(x=>Number(x.id)===2&&x.name==='fresh-browser'))
        throw new Error('account create did not reconcile newly persisted ID')
      if(XportOperation.busy||document.querySelector('#modal')?.classList.contains('hidden')!==true)
        throw new Error('account creation did not close its modal')
      // A genuine 400 conflict with no new matching ID must keep the editor
      // actionable; it must never claim the account was created.
      globalThis.__mockCreatedAccount=null
      globalThis.__mockCreateMode='real-conflict'
      document.querySelector('#add-account').click()
      const conflictForm=await waitFor('#account-form')
      conflictForm.querySelector('input[name="name"]').value='new-conflict'
      const conflictButton=conflictForm.querySelector('button[type="submit"]')
      await conflictForm.onsubmit({preventDefault(){},currentTarget:conflictForm})
      if(conflictButton.disabled || conflictForm.dataset.submitting!=='0')
        throw new Error('genuine conflict did not allow correction')
      if(document.querySelector('#modal')?.classList.contains('hidden')===true)
        throw new Error('genuine conflict incorrectly closed account editor')
      document.querySelector('#modal-close').click()
      globalThis.__mockCreateMode='success'
      globalThis.__mockCreatedAccount=null

      // The remaining pages are tested with stateful, never-real API mocks.
      // Each action below must read back its persisted state.
      globalThis.__fwConfirmResult=true
      document.querySelector('#nav [data-page="xray"]').click()
      await loadLogs()
      if(!document.querySelector('#log-output').textContent.includes('browser-log-ok'))
        throw new Error('Xray log refresh did not update the visible log')
      const restarts=globalThis.__mockXrayRestartCalls
      await restartXray()
      if(globalThis.__mockXrayRestartCalls!==restarts+1)
        throw new Error('Xray restart did not submit exactly once')
      globalThis.__mockXrayAvailable=true
      await checkUpdate(false)
      await updateXray()
      if(globalThis.__mockXrayVersion!=='test-next')
        throw new Error('Xray update did not confirm target running version')
      await checkXrayRollback()
      await rollbackXray()
      if(globalThis.__mockXrayVersion!=='test')
        throw new Error('Xray rollback did not confirm prior running version')
      globalThis.__mockGeodataAvailable=true
      await checkGeodata(false)
      await updateGeodata()
      if(globalThis.__mockGeodataCurrent!=='test-next')
        throw new Error('GeoData update did not confirm persisted version')
      const configEditor=await waitFor('#xray-global-config')
      await loadGlobalXrayConfigV11(false)
      const altered={routing:{rules:[]},dns:{},outbounds:[],policy:{}}
      configEditor.value=JSON.stringify(altered)
      await saveGlobalXrayConfigV11()
      if(!sameJSONV3(globalThis.__mockGlobalConfig,altered))
        throw new Error('Xray global editor did not persist exact changed JSON')
      document.querySelector('#nav [data-page="overview"]').click()
      document.querySelector('#refresh-overview').click()
      await waitForTextContains('#toast','服务器状态已刷新')
      document.querySelector('#nav [data-page="system"]').click()
      await loadSettingsV6()
      const settingsForm=document.querySelector('#settings-form')
      settingsForm.elements.namedItem('portMin').value='20001'
      await settingsForm.onsubmit({preventDefault(){},currentTarget:settingsForm})
      if(Number(globalThis.__mockSettings.portMin)!==20001)
        throw new Error('system settings save did not persist the changed port minimum')
      // Create/import/delete backup controls must mutate and verify named rows.
      const backupBefore=globalThis.__mockBackupCalls
      await createBackup()
      if(globalThis.__mockBackupCalls!==backupBefore+1||
         !document.querySelector('#backup-list .backup-row'))
        throw new Error('backup creation did not persist and render its identity')
      await importBackupFile(new File(['gzip-fixture'],'browser.gz',{type:'application/gzip'}))
      if(globalThis.__mockBackups.length!==2)
        throw new Error('backup import did not confirm second persisted identity')
      const firstBackup=globalThis.__mockBackups[0].name
      await deleteBackup(firstBackup)
      if(globalThis.__mockBackups.some(x=>x.name===firstBackup)||
         globalThis.__mockBackups.length!==1)
        throw new Error('backup delete did not confirm exact target absence')
      const secondBackup=globalThis.__mockBackups[0].name
      await deleteBackup(secondBackup)
      if(globalThis.__mockBackups.length||document.querySelector('#backup-list .backup-row'))
        throw new Error('backup deletion left a stale row')
      // All page navigation and non-write controls should remain operable.
      document.querySelector('#check-xport-update').click()
      await sleep(50)
      if(!document.querySelector('#xport-update-state'))
        throw new Error('panel update check control missing')
      const modeBefore=document.documentElement.dataset.theme
      document.querySelector('#theme-toggle').click()
      if(document.documentElement.dataset.theme===modeBefore)
        throw new Error('theme toggle did not switch mode')
      document.querySelector('#theme-toggle').click()
      globalThis.__fwConfirmResult=false
      document.querySelector('#export-accounts').click()
      await sleep(15)
      if(document.querySelector('.operation-confirm-input'))
        throw new Error('cancelled account-export prompt left an input behind')
      document.querySelector('#restart-xport').click()
      await sleep(25)
      if(XportOperation.busy)
        throw new Error('cancelled panel restart issued an operation')
      document.querySelector('#nav [data-page="firewall"]').click()
      document.querySelector('#fw-load-raw').click()
      await waitForTextContains('#fw-raw','table inet firewalld')
      globalThis.__fwConfirmResult=false
      // A verification failure must release the modal but prevent blind retry.
      const uncertain=await XportOperation.run({
        title:'状态核对测试',perform:async()=>({ok:true}),
        verify:async()=>{throw new Error('simulated read failure')}
      }).then(()=>null,err=>err)
      if(!uncertain?.applied||progress.open||XportOperation.busy)
        throw new Error('verification failure left modal stuck or falsely signaled a clean success')
      globalThis.__fwConfirmResult=false
      mark('pass', `clonePort=${portInput.value};legacyHits=0;fallbackDest=${dest};layout=contained;globalXray=ok;firewallEdits=guarded`)
    } catch (err) {
      mark('fail', String(err?.stack || err).slice(0,900))
    }
  }, {once:true})
})()
JS

node --check "$TMP/__driver.js"

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
"$BROWSER" --headless=new --no-sandbox --disable-gpu --disable-dev-shm-usage --virtual-time-budget=18000 --dump-dom "http://127.0.0.1:$PORT/" > "$DOM" 2>"$TMP/browser.log" || {
  cat "$TMP/browser.log" >&2
  exit 1
}
if ! grep -q 'data-browser-regression="pass"' "$DOM"; then
  echo "Browser regression failed" >&2
  python3 - "$DOM" <<'PY'
import re,sys
text=open(sys.argv[1],encoding='utf-8',errors='replace').read()
html=re.search(r'<html\b[^>]*>',text,re.I)
print('Browser html state: '+(html.group(0)[:1800] if html else '<missing>'),file=sys.stderr)
print('Browser DOM bytes: '+str(len(text))+' first: '+repr(text[:400]),file=sys.stderr)
PY
  grep -o 'data-browser-regression[^>]*' "$DOM" >&2 || true
  cat "$TMP/browser.log" >&2
  exit 1
fi
grep -o 'data-browser-regression="pass"[^>]*' "$DOM" | head -1
echo "Browser frontend regression passed"
