/* Live version handoff. The HTML meta is stamped by the server, never
   inferred from the newest API response (which may already be a new build). */
(() => {
  'use strict'
  const loaded = document.querySelector('meta[name="xport-build-version"]')?.content || ''
  const state = {checking:false, waiting:false, seen:'', banner:null, dirty:false}
  const loggedIn = () => !document.getElementById('app')?.classList.contains('hidden')
  // A blurred but unsaved settings field is still unsaved. Track changes
  // conservatively rather than silently reloading after input loses focus.
  document.addEventListener('input',event=>{
    const target=event.target
    // Logging in is not an unsaved edit to the running control panel.
    if(!loggedIn()||!target?.closest?.('#app'))return
    if(!target.matches?.('input,select,textarea,[contenteditable="true"]'))return
    if(target.id==='fw-search'||target.type==='search')return
    state.dirty=true
  },true)
  function editing() {
    if(state.dirty)return true
    if (document.querySelector('dialog[open], #modal:not(.hidden), .fw-edit-input')) return true
    const active = document.activeElement
    return Boolean(active?.closest?.('#app')&&
      active.matches?.('input:not([type="checkbox"]), textarea, select, [contenteditable="true"]'))
  }
  function decision(info, isEditing=editing()) {
    if (!loaded || !info?.version || loaded.includes('__XPORT_VERSION__')) return 'unknown'
    if (info.version===loaded) return 'same'
    if (info.firewallPending) return 'pending'
    if (isEditing) return 'editing'
    return 'reload'
  }
  function banner() {
    if (state.banner) return state.banner
    const node = document.createElement('div')
    node.id = 'xport-version-banner'
    node.className = 'xport-version-banner'
    node.hidden = true
    node.setAttribute('role','status')
    node.innerHTML = '<div class="xport-version-copy"><strong>新版 X-port 已就绪</strong>'+
      '<small id="xport-version-reason"></small></div>'+
      '<button class="btn ghost" type="button" id="xport-version-refresh">刷新页面</button>'
    document.body.appendChild(node)
    node.querySelector('#xport-version-refresh').addEventListener('click',()=>{void check(true,true)})
    state.banner = node
    return node
  }
  function notice(reason) {
    const node=banner()
    node.hidden=false
    node.querySelector('#xport-version-reason').textContent=reason
  }
  function hideNotice() {if(state.banner)state.banner.hidden=true}
  function reloadTo(version) {
    const next=new URL(location.href)
    next.searchParams.set('_xport_ui',version)
    location.replace(next.toString())
  }
  async function check(force=false,manual=false) {
    if(state.checking || (!force && !loggedIn())) return null
    state.checking=true
    try {
      const response=await fetch('/api/xport/version',{
        method:'GET',credentials:'same-origin',cache:'no-store',
        headers:{'Accept':'application/json'}
      })
      if(!response.ok)return null
      const info=await response.json()
      // A restart invalidates all in-memory login sessions. The version
      // endpoint remains public (version only); firewall pending metadata
      // remains authenticated and is fetched only when a build changed.
      if(info?.version && info.version!==loaded){
        const guard=await fetch('/api/xport/update-state',{
          method:'GET',credentials:'same-origin',cache:'no-store',
          headers:{'Accept':'application/json'}
        })
        if(guard.ok){
          const stateInfo=await guard.json()
          info.firewallPending=Boolean(stateInfo.firewallPending)
        }else if(guard.status===401){
          // The browser needs a fresh login after restart. Its old session
          // cannot confirm pending rules; native recovery stays independent.
          info.sessionExpired=true
        }else{
          notice('无法确认防火墙待处理状态；暂缓自动刷新。')
          return info
        }
      }
      const result=decision(info)
      state.seen=String(info.version||'')
      if(result==='reload'){reloadTo(info.version)}
      else if(result==='pending')notice('防火墙还有未确认的恢复任务；确认完成后自动刷新。')
      else if(result==='editing'){
        if(manual && window.confirm('页面可能存在未保存的输入。确认放弃这些更改并刷新吗？'))
          reloadTo(info.version)
        else notice('检测到正在编辑或尚未保存的输入，请确认后手动刷新。')
      }
      else if(result==='same')hideNotice()
      return info
    } catch {
      // A brief connection refusal is normal while systemd restarts X-port.
      return null
    } finally {state.checking=false}
  }
  async function waitFor(version) {
    if(state.waiting)return
    state.waiting=true
    const end=Date.now()+120000
    try {
      while(Date.now()<end){
        const info=await check(true)
        if(info?.version===version || (info?.version && info.version!==loaded)){
          // check() either initiated navigation or left an actionable banner.
          return
        }
        await new Promise(resolve=>setTimeout(resolve,1100))
      }
      notice('服务器尚未确认完成更新。请检查 X-port 状态，再刷新页面。')
    } finally {state.waiting=false}
  }
  window.XportVersionMonitor=Object.freeze({check,waitFor,decision,loaded})
  // Other tabs detect a new server version without paying the cost of
  // full firewall process/Docker inspection on every poll.
  setInterval(()=>{if(!document.hidden)void check()},20000)
  document.addEventListener('visibilitychange',()=>{if(!document.hidden)void check()})
})()
