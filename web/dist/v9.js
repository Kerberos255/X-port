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
