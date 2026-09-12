let xportOnlineConnections={}
let xportOnlineTimer=null

function isTransientNetworkErrorV3(err){
 return !err?.status && /NetworkError|Failed to fetch|Load failed|network request/i.test(String(err?.message||err||''))
}
function sleepV3(ms){return new Promise(r=>setTimeout(r,ms))}
async function waitForPanelV3(timeoutMs=12000){
 const deadline=Date.now()+timeoutMs
 while(Date.now()<deadline){
  try{await api('/api/health');return true}catch{}
  await sleepV3(650)
 }
 return false
}
async function mutationWithRecoveryV3(run,verify){
 try{return {value:await run(),recovered:false}}
 catch(err){
  if(!isTransientNetworkErrorV3(err))throw err
  toast('Xray 正在应用配置，等待连接恢复…')
  if(!await waitForPanelV3())throw err
  try{if(await verify())return {value:null,recovered:true}}catch{}
  throw err
 }
}

async function loadOnlineConnectionsV3(render=true){
 if(state.page!=='accounts')return
 try{
  const d=await api('/api/accounts/online')
  xportOnlineConnections=d.connections||{}
  if(render)renderAccountsV3()
 }catch{}
}
function onlinePillV3(a){
 const n=Number(xportOnlineConnections[String(a.id)]??0)
 return `<span class="online-pill ${n>0?'on':''}" title="当前 ESTABLISHED TCP 连接数；XHTTP 单个客户端可能同时建立多条连接"><i></i>${n>0?`在线 ${n}`:'离线'}</span>`
}
function renderAccountsV3(){
 const arr=state.accounts,list=$('#accounts-list');if(!list)return
 const up=arr.reduce((n,a)=>n+Number(a.upBytes||0),0),down=arr.reduce((n,a)=>n+Number(a.downBytes||0),0),all=arr.reduce((n,a)=>n+Number(a.allTimeBytes||0),0)
 $('#sum-total').textContent=arr.length;$('#sum-active').textContent=arr.filter(a=>a.enabled).length
 $('#sum-traffic').textContent=`↑ ${fmtBytes(up)} · ↓ ${fmtBytes(down)}`;$('#sum-monthly').textContent=fmtBytes(all)
 if(!arr.length){list.innerHTML='<div class="empty"><b>还没有账号</b><span>新增账号，或通过迁移脚本导入 X-Panel / 3x-ui。</span></div>';return}
 list.innerHTML=arr.map(a=>{
  const used=Number(a.upBytes||0)+Number(a.downBytes||0),quota=Number(a.quotaBytes||0),pct=quota?Math.min(100,Math.round(used/quota*100)):0,editable=Boolean(a.editable)
  const toggleTitle=a.enabled?'停用账号':a.disabledReason==='quota'?'额度已用完':a.disabledReason==='expiry'?'账号已到期':'启用账号'
  return `<article class="account-row" data-id="${a.id}"><div class="account-name"><i class="state ${a.enabled?'on':''}"></i><div><b>${esc(a.name)}</b><small>${esc(reasonText(a))} · ID ${a.id}</small></div></div><div class="online-cell">${onlinePillV3(a)}</div><div class="enable-cell"><button class="account-toggle ${a.enabled?'on':''}" data-act="toggle" role="switch" aria-checked="${a.enabled?'true':'false'}" title="${esc(toggleTitle)}"><i></i></button></div><div class="port-cell"><span class="tag">${a.port}</span></div><div class="protocol-cell hide-mid"><span class="tag">${esc(String(a.protocol||'').toUpperCase())}${a.security?` · ${esc(String(a.security).toUpperCase())}`:''}</span></div><div class="usage-cell hide-mid"><small>本期 ${quota?`/ ${fmtBytes(quota)}`:'· 不限'}</small><b>${fmtBytes(used)}</b><div class="mini-bar"><i style="width:${pct}%"></i></div></div><div class="hide-mid"><small class="muted tiny">${a.expiryTime?new Date(a.expiryTime).toLocaleDateString():'永久'}</small></div><div class="row-actions"><button class="icon-btn" data-act="edit" title="编辑">✎</button><button class="icon-btn" data-act="clone" title="克隆" ${editable?'':'disabled'}>⧉</button><button class="icon-btn" data-act="share" title="分享" ${editable?'':'disabled'}>⌁</button><button class="icon-btn" data-act="reset" title="重置本期流量">↺</button><button class="icon-btn danger-icon" data-act="delete" title="删除">×</button></div></article>`
 }).join('')
 $$('.account-row').forEach(row=>row.onclick=e=>{
  const b=e.target.closest('button[data-act]');if(!b||b.disabled)return
  const id=Number(row.dataset.id),a=state.accounts.find(x=>x.id===id)
  ;({edit:()=>openAccountModalV3(a),clone:()=>openCloneModalV3(id),delete:()=>deleteAccountV3(id),share:()=>openShareModal(id),reset:()=>resetTrafficV3(id),toggle:()=>toggleAccountV3(id,!a.enabled,b)})[b.dataset.act]?.()
 })
}

function reqLabelV3(text){return `${text}<span class="required-mark">*</span>`}
function commonAccountFieldsV3(a){
 const expiry=a.expiryTime?new Date(Number(a.expiryTime)-new Date().getTimezoneOffset()*60000).toISOString().slice(0,16):'',qgb=a.quotaBytes?(Number(a.quotaBytes)/1073741824).toFixed(2):''
 return `<label>${reqLabelV3('账号')}<input name="name" value="${esc(a.name||'')}" required maxlength="80"></label><label>${reqLabelV3('端口')}<input name="port" type="number" min="1" max="65535" value="${a.port||''}" required placeholder="1 - 65535"></label><label>流量上限 / GB<input name="quota" type="number" min="0" step="0.1" value="${qgb}" placeholder="0 = 不限"></label><label>到期时间<input name="expiry" type="datetime-local" value="${expiry}"></label><label class="checkline"><input name="enabled" type="checkbox" ${a.id?(a.enabled?'checked':''):'checked'}>启用账号</label><label class="checkline"><input name="monthlyReset" type="checkbox" ${a.monthlyReset?'checked':''}>每月 1 日自动清零本期流量</label><p class="muted tiny full">月初清零只重置本期上下行，累计总流量保留；额度耗尽导致的停用会在清零后自动恢复。</p>`
}
function createAdvancedV3(p,a,network,security){
 let body='<p class="muted tiny expert-note">低频参数放在这里。未填写的项目使用 Xray 默认值；创建后还能继续编辑更完整的高级设置。</p>'
 if(security==='reality'){
  body+=`<div class="expert-section"><div class="expert-section-title">REALITY <small>服务端</small></div><label>Target<input name="dest" value="${esc(a.dest||'')}" placeholder="example.com:443"></label><label>Server Name / SNI<input name="serverName" value="${esc(a.serverName||'')}" placeholder="example.com"></label><label>Private Key<input name="privateKey" value="${esc(a.privateKey||'')}" placeholder="留空自动生成"></label><label>Short ID<input name="shortId" value="${esc(a.shortId||'')}" placeholder="留空自动生成"></label><p class="muted tiny full expert-note">Public Key 会由 Private Key 自动派生，创建后在这里显示。</p></div>`
 }
 if(network==='xhttp')body+=`<div class="expert-section"><div class="expert-section-title">XHTTP <small>创建后可配置 mode / padding / stream 参数</small></div><p class="muted tiny full expert-note">新账号默认交给 Xray 使用 auto；packet-up、stream-up 等低频项可创建后再调整。</p></div>`
 return body
}
function protocolFieldsV3(p,a){
 const transport=!['socks','http'].includes(p),network=a.network||'tcp',security=a.security||(p==='vless'||p==='trojan'?'reality':'none');let cred=''
 if(p==='vless')cred=`<label>UUID<input name="credential" value="${esc(a.credential||'')}" placeholder="留空自动生成"></label><label>Flow<input name="flow" value="${esc(a.flow||'xtls-rprx-vision')}"></label>`
 if(p==='vmess')cred=`<label>UUID<input name="credential" value="${esc(a.credential||'')}" placeholder="留空自动生成"></label><label>VMess Security<select name="clientSecurity">${['auto','aes-128-gcm','chacha20-poly1305','none','zero'].map(x=>option(x,x,a.clientSecurity||'auto')).join('')}</select></label>`
 if(p==='trojan')cred=`<label>Trojan 密码<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成"></label>`
 if(p==='shadowsocks')cred=`<label>加密方式<select name="method">${['chacha20-ietf-poly1305','aes-256-gcm','2022-blake3-aes-256-gcm','2022-blake3-chacha20-poly1305'].map(x=>option(x,x,a.method||'chacha20-ietf-poly1305')).join('')}</select></label><label>服务器密码<input name="serverPassword" value="${esc(a.serverPassword||'')}" placeholder="留空自动生成"></label><label>客户端密码<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成"></label>`
 if(p==='socks'||p==='http')cred=`<label>用户名<input name="username" value="${esc(a.username||'')}"></label><label>密码<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成"></label>`
 if(!transport)return `<div class="advanced-grid">${cred}</div>`
 const allowedSecurity=(p==='vless'||p==='trojan')?['none','tls','reality']:['none','tls']
 return `<div class="advanced-grid">${cred}<label>传输<select name="network" id="network-select">${[['tcp','RAW'],['xhttp','XHTTP'],['grpc','gRPC'],['ws','WebSocket'],['httpupgrade','HTTPUpgrade']].map(([v,l])=>option(v,l,network)).join('')}</select></label><label>传输安全<select name="security" id="security-select">${allowedSecurity.map(x=>option(x,x.toUpperCase(),security)).join('')}</select></label><label class="transport-host">Host<input name="host" value="${esc(a.host||'')}"></label><label class="transport-path">Path<input name="path" value="${esc(a.path||'')}" placeholder="/"></label><label class="transport-service">gRPC Service<input name="serviceName" value="${esc(a.serviceName||'')}"></label></div>`
}
function accountFormHTMLV3(a={}){
 const p=String(a.protocol||'vless').toLowerCase(),editing=Boolean(a.id)
 if(editing&&!a.editable)return `<div class="modal-title"><span>IMPORTED ACCOUNT</span><h2>${esc(a.name)}</h2></div><div class="readonly-note"><p>该账号使用 ${esc(p.toUpperCase())}，X-port 会继续原样运行它。</p><p class="muted tiny">当前没有专用编辑器；原始 Xray 配置保持不变。</p></div>`
 return `<div class="modal-title"><span>${editing?'EDIT ACCOUNT':'NEW ACCOUNT'}</span><h2>${editing?'编辑账号':'新增账号'}</h2></div><form id="account-form" class="form-grid"><label class="protocol-first full">${reqLabelV3('协议')}<select name="protocol" id="protocol-select" ${editing?'disabled':''}>${protocols.map(x=>option(x,x.toUpperCase(),p)).join('')}</select>${editing?'<small class="field-hint">创建后不可修改</small>':''}</label>${commonAccountFieldsV3(a)}<div class="full proto-fields" id="proto-fields"></div><details id="account-advanced" class="advanced account-advanced full"><summary><span>高级</span></summary><div id="expert-content" class="advanced-loading muted tiny">${editing?'展开后读取高级配置。':'低频配置，可按需展开。'}</div></details><div class="form-actions full"><button type="button" class="btn ghost" id="cancel-form">取消</button><button type="submit" class="btn primary">${editing?'保存并应用':'创建并应用'}</button></div></form>`
}
function refreshProtocolFieldsV3(full){
 const p=$('#protocol-select')?.value||full.protocol||'vless',box=$('#proto-fields');if(!box)return
 box.innerHTML=protocolFieldsV3(p,full);const net=$('#network-select'),sec=$('#security-select')
 const tune=()=>{const n=net?.value||'tcp';$$('.transport-host',box).forEach(e=>e.style.display=['ws','httpupgrade','xhttp'].includes(n)?'grid':'none');$$('.transport-path',box).forEach(e=>e.style.display=['ws','httpupgrade','xhttp'].includes(n)?'grid':'none');$$('.transport-service',box).forEach(e=>e.style.display=n==='grpc'?'grid':'none');if(!full.id){const c=$('#expert-content');if(c)c.innerHTML=createAdvancedV3(p,full,n,sec?.value||'none')}}
 net?.addEventListener('change',tune);sec?.addEventListener('change',tune);tune()
}
function commaListV3(a){return Array.isArray(a)?a.join(', '):''}
function expertSectionV3(title,sub,body){return `<div class="expert-section"><div class="expert-section-title">${title}<small>${sub||''}</small></div>${body}</div>`}
function renderExpertV3(d){
 let html=''
 let common=`<label class="expert-check full"><input id="expert-proxy" type="checkbox" ${d.acceptProxyProtocol?'checked':''}>接收 PROXY Protocol<small class="field-hint">仅在可信 L4 反向代理后开启；不要直接暴露给不受信任客户端。</small></label>`
 if(d.supportsFallbacks)common+=`<label class="full">Fallbacks JSON<textarea id="expert-fallbacks" spellcheck="false">${esc(d.fallbacksJson||'[]')}</textarea><small class="field-hint">Xray 当前要求 RAW/TCP + TLS；通常还需 ALPN http/1.1。</small></label>`
 if(d.supportsHttpHeader)common+=`<label class="full">HTTP 伪装 Header JSON<textarea id="expert-http-header" spellcheck="false" placeholder='{"type":"http","request":{},"response":{}}'>${esc(d.httpHeaderJson||'')}</textarea></label>`
 html+=expertSectionV3('通用','Xray 低频项',common)
 if(d.supportsReality){
  html+=expertSectionV3('REALITY','当前 Xray 服务端字段',`<label>Target<input id="expert-reality-target" value="${esc(d.realityTarget||'')}"></label><label>Server Names<input id="expert-reality-names" value="${esc(commaListV3(d.realityServerNames))}" placeholder="example.com"></label><label class="full">Private Key<input id="expert-reality-private" value="${esc(d.realityPrivateKey||'')}"></label><label class="full">Public Key（只读）<input id="expert-reality-public" value="${esc(d.realityPublicKey||'')}" readonly></label><label>Short IDs<input id="expert-reality-shortids" value="${esc(commaListV3(d.realityShortIds))}"></label><label>xver<input id="expert-reality-xver" type="number" min="0" max="2" value="${Number(d.realityXver||0)}"></label><label>Min Client Ver<input id="expert-reality-minver" value="${esc(d.realityMinClientVer||'')}"></label><label>Max Client Ver<input id="expert-reality-maxver" value="${esc(d.realityMaxClientVer||'')}"></label><label>Max Time Diff / ms<input id="expert-reality-timediff" type="number" min="0" value="${Number(d.realityMaxTimeDiff||0)}"></label><label>ML-DSA-65 Seed<input id="expert-reality-mldsa" value="${esc(d.realityMldsa65Seed||'')}"></label><label class="expert-check full"><input id="expert-reality-show" type="checkbox" ${d.realityShow?'checked':''}>Show debug</label>`)
 }
 if(d.supportsTls){
  html+=expertSectionV3('TLS','服务端安全参数',`<label>ALPN<input id="expert-tls-alpn" value="${esc(commaListV3(d.tlsAlpn))}" placeholder="h2, http/1.1"></label><label>Min Version<input id="expert-tls-min" value="${esc(d.tlsMinVersion||'')}"></label><label>Max Version<input id="expert-tls-max" value="${esc(d.tlsMaxVersion||'')}"></label><label>Cipher Suites<input id="expert-tls-ciphers" value="${esc(d.tlsCipherSuites||'')}"></label><label class="expert-check full"><input id="expert-tls-reject" type="checkbox" ${d.tlsRejectUnknownSni?'checked':''}>Reject Unknown SNI</label><label class="full">Certificates JSON<textarea id="expert-tls-certs" spellcheck="false">${esc(d.tlsCertificatesJson||'[]')}</textarea></label>`)
 }
 if(d.supportsXhttp){
  html+=expertSectionV3('XHTTP','当前 Xray transport 参数',`<label>Mode<select id="expert-xhttp-mode">${['','auto','packet-up','stream-up','stream-one'].map(x=>option(x,x||'默认 / auto',d.xhttpMode||'')).join('')}</select></label><label>xPaddingBytes<input id="expert-xhttp-padding" value="${esc(d.xhttpXPaddingBytes||'')}"></label><label>scMaxEachPostBytes<input id="expert-xhttp-postbytes" value="${esc(d.xhttpScMaxEachPostBytes||'')}"></label><label>scMaxBufferedPosts<input id="expert-xhttp-buffered" type="number" min="0" value="${Number(d.xhttpScMaxBufferedPosts||0)}"></label><label>scStreamUpServerSecs<input id="expert-xhttp-streamsecs" value="${esc(d.xhttpScStreamUpServerSecs||'')}"></label><label>uplinkHTTPMethod<input id="expert-xhttp-method" value="${esc(d.xhttpUplinkHttpMethod||'')}"></label><label class="expert-check full"><input id="expert-xhttp-nosse" type="checkbox" ${d.xhttpNoSseHeader?'checked':''}>noSSEHeader</label>`)
 }
 html+=`<p class="muted tiny expert-note">未展示的迁移字段不会被清空；保存只修改本页明确列出的高级项。</p><div class="expert-actions"><button id="save-expert" type="button" class="btn ghost">保存高级设置</button></div>`
 return `<div class="expert-body">${html}</div>`
}
function splitListV3(v){return String(v||'').split(',').map(s=>s.trim()).filter(Boolean)}
function expertBodyFromFormV3(d){
 return {protocol:d.protocol,method:d.method,security:d.security,fallbacksJson:$('#expert-fallbacks')?.value||'',acceptProxyProtocol:$('#expert-proxy')?.checked||false,httpHeaderJson:$('#expert-http-header')?.value||'',realityTarget:$('#expert-reality-target')?.value||'',realityServerNames:splitListV3($('#expert-reality-names')?.value),realityPrivateKey:$('#expert-reality-private')?.value||'',realityPublicKey:d.realityPublicKey||'',realityShortIds:splitListV3($('#expert-reality-shortids')?.value),realityXver:Number($('#expert-reality-xver')?.value||0),realityMinClientVer:$('#expert-reality-minver')?.value||'',realityMaxClientVer:$('#expert-reality-maxver')?.value||'',realityMaxTimeDiff:Number($('#expert-reality-timediff')?.value||0),realityShow:$('#expert-reality-show')?.checked||false,realityMldsa65Seed:$('#expert-reality-mldsa')?.value||'',tlsAlpn:splitListV3($('#expert-tls-alpn')?.value),tlsMinVersion:$('#expert-tls-min')?.value||'',tlsMaxVersion:$('#expert-tls-max')?.value||'',tlsCipherSuites:$('#expert-tls-ciphers')?.value||'',tlsRejectUnknownSni:$('#expert-tls-reject')?.checked||false,tlsCertificatesJson:$('#expert-tls-certs')?.value||'',xhttpMode:$('#expert-xhttp-mode')?.value||'',xhttpXPaddingBytes:$('#expert-xhttp-padding')?.value||'',xhttpNoSseHeader:$('#expert-xhttp-nosse')?.checked||false,xhttpScMaxBufferedPosts:Number($('#expert-xhttp-buffered')?.value||0),xhttpScMaxEachPostBytes:$('#expert-xhttp-postbytes')?.value||'',xhttpScStreamUpServerSecs:$('#expert-xhttp-streamsecs')?.value||'',xhttpUplinkHttpMethod:$('#expert-xhttp-method')?.value||''}
}
async function loadExpertV3(id,details){
 const c=$('#expert-content');if(!c)return
 c.textContent='正在读取高级配置…'
 try{
  const d=await api(`/api/accounts/${id}/expert`);c.innerHTML=renderExpertV3(d)
  $('#save-expert').onclick=async()=>{
   const b=$('#save-expert'),body=expertBodyFromFormV3(d);b.disabled=true;b.textContent='应用中…'
   try{
    await mutationWithRecoveryV3(()=>api(`/api/accounts/${id}/expert`,{method:'PUT',body:JSON.stringify(body)}),async()=>{const cur=await api(`/api/accounts/${id}/expert`);return Boolean(cur)})
    toast('高级设置已应用');await loadAccounts();await loadOverview();await loadOnlineConnectionsV3(false);await loadExpertV3(id,details)
   }catch(e){toast(e.message,true);b.disabled=false;b.textContent='保存高级设置'}
  }
 }catch(e){c.innerHTML=`<p class="error tiny">${esc(e.message)}</p>`}
}
async function openAccountModalV3(a=null){
 let full=a||{};if(a?.id){try{full=await api(`/api/accounts/${a.id}`)}catch(e){toast(e.message,true);return}}
 openModal(accountFormHTMLV3(full));if(full.id&&!full.editable)return
 $('#cancel-form').onclick=closeModal;refreshProtocolFieldsV3(full)
 const details=$('#account-advanced');if(full.id)details?.addEventListener('toggle',()=>{if(details.open&&!details.dataset.loaded){details.dataset.loaded='1';void loadExpertV3(full.id,details)}})
 $('#protocol-select')?.addEventListener('change',()=>{const p=$('#protocol-select').value;full={...full,protocol:p,credential:'',username:'',password:'',serverPassword:'',privateKey:'',shortId:'',serverName:'',dest:'',host:'',path:'',serviceName:'',network:'tcp',security:(p==='vless'||p==='trojan')?'reality':'none'};refreshProtocolFieldsV3(full)})
 $('#account-form').onsubmit=async e=>{
  e.preventDefault();const f=e.currentTarget,p=full.id?full.protocol:formValue(f,'protocol'),port=Number(formValue(f,'port'))
  if(!port){toast('端口不能为空',true);return}
  const body={name:formValue(f,'name'),enabled:new FormData(f).get('enabled')==='on',monthlyReset:new FormData(f).get('monthlyReset')==='on',port,protocol:p,credential:formValue(f,'credential'),username:formValue(f,'username'),password:formValue(f,'password'),serverPassword:formValue(f,'serverPassword'),clientSecurity:formValue(f,'clientSecurity'),method:formValue(f,'method'),flow:formValue(f,'flow'),network:formValue(f,'network'),security:formValue(f,'security'),serverName:formValue(f,'serverName'),dest:formValue(f,'dest'),privateKey:formValue(f,'privateKey'),shortId:formValue(f,'shortId'),host:formValue(f,'host'),path:formValue(f,'path'),serviceName:formValue(f,'serviceName'),quotaBytes:Math.round(Number(formValue(f,'quota')||0)*1073741824),expiryTime:formValue(f,'expiry')?new Date(formValue(f,'expiry')).getTime():0}
  const b=f.querySelector('button[type=submit]');b.disabled=true;b.textContent='应用中…';const before=new Set(state.accounts.map(x=>Number(x.id)))
  try{
   await mutationWithRecoveryV3(()=>full.id?api(`/api/accounts/${full.id}`,{method:'PUT',body:JSON.stringify(body)}):api('/api/accounts',{method:'POST',body:JSON.stringify(body)}),async()=>{if(full.id){const cur=await api(`/api/accounts/${full.id}`);return cur.name===body.name&&Number(cur.port)===body.port&&String(cur.protocol).toLowerCase()===String(body.protocol).toLowerCase()}const d=await api('/api/accounts');return (d.accounts||[]).some(x=>!before.has(Number(x.id))&&x.name===body.name&&Number(x.port)===body.port)})
   closeModal();await loadAccounts();await loadOverview();await loadOnlineConnectionsV3(false);toast(full.id?'账号已更新':'账号已创建')
  }catch(x){toast(x.message,true);b.disabled=false;b.textContent=full.id?'保存并应用':'创建并应用'}
 }
}

async function toggleAccountV3(id,enabled,button){
 button.disabled=true
 try{await mutationWithRecoveryV3(()=>api(`/api/accounts/${id}/enabled`,{method:'PATCH',body:JSON.stringify({enabled})}),async()=>{const cur=await api(`/api/accounts/${id}`);return Boolean(cur.enabled)===enabled});await loadAccounts();await loadOverview();await loadOnlineConnectionsV3(false);toast(enabled?'账号已启用':'账号已停用')}
 catch(e){toast(e.message,true);button.disabled=false}
}
function openCloneModalV3(id){
 const a=state.accounts.find(x=>x.id===id);openModal(`<div class="modal-title"><span>CLONE ACCOUNT</span><h2>克隆 ${esc(a?.name||'账号')}</h2></div><form id="clone-form" class="form-grid"><label>${reqLabelV3('新账号')}<input name="name" value="${esc((a?.name||'account')+' copy')}" required></label><label>${reqLabelV3('新端口')}<input name="port" type="number" min="1" max="65535" required></label><p class="muted tiny full">协议与传输参数会复制；UUID/密码自动重新生成，本期与累计流量清零。</p><div class="form-actions full"><button type="button" class="btn ghost" id="clone-cancel">取消</button><button class="btn primary" type="submit">克隆并应用</button></div></form>`)
 $('#clone-cancel').onclick=closeModal;$('#clone-form').onsubmit=async e=>{e.preventDefault();const f=new FormData(e.currentTarget),name=String(f.get('name')||''),port=Number(f.get('port')||0),before=new Set(state.accounts.map(x=>Number(x.id)));try{await mutationWithRecoveryV3(()=>api(`/api/accounts/${id}/clone`,{method:'POST',body:JSON.stringify({name,port})}),async()=>{const d=await api('/api/accounts');return(d.accounts||[]).some(x=>!before.has(Number(x.id))&&x.name===name&&Number(x.port)===port)});closeModal();await loadAccounts();await loadOverview();await loadOnlineConnectionsV3(false);toast('账号已克隆')}catch(x){toast(x.message,true)}}
}
async function resetTrafficV3(id){
 const a=state.accounts.find(x=>x.id===id);if(!confirm(`清零「${a?.name||id}」的本期上下行流量？\n累计总流量不会清零。`))return
 try{await mutationWithRecoveryV3(()=>api(`/api/accounts/${id}/traffic/reset`,{method:'POST',body:'{}'}),async()=>{const d=await api('/api/accounts');const cur=(d.accounts||[]).find(x=>Number(x.id)===id);return cur&&Number(cur.upBytes||0)===0&&Number(cur.downBytes||0)===0});await loadAccounts();await loadOverview();toast('本期流量已清零')}catch(e){toast(e.message,true)}
}
async function deleteAccountV3(id){
 const a=state.accounts.find(x=>x.id===id);if(!confirm(`删除账号「${a?.name||id}」？\n对应 inbound 会从 Xray 配置移除。`))return
 try{await mutationWithRecoveryV3(()=>api(`/api/accounts/${id}`,{method:'DELETE'}),async()=>{const d=await api('/api/accounts');return!(d.accounts||[]).some(x=>Number(x.id)===id)});await loadAccounts();await loadOverview();await loadOnlineConnectionsV3(false);toast('账号已删除')}catch(e){toast(e.message,true)}
}

async function loadSettingsV3(){
 try{const d=await api('/api/settings'),f=$('#settings-form');for(const [k,v0] of Object.entries(d)){const el=f.elements.namedItem(k);if(!el)continue;let v=v0??'';if(k==='panelListen'&&/^:\d+$/.test(String(v)))v=String(v).slice(1);el.value=v}f.elements.currentPassword.value='';f.elements.newPassword.value='';const input=f.elements.namedItem('panelListen');if(input&&!input.parentElement.querySelector('.listen-hint'))input.insertAdjacentHTML('afterend','<small class="listen-hint">只填端口表示监听所有地址；需要限制地址时可填 127.0.0.1:8080。</small>')}catch(e){toast(e.message,true)}
}
async function saveSettingsV3(e){
 e.preventDefault();const f=e.currentTarget;let listen=String(formValue(f,'panelListen')||'').trim();if(/^\d+$/.test(listen))listen=':'+listen
 const body={panelListen:listen,panelBasePath:formValue(f,'panelBasePath'),panelDomain:formValue(f,'panelDomain'),panelCertFile:formValue(f,'panelCertFile'),panelKeyFile:formValue(f,'panelKeyFile'),xrayApiPort:Number(formValue(f,'xrayApiPort')),portMin:Number(formValue(f,'portMin')),portMax:Number(formValue(f,'portMax')),defaultRealitySni:formValue(f,'defaultRealitySni'),defaultRealityDest:formValue(f,'defaultRealityDest'),adminUsername:formValue(f,'adminUsername'),currentPassword:formValue(f,'currentPassword'),newPassword:formValue(f,'newPassword')}
 try{const d=await api('/api/settings',{method:'PUT',body:JSON.stringify(body)});f.elements.currentPassword.value='';f.elements.newPassword.value='';await loadSettingsV3();toast(d.restartRequired?'设置已保存；重启 X-port 后生效':'设置已保存')}catch(x){toast(x.message,true)}
}

function initV3(){
 renderAccounts=renderAccountsV3;openAccountModal=openAccountModalV3;loadSettings=loadSettingsV3
 const add=$('#add-account');if(add)add.onclick=()=>openAccountModalV3()
 const settings=$('#settings-form');if(settings)settings.onsubmit=saveSettingsV3
 const dayUnit=$('#uptime-days')?.nextElementSibling;if(dayUnit)dayUnit.textContent='天'
 if(state.page==='accounts'){renderAccountsV3();void loadOnlineConnectionsV3(true)}
 if(xportOnlineTimer)clearInterval(xportOnlineTimer)
 xportOnlineTimer=setInterval(()=>{if(state.page==='accounts'&&!$('#app').classList.contains('hidden'))void loadOnlineConnectionsV3(true)},5000)
}
document.addEventListener('DOMContentLoaded',initV3)
