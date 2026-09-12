let xportOnlineRawConnectionsV4={}

// v0.1.1 displayed raw ESTABLISHED TCP sockets as "online", which made a
// single XHTTP client look like many devices. Prefer unique remote IPs as a
// practical client estimate, while retaining the raw socket count in tooltip.
if(typeof loadOnlineConnectionsV3==='function'){
 loadOnlineConnectionsV3=async function(render=true){
  if(state.page!=='accounts')return
  try{
   const d=await api('/api/accounts/online')
   xportOnlineConnections=d.peers||d.connections||{}
   xportOnlineRawConnectionsV4=d.connections||{}
   if(render)renderAccountsV3()
  }catch{}
 }
}

if(typeof onlinePillV3==='function'){
 onlinePillV3=function(a){
  const id=String(a.id),peers=Number(xportOnlineConnections[id]??0),connections=Number(xportOnlineRawConnectionsV4[id]??0)
  const title=peers>0
   ?`按 ESTABLISHED TCP 的远端 IP 去重，约 ${peers} 个活跃客户端；当前 ${connections} 条 TCP 连接。相同 NAT 下多个设备可能合并，UDP 不计入。`
   :'当前没有检测到 ESTABLISHED TCP 客户端；UDP 不计入在线判断。'
  return `<span class="online-pill ${peers>0?'on':''}" title="${esc(title)}"><i></i>${peers>0?`在线 ${peers}`:'离线'}</span>`
 }
}

function ensureThemeControlV4(){
 if(!$('#theme-toggle')&&typeof initThemeControl==='function')initThemeControl()
 const b=$('#theme-toggle')
 if(b)b.setAttribute('aria-label',b.title||'切换白天/夜间模式')
}

function polishAccountCopyV4(){
 const sum=$('#sum-monthly')?.closest('div')?.querySelector('small')
 if(sum)sum.textContent='累计流量'
 const list=$('#accounts-list')
 if(list&&!$('#online-note')){
  const note=document.createElement('p')
  note.id='online-note';note.className='online-note'
  note.textContent='在线数按当前 ESTABLISHED TCP 的远端 IP 去重，是设备数近似值；同一 NAT 下多个设备可能合并，UDP 不计入。'
  list.insertAdjacentElement('afterend',note)
 }
}

/* v0.1.3 overview composition. Xray updates belong on the Xray page; the
   overview keeps health where the update card used to be and swaps services
   with real-time traffic. */
function arrangeOverviewV5(){
 const grid=$('.overview-grid');if(!grid)return
 const server=$('.server-panel',grid),services=$('.services',grid),traffic=$('.traffic-panel',grid),health=$('.health-panel',grid),update=$('.update-panel',grid)
 update?.remove()
 ;[server,services,traffic,health].filter(Boolean).forEach(el=>grid.appendChild(el))
}

function formatBootTimeV5(value){
 const n=Number(value||0)
 if(!n)return '—'
 try{return new Date(n).toLocaleString([], {year:'numeric',month:'2-digit',day:'2-digit',hour:'2-digit',minute:'2-digit'})}catch{return '—'}
}

function renderServerDetailsV5(){
 const panel=$('.server-panel'),sys=state.overview?.system||{};if(!panel)return
 let box=$('#server-details')
 if(!box){
  box=document.createElement('div');box.id='server-details';box.className='server-details'
  panel.querySelector('.server-core')?.insertAdjacentElement('afterend',box)
 }
 const fields=[
  ['发行版本',sys.distribution||sys.os||'—'],
  ['内核版本',sys.kernelVersion||'—'],
  ['系统类型',sys.systemType||sys.os||'—'],
  ['主机地址',sys.hostAddress||'—'],
  ['启动时间',formatBootTimeV5(sys.bootTime)]
 ]
 box.innerHTML=fields.map(([k,v],i)=>`<div class="server-detail ${i===4?'wide':''}"><small>${esc(k)}</small><b title="${esc(v)}">${esc(v)}</b></div>`).join('')
}

if(typeof loadOverview==='function'){
 const loadOverviewV5=loadOverview
 loadOverview=async function(...args){
  const out=await loadOverviewV5(...args)
  renderServerDetailsV5()
  return out
 }
}

/* Real DOM content for account switches: do not use a floating ::after label.
   This keeps 开/关 clipped inside the switch even when the account row narrows. */
if(typeof renderAccountsV3==='function'){
 const renderAccountsV5=renderAccountsV3
 renderAccountsV3=function(){
  const out=renderAccountsV5()
  $$('.account-toggle').forEach(b=>{
   const on=b.classList.contains('on')
   let label=b.querySelector('.account-toggle-state')
   if(!label){label=document.createElement('span');label.className='account-toggle-state';b.appendChild(label)}
   label.textContent=on?'开':'关'
   b.setAttribute('aria-label',on?'停用账号':'启用账号')
  })
  return out
 }
}

/* Put the two account policy switches in the footer, left of the actions. */
function accountCoreFieldsV5(a){
 const expiry=a.expiryTime?new Date(Number(a.expiryTime)-new Date().getTimezoneOffset()*60000).toISOString().slice(0,16):'',qgb=a.quotaBytes?(Number(a.quotaBytes)/1073741824).toFixed(2):''
 return `<label>${reqLabelV3('账号')}<input name="name" value="${esc(a.name||'')}" required maxlength="80"></label><label>${reqLabelV3('端口')}<input name="port" type="number" min="1" max="65535" value="${a.port||''}" required placeholder="1 - 65535"></label><label>流量上限 / GB<input name="quota" type="number" min="0" step="0.1" value="${qgb}" placeholder="0 = 不限"></label><label>到期时间<input name="expiry" type="datetime-local" value="${expiry}"></label>`
}

function editorPolicySwitchV5(name,label,checked){
 return `<label class="editor-policy"><input name="${name}" type="checkbox" ${checked?'checked':''}><span class="editor-policy-track"><i></i></span><span>${label}</span></label>`
}

if(typeof accountFormHTMLV3==='function'){
 accountFormHTMLV3=function(a={}){
  const p=String(a.protocol||'vless').toLowerCase(),editing=Boolean(a.id)
  if(editing&&!a.editable)return `<div class="modal-title"><span>IMPORTED ACCOUNT</span><h2>${esc(a.name)}</h2></div><div class="readonly-note"><p>该账号使用 ${esc(p.toUpperCase())}，X-port 会继续原样运行它。</p><p class="muted tiny">当前没有专用编辑器；原始 Xray 配置保持不变。</p></div>`
  const enabled=editing?Boolean(a.enabled):true
  return `<div class="modal-title"><span>${editing?'EDIT ACCOUNT':'NEW ACCOUNT'}</span><h2>${editing?'编辑账号':'新增账号'}</h2></div><form id="account-form" class="form-grid"><label class="protocol-first full">${reqLabelV3('协议')}<select name="protocol" id="protocol-select" ${editing?'disabled':''}>${protocols.map(x=>option(x,x.toUpperCase(),p)).join('')}</select>${editing?'<small class="field-hint">创建后不可修改</small>':''}</label>${accountCoreFieldsV5(a)}<div class="full proto-fields" id="proto-fields"></div><details id="account-advanced" class="advanced account-advanced full"><summary><span>高级</span></summary><div id="expert-content" class="advanced-loading muted tiny">${editing?'展开后读取高级配置。':'低频配置，可按需展开。'}</div></details><div class="account-editor-footer full"><div class="editor-policies">${editorPolicySwitchV5('enabled','启用账号',enabled)}${editorPolicySwitchV5('monthlyReset','每月 1 日自动清零',Boolean(a.monthlyReset))}</div><div class="editor-actions"><button type="button" class="btn ghost" id="cancel-form">取消</button><button type="submit" class="btn primary">${editing?'保存并应用':'创建并应用'}</button></div></div></form>`
 }
}

document.addEventListener('DOMContentLoaded',()=>{
 ensureThemeControlV4()
 polishAccountCopyV4()
 arrangeOverviewV5()
 renderServerDetailsV5()
})
