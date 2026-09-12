/* X-port v0.1.5: inbound correctness and system/settings UX. */
let xportPanelListenHostV6=''
let xportSettingsAdminV6=''
let xportPortRangeV6={min:20000,max:60000}

function parsePanelListenV6(value){
 const v=String(value||'').trim()
 if(/^\d+$/.test(v))return{host:'',port:v}
 let m=v.match(/^:(\d+)$/);if(m)return{host:'',port:m[1]}
 m=v.match(/^(\[[^\]]+\]|[^:]+):(\d+)$/);if(m)return{host:m[1],port:m[2]}
 return{host:'',port:v.replace(/^:/,'')}
}
function joinPanelListenV6(host,port){return host?`${host}:${port}`:`:${port}`}
function visionCompatibleV6(protocol,network,security){return protocol==='vless'&&['tcp','raw'].includes(network)&&['tls','reality'].includes(security)}
function securityChoicesV6(protocol,network){
 if(!['vless','trojan'].includes(protocol))return['none','tls']
 return['ws','websocket','httpupgrade'].includes(network)?['none','tls']:['none','tls','reality']
}

function protocolFieldsV6(p,a){
 const transport=!['socks','http'].includes(p),network=a.network||'tcp',security=a.security||(p==='vless'||p==='trojan'?'reality':'none');let cred=''
 if(p==='vless'){
  const canVision=visionCompatibleV6(p,network,security),flow=canVision?(a.flow||(a.id?'':'xtls-rprx-vision')):''
  cred=`<label>UUID<input name="credential" value="${esc(a.credential||'')}" placeholder="留空自动生成"></label><label class="vless-flow ${canVision?'':'hidden-field'}">Flow<select name="flow"><option value="" ${flow===''?'selected':''}>无</option><option value="xtls-rprx-vision" ${flow==='xtls-rprx-vision'?'selected':''}>xtls-rprx-vision</option></select></label>`
 }
 if(p==='vmess')cred=`<label>UUID<input name="credential" value="${esc(a.credential||'')}" placeholder="留空自动生成"></label><label>VMess Security<select name="clientSecurity">${['auto','aes-128-gcm','chacha20-poly1305','none','zero'].map(x=>option(x,x,a.clientSecurity||'auto')).join('')}</select></label>`
 if(p==='trojan')cred=`<label>Trojan 密码<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成"></label>`
 if(p==='shadowsocks')cred=`<label>加密方式<select name="method">${['chacha20-ietf-poly1305','aes-256-gcm','2022-blake3-aes-256-gcm','2022-blake3-chacha20-poly1305'].map(x=>option(x,x,a.method||'chacha20-ietf-poly1305')).join('')}</select></label><label>服务器密码<input name="serverPassword" value="${esc(a.serverPassword||'')}" placeholder="留空自动生成"></label><label>客户端密码<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成"></label>`
 if(p==='socks'||p==='http')cred=`<label>用户名<input name="username" value="${esc(a.username||'')}"></label><label>密码<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成"></label>`
 if(!transport)return `<div class="advanced-grid">${cred}</div>`
 const secChoices=securityChoicesV6(p,network),safeSecurity=secChoices.includes(security)?security:secChoices[0]
 return `<div class="advanced-grid">${cred}<label>传输<select name="network" id="network-select">${[['tcp','RAW'],['xhttp','XHTTP'],['grpc','gRPC'],['ws','WebSocket'],['httpupgrade','HTTPUpgrade']].map(([v,l])=>option(v,l,network)).join('')}</select></label><label>传输安全<select name="security" id="security-select">${secChoices.map(x=>option(x,x.toUpperCase(),safeSecurity)).join('')}</select></label><label class="transport-host">Host<input name="host" value="${esc(a.host||'')}"></label><label class="transport-path">Path<input name="path" value="${esc(a.path||'')}" placeholder="/"></label><label class="transport-service">gRPC Service<input name="serviceName" value="${esc(a.serviceName||'')}"></label></div>`
}
protocolFieldsV3=protocolFieldsV6

function syntheticExpertV6(protocol,network,security){
 const method=network==='tcp'?'raw':network
 return {protocol,method,security,acceptProxyProtocol:false,fallbacksJson:'[]',httpHeaderJson:'',supportsFallbacks:(protocol==='vless'||protocol==='trojan')&&method==='raw'&&security==='tls',supportsHttpHeader:method==='raw',supportsReality:security==='reality',supportsTls:security==='tls',supportsXhttp:method==='xhttp',xhttpMode:'auto',xhttpXPaddingBytes:'',xhttpNoSseHeader:false,xhttpScMaxBufferedPosts:0,xhttpScMaxEachPostBytes:'',xhttpScStreamUpServerSecs:'',xhttpUplinkHttpMethod:''}
}

refreshProtocolFieldsV3=function(full){
 const p=$('#protocol-select')?.value||full.protocol||'vless',box=$('#proto-fields');if(!box)return
 box.innerHTML=protocolFieldsV6(p,full)
 const net=$('#network-select'),sec=$('#security-select'),details=$('#account-advanced')
 const tune=()=>{
  const n=net?.value||'tcp',allowed=securityChoicesV6(p,n),old=sec?.value||'none'
  if(sec){const next=allowed.includes(old)?old:(allowed.includes('reality')?'reality':allowed[0]);sec.innerHTML=allowed.map(x=>option(x,x.toUpperCase(),next)).join('');sec.value=next}
  const security=sec?.value||'none'
  $$('.transport-host',box).forEach(e=>e.style.display=['ws','httpupgrade','xhttp'].includes(n)?'grid':'none')
  $$('.transport-path',box).forEach(e=>e.style.display=['ws','httpupgrade','xhttp'].includes(n)?'grid':'none')
  $$('.transport-service',box).forEach(e=>e.style.display=n==='grpc'?'grid':'none')
  const flow=box.querySelector('.vless-flow'),flowSelect=flow?.querySelector('select[name="flow"]'),canVision=visionCompatibleV6(p,n,security)
  if(flow){flow.classList.toggle('hidden-field',!canVision);if(!canVision&&flowSelect)flowSelect.value=''}
  if(!full.id&&details?.open){details.dataset.loaded='1';details._expertData=syntheticExpertV6(p,n,security);const c=$('#expert-content');if(c)c.innerHTML=renderExpertV3(details._expertData)}
 }
 net?.addEventListener('change',tune);sec?.addEventListener('change',tune);tune()
 if(!full.id&&details)details.addEventListener('toggle',()=>{if(details.open){const n=net?.value||'tcp',security=sec?.value||'none';details.dataset.loaded='1';details._expertData=syntheticExpertV6(p,n,security);const c=$('#expert-content');if(c)c.innerHTML=renderExpertV3(details._expertData)}})
}

function accountFormHTMLV6(a={}){
 const p=String(a.protocol||'vless').toLowerCase(),editing=Boolean(a.id)
 if(editing&&!a.editable)return `<div class="modal-title"><span>IMPORTED ACCOUNT</span><h2>${esc(a.name)}</h2></div><div class="readonly-note"><p>该账号包含当前内置编辑器不支持的配置，X-port 会保持原始 Xray 配置不变。</p></div>`
 const expiry=a.expiryTime?new Date(Number(a.expiryTime)-new Date().getTimezoneOffset()*60000).toISOString().slice(0,16):'',qgb=a.quotaBytes?(Number(a.quotaBytes)/1073741824).toFixed(2):'',enabled=editing?Boolean(a.enabled):true
 const range=a._portMin&&a._portMax?`${a._portMin}–${a._portMax}`:`${xportPortRangeV6.min}–${xportPortRangeV6.max}`
 return `<div class="modal-title"><span>${editing?'EDIT ACCOUNT':'NEW ACCOUNT'}</span><h2>${editing?'编辑账号':'新增账号'}</h2></div><form id="account-form" class="form-grid"><label class="protocol-first full">${reqLabelV3('协议')}<select name="protocol" id="protocol-select" ${editing?'disabled':''}>${protocols.map(x=>option(x,x.toUpperCase(),p)).join('')}</select>${editing?'<small class="field-hint">创建后不可修改</small>':''}</label><label>${reqLabelV3('账号')}<input name="name" value="${esc(a.name||'')}" required maxlength="80"></label><label>端口<input name="port" type="number" min="1" max="65535" value="${a.port||''}" placeholder="留空自动分配"><small class="account-port-note">自动端口范围 ${esc(range)}</small></label><label>流量上限 / GB<input name="quota" type="number" min="0" step="0.1" value="${qgb}" placeholder="0 = 不限"></label><label>到期时间<input name="expiry" type="datetime-local" value="${expiry}"></label><div class="full proto-fields" id="proto-fields"></div><details id="account-advanced" class="advanced account-advanced full"><summary><span>高级</span></summary><div id="expert-content" class="advanced-loading muted tiny"></div></details><div class="account-editor-footer full"><div class="editor-policies">${editorPolicyV5('enabled','启用账号',enabled)}${editorPolicyV5('monthlyReset','每月 1 日自动清零',Boolean(a.monthlyReset))}</div><div class="editor-actions"><button type="button" class="btn ghost" id="cancel-form">取消</button><button type="submit" class="btn primary">${editing?'保存并应用':'创建并应用'}</button></div></div></form>`
}
accountFormHTMLV3=accountFormHTMLV6

openAccountModalV3=async function(a=null){
 let full=a||{}
 if(a?.id){try{full=await api(`/api/accounts/${a.id}`)}catch(e){toast(e.message,true);return}}
 else{
  try{const d=await api('/api/accounts/port-suggestion');full={...full,port:d.port,_portMin:d.min,_portMax:d.max};xportPortRangeV6={min:d.min,max:d.max}}
  catch{try{const d=await api('/api/settings');xportPortRangeV6={min:Number(d.portMin)||20000,max:Number(d.portMax)||60000};full={...full,_portMin:xportPortRangeV6.min,_portMax:xportPortRangeV6.max}}catch{}}
 }
 openModal(accountFormHTMLV6(full));if(full.id&&!full.editable)return
 $('#cancel-form').onclick=closeModal
 refreshProtocolFieldsV3(full)
 const details=$('#account-advanced')
 if(full.id)details?.addEventListener('toggle',()=>{if(details.open&&!details.dataset.loaded){details.dataset.loaded='1';void loadExpertV3(full.id,details)}})
 $('#protocol-select')?.addEventListener('change',()=>{const p=$('#protocol-select').value;full={...full,protocol:p,credential:'',username:'',password:'',serverPassword:'',privateKey:'',shortId:'',serverName:'',dest:'',host:'',path:'',serviceName:'',network:'tcp',security:(p==='vless'||p==='trojan')?'reality':'none',flow:''};refreshProtocolFieldsV3(full)})
 $('#account-form').onsubmit=async e=>{
  e.preventDefault();const f=e.currentTarget,p=full.id?full.protocol:formValue(f,'protocol'),port=Number(formValue(f,'port')||0),fd=new FormData(f)
  const body={name:formValue(f,'name'),enabled:fd.get('enabled')==='on',monthlyReset:fd.get('monthlyReset')==='on',port,protocol:p,credential:formValue(f,'credential'),username:formValue(f,'username'),password:formValue(f,'password'),serverPassword:formValue(f,'serverPassword'),clientSecurity:formValue(f,'clientSecurity'),method:formValue(f,'method'),flow:formValue(f,'flow'),network:formValue(f,'network'),security:formValue(f,'security'),serverName:formValue(f,'serverName'),dest:formValue(f,'dest'),privateKey:formValue(f,'privateKey'),shortId:formValue(f,'shortId'),host:formValue(f,'host'),path:formValue(f,'path'),serviceName:formValue(f,'serviceName'),quotaBytes:Math.round(Number(formValue(f,'quota')||0)*1073741824),expiryTime:formValue(f,'expiry')?new Date(formValue(f,'expiry')).getTime():0}
  const expertData=(details?.dataset.loaded==='1'&&details._expertData)?expertBodyFromFormV3(details._expertData):null
  const b=f.querySelector('button[type=submit]');b.disabled=true;b.textContent='应用中…';const before=new Set(state.accounts.map(x=>Number(x.id)))
  try{
   const result=await mutationWithRecoveryV3(()=>full.id?api(`/api/accounts/${full.id}`,{method:'PUT',body:JSON.stringify(body)}):api('/api/accounts',{method:'POST',body:JSON.stringify(body)}),async()=>{if(full.id){const cur=await api(`/api/accounts/${full.id}`);return cur.name===body.name&&Number(cur.port)===(body.port||Number(cur.port))}const d=await api('/api/accounts');return(d.accounts||[]).some(x=>!before.has(Number(x.id))&&x.name===body.name)})
   let savedID=Number(full.id||result.value?.id||0)
   if(!savedID&&!full.id){const d=await api('/api/accounts');const created=(d.accounts||[]).find(x=>!before.has(Number(x.id))&&x.name===body.name);savedID=Number(created?.id||0)}
   if(expertData&&savedID)await mutationWithRecoveryV3(()=>api(`/api/accounts/${savedID}/expert`,{method:'PUT',body:JSON.stringify(expertData)}),async()=>Boolean(await api(`/api/accounts/${savedID}/expert`)))
   closeModal();await loadAccounts();await loadOverview();await loadOnlineConnectionsV3(false);toast(full.id?'账号已更新':'账号已创建')
  }catch(x){toast(x.message,true);b.disabled=false;b.textContent=full.id?'保存并应用':'创建并应用'}
 }
}
openAccountModal=openAccountModalV3

function settingsCredentialChangedV6(f){return String(formValue(f,'adminUsername')).trim()!==xportSettingsAdminV6||String(formValue(f,'newPassword')).length>0}
function syncAdminReauthV6(){
 const f=$('#settings-form');if(!f)return
 const changed=settingsCredentialChangedV6(f);let wrap=$('#settings-current-password-wrap')
 if(changed&&!wrap){const anchor=f.elements.namedItem('newPassword')?.closest('label');anchor?.insertAdjacentHTML('afterend','<label id="settings-current-password-wrap" class="admin-reauth">确认当前密码*<input name="currentPassword" type="password" autocomplete="current-password" required></label>')}
 if(!changed&&wrap)wrap.remove()
}
async function loadSettingsV6(){
 try{
  const d=await api('/api/settings'),f=$('#settings-form'),listen=parsePanelListenV6(d.panelListen)
  xportPanelListenHostV6=listen.host;xportSettingsAdminV6=String(d.adminUsername||'');xportPortRangeV6={min:Number(d.portMin)||20000,max:Number(d.portMax)||60000}
  for(const [k,v] of Object.entries(d)){if(k==='panelListen'||k==='currentPassword')continue;const el=f.elements.namedItem(k);if(el)el.value=v??''}
  f.elements.namedItem('panelListen').value=listen.port;f.elements.namedItem('newPassword').value='';$('#settings-current-password-wrap')?.remove()
 }catch(e){toast(e.message,true)}
}
async function saveSettingsV6(e){
 e.preventDefault();const f=e.currentTarget,port=String(formValue(f,'panelListen')||'').trim()
 if(!/^\d+$/.test(port)||Number(port)<1||Number(port)>65535){toast('面板端口必须是 1–65535',true);return}
 const changing=settingsCredentialChangedV6(f),current=changing?String(formValue(f,'currentPassword')||''):''
 if(changing&&!current){syncAdminReauthV6();f.elements.namedItem('currentPassword')?.focus();toast('修改管理员账号或密码时需要确认当前密码',true);return}
 const body={panelListen:joinPanelListenV6(xportPanelListenHostV6,port),panelBasePath:formValue(f,'panelBasePath'),panelDomain:formValue(f,'panelDomain'),panelCertFile:formValue(f,'panelCertFile'),panelKeyFile:formValue(f,'panelKeyFile'),xrayApiPort:Number(formValue(f,'xrayApiPort')),portMin:Number(formValue(f,'portMin')),portMax:Number(formValue(f,'portMax')),defaultRealitySni:formValue(f,'defaultRealitySni'),defaultRealityDest:formValue(f,'defaultRealityDest'),adminUsername:formValue(f,'adminUsername'),currentPassword:current,newPassword:formValue(f,'newPassword')}
 try{const d=await api('/api/settings',{method:'PUT',body:JSON.stringify(body)});await loadSettingsV6();toast(d.restartRequired?'设置已保存；重启 X-port 后生效':'设置已保存')}catch(x){toast(x.message,true)}
}

function initV6(){
 loadSettings=loadSettingsV6
 const f=$('#settings-form');if(f){f.onsubmit=saveSettingsV6;f.querySelector('.listen-hint')?.remove();f.elements.namedItem('adminUsername')?.addEventListener('input',syncAdminReauthV6);f.elements.namedItem('newPassword')?.addEventListener('input',syncAdminReauthV6)}
 const add=$('#add-account');if(add)add.onclick=()=>void openAccountModalV3()
}
document.addEventListener('DOMContentLoaded',initV6)
