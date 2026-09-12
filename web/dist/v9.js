/* X-port v0.1.8: stable protocol/transport field matrix. */
function editorNetworkV9(value){
 const v=String(value||'').trim().toLowerCase()
 if(v===''||v==='tcp'||v==='raw')return'tcp'
 if(v==='ws'||v==='websocket')return'ws'
 if(v==='xhttp'||v==='splithttp')return'xhttp'
 return v
}
function editorSecurityChoicesV9(protocol,network){
 const p=String(protocol||'').toLowerCase(),n=editorNetworkV9(network)
 if(!['vless','trojan'].includes(p))return['none','tls']
 return['ws','httpupgrade'].includes(n)?['none','tls']:['none','tls','reality']
}
function editorVisionCompatibleV9(protocol,network,security){
 return String(protocol||'').toLowerCase()==='vless'&&editorNetworkV9(network)==='tcp'&&['tls','reality'].includes(String(security||'').toLowerCase())
}
function editorLabelV9(text,control,cls=''){
 return `<label${cls?` class="${cls}"`:''}>${text}${control}</label>`
}
function protocolFieldsV9(p,a={}){
 p=String(p||'vless').toLowerCase()
 const transport=!['socks','http'].includes(p)
 const network=editorNetworkV9(a.network||'tcp')
 const requestedSecurity=String(a.security||(p==='vless'||p==='trojan'?'reality':'none')).toLowerCase()
 let auth=''
 if(p==='vless'){
  const choices=editorSecurityChoicesV9(p,network),security=choices.includes(requestedSecurity)?requestedSecurity:choices[0]
  const canVision=editorVisionCompatibleV9(p,network,security)
  const flow=canVision?(a.flow||(a.id?'':'xtls-rprx-vision')):String(a.flow||'')
  auth=editorLabelV9('UUID',`<input name="credential" value="${esc(a.credential||'')}" placeholder="留空自动生成">`)+editorLabelV9('Flow',`<select name="flow"><option value="" ${flow===''?'selected':''}>无</option><option value="xtls-rprx-vision" ${flow==='xtls-rprx-vision'?'selected':''}>xtls-rprx-vision</option></select>`,'vless-flow')
 }
 if(p==='vmess')auth=editorLabelV9('UUID',`<input name="credential" value="${esc(a.credential||'')}" placeholder="留空自动生成">`)+editorLabelV9('VMess Security',`<select name="clientSecurity">${['auto','aes-128-gcm','chacha20-poly1305','none','zero'].map(x=>option(x,x,a.clientSecurity||'auto')).join('')}</select>`)
 if(p==='trojan')auth=editorLabelV9('Trojan 密码',`<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成">`)
 if(p==='shadowsocks')auth=editorLabelV9('加密方式',`<select name="method">${['chacha20-ietf-poly1305','aes-256-gcm','2022-blake3-aes-256-gcm','2022-blake3-chacha20-poly1305'].map(x=>option(x,x,a.method||'chacha20-ietf-poly1305')).join('')}</select>`)+editorLabelV9('服务器密码',`<input name="serverPassword" value="${esc(a.serverPassword||'')}" placeholder="留空自动生成">`)+editorLabelV9('客户端密码',`<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成">`)
 if(p==='socks'||p==='http'){
  auth=editorLabelV9('用户名',`<input name="username" value="${esc(a.username||'')}">`)+editorLabelV9('密码',`<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成">`)
  return `<div class="protocol-auth-only-grid">${auth}</div>`
 }
 if(!transport)return `<div class="protocol-auth-only-grid">${auth}</div>`
 const secChoices=editorSecurityChoicesV9(p,network),safeSecurity=secChoices.includes(requestedSecurity)?requestedSecurity:secChoices[0]
 const transportControls=editorLabelV9('传输',`<select name="network" id="network-select">${[['tcp','RAW'],['xhttp','XHTTP'],['grpc','gRPC'],['ws','WebSocket'],['httpupgrade','HTTPUpgrade']].map(([v,l])=>option(v,l,network)).join('')}</select>`)+editorLabelV9('传输安全',`<select name="security" id="security-select">${secChoices.map(x=>option(x,x.toUpperCase(),safeSecurity)).join('')}</select>`)
 const extras=editorLabelV9('Host',`<input name="host" value="${esc(a.host||'')}">`,'transport-host')+editorLabelV9('Path',`<input name="path" value="${esc(a.path||'')}" placeholder="/">`,'transport-path')+editorLabelV9('gRPC Service',`<input name="serviceName" value="${esc(a.serviceName||'')}">`,'transport-service')
 return `<div class="protocol-config-grid"><div class="protocol-column protocol-auth-column">${auth}</div><div class="protocol-column protocol-transport-column">${transportControls}</div><div class="protocol-transport-extra">${extras}</div></div>`
}
protocolFieldsV6=protocolFieldsV9
protocolFieldsV3=protocolFieldsV9
securityChoicesV6=editorSecurityChoicesV9
visionCompatibleV6=editorVisionCompatibleV9

function setTransportFieldVisibleV9(box,selector,visible){
 $$(selector,box).forEach(el=>{
  el.style.display=visible?'grid':'none'
  $$('input,select,textarea',el).forEach(control=>control.disabled=!visible)
 })
}
function resetExistingAdvancedAfterTransportChangeV9(details,full){
 if(!full.id||!details)return
 details.open=false
 delete details.dataset.loaded
 details._expertData=null
 const c=$('#expert-content')
 if(c){c.className='advanced-loading muted tiny';c.textContent=''}
}
function bindDirectProtocolAdvancedV9(p,details,full){
 if(full.id||!details)return
 details.addEventListener('toggle',()=>{
  if(!details.open)return
  details.dataset.loaded='1'
  details._expertData=syntheticExpertV6(p,'','none')
  const c=$('#expert-content');if(c)c.innerHTML=renderExpertV3(details._expertData)
 })
}
refreshProtocolFieldsV3=function(full){
 const p=$('#protocol-select')?.value||full.protocol||'vless',box=$('#proto-fields');if(!box)return
 box.innerHTML=protocolFieldsV9(p,full)
 const net=$('#network-select'),sec=$('#security-select'),details=$('#account-advanced')
 if(!net||!sec){bindDirectProtocolAdvancedV9(p,details,full);return}
 let preferredSecurity=String(sec.value||'none').toLowerCase()
 const flow=box.querySelector('.vless-flow'),flowSelect=flow?.querySelector('select[name="flow"]')
 let initial=true
 const renderSecurity=()=>{
  const n=editorNetworkV9(net.value),allowed=editorSecurityChoicesV9(p,n)
  let next=allowed.includes(preferredSecurity)?preferredSecurity:''
  if(!next&&preferredSecurity==='reality'&&allowed.includes('tls'))next='tls'
  if(!next)next=allowed[0]
  sec.innerHTML=allowed.map(x=>option(x,x.toUpperCase(),next)).join('')
  sec.value=next
 }
 const tune=(transportChanged=false)=>{
  renderSecurity()
  const n=editorNetworkV9(net.value),security=String(sec.value||'none').toLowerCase()
  setTransportFieldVisibleV9(box,'.transport-host',['ws','httpupgrade','xhttp'].includes(n))
  setTransportFieldVisibleV9(box,'.transport-path',['ws','httpupgrade','xhttp'].includes(n))
  setTransportFieldVisibleV9(box,'.transport-service',n==='grpc')
  const canVision=editorVisionCompatibleV9(p,n,security)
  if(flow){
   flow.classList.toggle('hidden-field',!canVision)
   flow.style.display=canVision?'grid':'none'
   if(flowSelect)flowSelect.disabled=!canVision
  }
  if(transportChanged&&!initial)resetExistingAdvancedAfterTransportChangeV9(details,full)
  if(!full.id&&details?.open){
   details.dataset.loaded='1'
   details._expertData=syntheticExpertV6(p,n,security)
   const c=$('#expert-content');if(c)c.innerHTML=renderExpertV3(details._expertData)
  }
  initial=false
 }
 net.addEventListener('change',()=>tune(true))
 sec.addEventListener('change',()=>{preferredSecurity=String(sec.value||'none').toLowerCase();tune(true)})
 tune(false)
 if(!full.id&&details)details.addEventListener('toggle',()=>{
  if(details.open){
   const n=editorNetworkV9(net.value),security=String(sec.value||'none').toLowerCase()
   details.dataset.loaded='1';details._expertData=syntheticExpertV6(p,n,security)
   const c=$('#expert-content');if(c)c.innerHTML=renderExpertV3(details._expertData)
  }
 })
}
/* X-port v0.1.9: account editor and overview layout polish. */
function randomUUIDV10(){
 const c=globalThis.crypto
 if(c?.randomUUID)return c.randomUUID()
 const bytes=new Uint8Array(16)
 if(c?.getRandomValues)c.getRandomValues(bytes)
 else for(let i=0;i<bytes.length;i++)bytes[i]=Math.floor(Math.random()*256)
 bytes[6]=(bytes[6]&0x0f)|0x40
 bytes[8]=(bytes[8]&0x3f)|0x80
 const h=[...bytes].map(x=>x.toString(16).padStart(2,'0')).join('')
 return `${h.slice(0,8)}-${h.slice(8,12)}-${h.slice(12,16)}-${h.slice(16,20)}-${h.slice(20)}`
}
function uuidProtocolV10(p){return['vless','vmess'].includes(String(p||'').toLowerCase())}
function renderUUIDFieldV10(p,a={}){
 const box=$('#proto-credential-fields');if(!box)return
 if(!uuidProtocolV10(p)){box.innerHTML='';box.classList.add('hidden-field');return}
 let value=String(a.credential||'')
 if(!value&&!a.id){value=randomUUIDV10();a.credential=value}
 box.classList.remove('hidden-field')
 box.innerHTML=`<label class="uuid-field">${reqLabelV3('UUID')}<div class="uuid-control"><input name="credential" value="${esc(value)}" required autocomplete="off" spellcheck="false"><button type="button" class="uuid-regen" title="重新生成 UUID" aria-label="重新生成 UUID"><svg viewBox="0 0 24 24" width="15" height="15" fill="none" stroke="currentColor" stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M21 12a9 9 0 1 1-2.64-6.36L21 8"/><path d="M21 3v5h-5"/></svg></button></div></label>`
 const input=box.querySelector('input[name="credential"]')
 box.querySelector('.uuid-regen')?.addEventListener('click',()=>{const v=randomUUIDV10();input.value=v;a.credential=v})
}
function protocolFieldsV10(p,a={}){
 p=String(p||'vless').toLowerCase()
 const transport=!['socks','http'].includes(p)
 const network=editorNetworkV9(a.network||'tcp')
 const requestedSecurity=String(a.security||(p==='vless'||p==='trojan'?'reality':'none')).toLowerCase()
 let auth=''
 if(p==='vless'){
  const choices=editorSecurityChoicesV9(p,network),security=choices.includes(requestedSecurity)?requestedSecurity:choices[0]
  const canVision=editorVisionCompatibleV9(p,network,security)
  const flow=canVision?(a.flow||(a.id?'':'xtls-rprx-vision')):String(a.flow||'')
  auth=editorLabelV9('Flow',`<select name="flow"><option value="" ${flow===''?'selected':''}>无</option><option value="xtls-rprx-vision" ${flow==='xtls-rprx-vision'?'selected':''}>xtls-rprx-vision</option></select>`,'vless-flow')
 }
 if(p==='vmess')auth=editorLabelV9('VMess Security',`<select name="clientSecurity">${['auto','aes-128-gcm','chacha20-poly1305','none','zero'].map(x=>option(x,x,a.clientSecurity||'auto')).join('')}</select>`)
 if(p==='trojan')auth=editorLabelV9('Trojan 密码',`<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成">`)
 if(p==='shadowsocks')auth=editorLabelV9('加密方式',`<select name="method">${['chacha20-ietf-poly1305','aes-256-gcm','2022-blake3-aes-256-gcm','2022-blake3-chacha20-poly1305'].map(x=>option(x,x,a.method||'chacha20-ietf-poly1305')).join('')}</select>`)+editorLabelV9('服务器密码',`<input name="serverPassword" value="${esc(a.serverPassword||'')}" placeholder="留空自动生成">`)+editorLabelV9('客户端密码',`<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成">`)
 if(p==='socks'||p==='http'){
  auth=editorLabelV9('用户名',`<input name="username" value="${esc(a.username||'')}">`)+editorLabelV9('密码',`<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成">`)
  return `<div class="protocol-auth-only-grid">${auth}</div>`
 }
 if(!transport)return `<div class="protocol-auth-only-grid">${auth}</div>`
 const secChoices=editorSecurityChoicesV9(p,network),safeSecurity=secChoices.includes(requestedSecurity)?requestedSecurity:secChoices[0]
 const transportControls=editorLabelV9('传输',`<select name="network" id="network-select">${[['tcp','RAW'],['xhttp','XHTTP'],['grpc','gRPC'],['ws','WebSocket'],['httpupgrade','HTTPUpgrade']].map(([v,l])=>option(v,l,network)).join('')}</select>`)+editorLabelV9('传输安全',`<select name="security" id="security-select">${secChoices.map(x=>option(x,x.toUpperCase(),safeSecurity)).join('')}</select>`)
 const extras=editorLabelV9('Host',`<input name="host" value="${esc(a.host||'')}">`,'transport-host')+editorLabelV9('Path',`<input name="path" value="${esc(a.path||'')}" placeholder="/">`,'transport-path')+editorLabelV9('gRPC Service',`<input name="serviceName" value="${esc(a.serviceName||'')}">`,'transport-service')
 return `<div class="protocol-config-grid">${transportControls}${auth}${extras}</div>`
}
function refreshProtocolFieldsV10(full){
 const p=$('#protocol-select')?.value||full.protocol||'vless',box=$('#proto-fields');if(!box)return
 renderUUIDFieldV10(p,full)
 box.innerHTML=protocolFieldsV10(p,full)
 const net=$('#network-select'),sec=$('#security-select'),details=$('#account-advanced')
 if(!net||!sec){bindDirectProtocolAdvancedV9(p,details,full);return}
 let preferredSecurity=String(sec.value||'none').toLowerCase()
 const flow=box.querySelector('.vless-flow'),flowSelect=flow?.querySelector('select[name="flow"]')
 let initial=true
 const renderSecurity=()=>{
  const n=editorNetworkV9(net.value),allowed=editorSecurityChoicesV9(p,n)
  let next=allowed.includes(preferredSecurity)?preferredSecurity:''
  if(!next&&preferredSecurity==='reality'&&allowed.includes('tls'))next='tls'
  if(!next)next=allowed[0]
  sec.innerHTML=allowed.map(x=>option(x,x.toUpperCase(),next)).join('')
  sec.value=next
 }
 const tune=(transportChanged=false)=>{
  renderSecurity()
  const n=editorNetworkV9(net.value),security=String(sec.value||'none').toLowerCase()
  setTransportFieldVisibleV9(box,'.transport-host',['ws','httpupgrade','xhttp'].includes(n))
  setTransportFieldVisibleV9(box,'.transport-path',['ws','httpupgrade','xhttp'].includes(n))
  setTransportFieldVisibleV9(box,'.transport-service',n==='grpc')
  const canVision=editorVisionCompatibleV9(p,n,security)
  if(flow){
   flow.classList.toggle('hidden-field',!canVision)
   flow.style.display=canVision?'grid':'none'
   if(flowSelect)flowSelect.disabled=!canVision
  }
  if(transportChanged&&!initial)resetExistingAdvancedAfterTransportChangeV9(details,full)
  if(!full.id&&details?.open){
   details.dataset.loaded='1';details._expertData=syntheticExpertV6(p,n,security)
   const c=$('#expert-content');if(c)c.innerHTML=renderExpertV3(details._expertData)
  }
  initial=false
 }
 net.addEventListener('change',()=>tune(true))
 sec.addEventListener('change',()=>{preferredSecurity=String(sec.value||'none').toLowerCase();tune(true)})
 tune(false)
 if(!full.id&&details)details.addEventListener('toggle',()=>{
  if(details.open){
   const n=editorNetworkV9(net.value),security=String(sec.value||'none').toLowerCase()
   details.dataset.loaded='1';details._expertData=syntheticExpertV6(p,n,security)
   const c=$('#expert-content');if(c)c.innerHTML=renderExpertV3(details._expertData)
  }
 })
}
function accountFormHTMLV10(a={}){
 const p=String(a.protocol||'vless').toLowerCase(),editing=Boolean(a.id)
 if(editing&&!a.editable)return `<div class="modal-title"><span>IMPORTED ACCOUNT</span><h2>${esc(a.name)}</h2></div><div class="readonly-note"><p>该账号包含当前内置编辑器不支持的配置，X-port 会保持原始 Xray 配置不变。</p></div>`
 const expiry=a.expiryTime?new Date(Number(a.expiryTime)-new Date().getTimezoneOffset()*60000).toISOString().slice(0,16):'',qgb=a.quotaBytes?(Number(a.quotaBytes)/1073741824).toFixed(2):'',enabled=editing?Boolean(a.enabled):true
 const range=a._portMin&&a._portMax?`${a._portMin}–${a._portMax}`:`${xportPortRangeV6.min}–${xportPortRangeV6.max}`
 return `<form id="account-form" class="account-editor-form"><div class="account-editor-scroll"><div class="modal-title"><span>${editing?'EDIT ACCOUNT':'NEW ACCOUNT'}</span><h2>${editing?'编辑账号':'新增账号'}</h2></div><div class="form-grid account-editor-grid"><label class="protocol-first full">${reqLabelV3('协议')}<select name="protocol" id="protocol-select" ${editing?'disabled':''}>${protocols.map(x=>option(x,x.toUpperCase(),p)).join('')}</select>${editing?'<small class="field-hint">创建后不可修改</small>':''}</label><label>${reqLabelV3('账号')}<input name="name" value="${esc(a.name||'')}" required maxlength="80"></label><label><span class="field-label-line">${reqLabelV3('端口')} <span class="field-inline-note">（端口范围 ${esc(range)}）</span></span><input name="port" type="number" min="1" max="65535" value="${a.port||''}" required placeholder="请输入端口"></label><div class="full proto-credential-fields" id="proto-credential-fields"></div><label>流量上限 / GB<input name="quota" type="number" min="0" step="0.1" value="${qgb}" placeholder="0 = 不限"></label><label>到期时间<input name="expiry" type="datetime-local" value="${expiry}"></label><div class="full proto-fields" id="proto-fields"></div><details id="account-advanced" class="advanced account-advanced full"><summary><span>高级</span></summary><div id="expert-content" class="advanced-loading muted tiny"></div></details></div></div><div class="account-editor-footer"><div class="editor-policies">${editorPolicyV5('enabled','启用账号',enabled)}${editorPolicyV5('monthlyReset','每月 1 日流量自动重置',Boolean(a.monthlyReset))}</div><div class="editor-actions"><button type="button" class="btn ghost" id="cancel-form">取消</button><button type="submit" class="btn primary">${editing?'保存并应用':'创建并应用'}</button></div></div></form>`
}
function renderServerDetailsV10(){
 const panel=$('.server-panel'),sys=state.overview?.system||{};if(!panel)return
 const core=panel.querySelector('.server-core');if(!core)return
 let top=panel.querySelector('.server-top-layout')
 if(!top){
  top=document.createElement('div');top.className='server-top-layout'
  core.insertAdjacentElement('beforebegin',top);top.appendChild(core)
 }
 let box=$('#server-details')
 if(!box){box=document.createElement('div');box.id='server-details'}
 box.className='server-details server-facts'
 if(box.parentElement!==top)top.appendChild(box)
 const fields=[
  ['发行版本',sys.distribution||sys.os||'—'],
  ['内核版本',sys.kernelVersion||'—'],
  ['系统类型',sys.systemType||sys.os||'—'],
  ['主机地址',sys.hostAddress||'—'],
  ['启动时间',formatBootTimeV5(sys.bootTime)]
 ]
 box.innerHTML=fields.map(([k,v])=>`<div class="server-fact"><small>${esc(k)}</small><b title="${esc(v)}">${esc(v)}</b></div>`).join('')
}
protocolFieldsV9=protocolFieldsV10
protocolFieldsV6=protocolFieldsV10
protocolFieldsV3=protocolFieldsV10
refreshProtocolFieldsV3=refreshProtocolFieldsV10
accountFormHTMLV6=accountFormHTMLV10
accountFormHTMLV3=accountFormHTMLV10
renderServerDetailsV5=renderServerDetailsV10

document.addEventListener('DOMContentLoaded',()=>{
 $('#online-note')?.remove()
 $$('.health-panel .health-row').forEach(row=>{if(row.querySelector('span')?.textContent?.trim()==='配置模型')row.remove()})
 renderServerDetailsV10()
})
