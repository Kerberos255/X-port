/* Live version handoff. The HTML meta is stamped by the server, never
   inferred from the newest API response (which may already be a new build). */
(() => {
  'use strict'
  const loaded = document.querySelector('meta[name="xport-build-version"]')?.content || ''
  const state = {checking:false, waiting:false, seen:'', banner:null}
  const loggedIn = () => !document.getElementById('app')?.classList.contains('hidden')
  function editing() {
    if (document.querySelector('dialog[open], #modal:not(.hidden), .fw-edit-input')) return true
    const active = document.activeElement
    return Boolean(active?.matches('input:not([type="checkbox"]), textarea, select, [contenteditable="true"]'))
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
    node.querySelector('#xport-version-refresh').addEventListener('click',()=>{void check(true)})
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
  async function check(force=false) {
    if(state.checking || (!force && !loggedIn())) return null
    state.checking=true
    try {
      const response=await fetch('/api/xport/version',{
        method:'GET',credentials:'same-origin',cache:'no-store',
        headers:{'Accept':'application/json'}
      })
      if(!response.ok)return null
      const info=await response.json()
      const result=decision(info)
      state.seen=String(info.version||'')
      if(result==='reload'){reloadTo(info.version)}
      else if(result==='pending')notice('防火墙还有未确认的恢复任务；完成确认后自动刷新。')
      else if(result==='editing')notice('当前有编辑窗口；完成或取消编辑后自动刷新。')
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
