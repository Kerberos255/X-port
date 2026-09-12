/* X-port v0.1.4 real-UI fixes. Loaded after v4. */

/* Required marks belong to the field label, not a second grid row. */
if(typeof reqLabelV3==='function'){
 reqLabelV3=function(text){return `<span class="field-label">${text}<span class="required-mark">*</span></span>`}
}

/* Keep Advanced focused on settings rather than explanatory copy. */
if(typeof expertSectionV3==='function'){
 expertSectionV3=function(title,sub,body){return `<div class="expert-section"><div class="expert-section-title">${title}</div>${body}</div>`}
}
if(typeof renderExpertV3==='function'){
 const renderExpertV5Base=renderExpertV3
 renderExpertV3=function(d){
  let html=renderExpertV5Base(d)
  html=html.replace(/<small class="field-hint">[\s\S]*?<\/small>/g,'')
  html=html.replace(/<p class="muted tiny(?: full)? expert-note">[\s\S]*?<\/p>/g,'')
  html=html.replace(/<div class="expert-actions"><button id="save-expert"[\s\S]*?<\/div>/g,'')
  return html
 }
}
if(typeof createAdvancedV3==='function'){
 const createAdvancedV5Base=createAdvancedV3
 createAdvancedV3=function(...args){return createAdvancedV5Base(...args).replace(/<p class="muted tiny(?: full)? expert-note">[\s\S]*?<\/p>/g,'')}
}

/* Advanced settings are loaded into the account form and saved by the same
   bottom-right Save button as the rest of the account. */
loadExpertV3=async function(id,details){
 const c=$('#expert-content');if(!c)return
 c.textContent='正在读取…'
 try{
  const d=await api(`/api/accounts/${id}/expert`)
  details._expertData=d
  c.innerHTML=renderExpertV3(d)
 }catch(e){c.innerHTML=`<p class="error tiny">${esc(e.message)}</p>`}
}

function editorPolicyV5(name,label,checked){
 return `<label class="editor-policy"><input name="${name}" type="checkbox" ${checked?'checked':''}><span class="editor-policy-track"><i></i></span><span>${label}</span></label>`
}

accountFormHTMLV3=function(a={}){
 const p=String(a.protocol||'vless').toLowerCase(),editing=Boolean(a.id)
 if(editing&&!a.editable)return `<div class="modal-title"><span>IMPORTED ACCOUNT</span><h2>${esc(a.name)}</h2></div><div class="readonly-note"><p>该账号使用 ${esc(p.toUpperCase())}，X-port 会继续原样运行它。</p></div>`
 const expiry=a.expiryTime?new Date(Number(a.expiryTime)-new Date().getTimezoneOffset()*60000).toISOString().slice(0,16):''
 const qgb=a.quotaBytes?(Number(a.quotaBytes)/1073741824).toFixed(2):''
 const enabled=editing?Boolean(a.enabled):true
 return `<div class="modal-title"><span>${editing?'EDIT ACCOUNT':'NEW ACCOUNT'}</span><h2>${editing?'编辑账号':'新增账号'}</h2></div><form id="account-form" class="form-grid">
 <label class="protocol-first full">${reqLabelV3('协议')}<select name="protocol" id="protocol-select" ${editing?'disabled':''}>${protocols.map(x=>option(x,x.toUpperCase(),p)).join('')}</select>${editing?'<small class="field-hint">创建后不可修改</small>':''}</label>
 <label>${reqLabelV3('账号')}<input name="name" value="${esc(a.name||'')}" required maxlength="80"></label>
 <label>${reqLabelV3('端口')}<input name="port" type="number" min="1" max="65535" value="${a.port||''}" required placeholder="1 - 65535"></label>
 <label>流量上限 / GB<input name="quota" type="number" min="0" step="0.1" value="${qgb}" placeholder="0 = 不限"></label>
 <label>到期时间<input name="expiry" type="datetime-local" value="${expiry}"></label>
 <div class="full proto-fields" id="proto-fields"></div>
 <details id="account-advanced" class="advanced account-advanced full"><summary><span>高级</span></summary><div id="expert-content" class="advanced-loading muted tiny"></div></details>
 <div class="account-editor-footer full"><div class="editor-policies">${editorPolicyV5('enabled','启用账号',enabled)}${editorPolicyV5('monthlyReset','每月 1 日自动清零',Boolean(a.monthlyReset))}</div><div class="editor-actions"><button type="button" class="btn ghost" id="cancel-form">取消</button><button type="submit" class="btn primary">${editing?'保存并应用':'创建并应用'}</button></div></div>
 </form>`
}

openAccountModalV3=async function(a=null){
 let full=a||{}
 if(a?.id){try{full=await api(`/api/accounts/${a.id}`)}catch(e){toast(e.message,true);return}}
 openModal(accountFormHTMLV3(full));if(full.id&&!full.editable)return
 $('#cancel-form').onclick=closeModal
 refreshProtocolFieldsV3(full)
 const details=$('#account-advanced')
 if(full.id)details?.addEventListener('toggle',()=>{if(details.open&&!details.dataset.loaded){details.dataset.loaded='1';void loadExpertV3(full.id,details)}})
 $('#protocol-select')?.addEventListener('change',()=>{const p=$('#protocol-select').value;full={...full,protocol:p,credential:'',username:'',password:'',serverPassword:'',privateKey:'',shortId:'',serverName:'',dest:'',host:'',path:'',serviceName:'',network:'tcp',security:(p==='vless'||p==='trojan')?'reality':'none'};refreshProtocolFieldsV3(full)})
 $('#account-form').onsubmit=async e=>{
  e.preventDefault();const f=e.currentTarget,p=full.id?full.protocol:formValue(f,'protocol'),port=Number(formValue(f,'port'))
  if(!port){toast('端口不能为空',true);return}
  const fd=new FormData(f)
  const body={name:formValue(f,'name'),enabled:fd.get('enabled')==='on',monthlyReset:fd.get('monthlyReset')==='on',port,protocol:p,credential:formValue(f,'credential'),username:formValue(f,'username'),password:formValue(f,'password'),serverPassword:formValue(f,'serverPassword'),clientSecurity:formValue(f,'clientSecurity'),method:formValue(f,'method'),flow:formValue(f,'flow'),network:formValue(f,'network'),security:formValue(f,'security'),serverName:formValue(f,'serverName'),dest:formValue(f,'dest'),privateKey:formValue(f,'privateKey'),shortId:formValue(f,'shortId'),host:formValue(f,'host'),path:formValue(f,'path'),serviceName:formValue(f,'serviceName'),quotaBytes:Math.round(Number(formValue(f,'quota')||0)*1073741824),expiryTime:formValue(f,'expiry')?new Date(formValue(f,'expiry')).getTime():0}
  const expertData=(full.id&&details?.dataset.loaded==='1'&&details._expertData)?expertBodyFromFormV3(details._expertData):null
  const b=f.querySelector('button[type=submit]');b.disabled=true;b.textContent='应用中…';const before=new Set(state.accounts.map(x=>Number(x.id)))
  try{
   await mutationWithRecoveryV3(()=>full.id?api(`/api/accounts/${full.id}`,{method:'PUT',body:JSON.stringify(body)}):api('/api/accounts',{method:'POST',body:JSON.stringify(body)}),async()=>{if(full.id){const cur=await api(`/api/accounts/${full.id}`);return cur.name===body.name&&Number(cur.port)===body.port&&String(cur.protocol).toLowerCase()===String(body.protocol).toLowerCase()}const d=await api('/api/accounts');return(d.accounts||[]).some(x=>!before.has(Number(x.id))&&x.name===body.name&&Number(x.port)===body.port)})
   if(expertData)await mutationWithRecoveryV3(()=>api(`/api/accounts/${full.id}/expert`,{method:'PUT',body:JSON.stringify(expertData)}),async()=>Boolean(await api(`/api/accounts/${full.id}/expert`)))
   closeModal();await loadAccounts();await loadOverview();await loadOnlineConnectionsV3(false);toast(full.id?'账号已更新':'账号已创建')
  }catch(x){toast(x.message,true);b.disabled=false;b.textContent=full.id?'保存并应用':'创建并应用'}
 }
}

/* Account list switch state stays entirely inside the switch. */
if(typeof renderAccountsV3==='function'){
 const renderAccountsV5Base=renderAccountsV3
 renderAccountsV3=function(){
  const out=renderAccountsV5Base()
  $$('.account-toggle').forEach(b=>{
   const on=b.classList.contains('on')
   let label=b.querySelector('.account-toggle-state')
   if(!label){label=document.createElement('span');label.className='account-toggle-state';b.appendChild(label)}
   label.textContent=on?'开':'关'
  })
  return out
 }
}

function arrangeOverviewV5(){
 const grid=$('.overview-grid');if(!grid)return
 const server=$('.server-panel',grid),services=$('.services',grid),traffic=$('.traffic-panel',grid),health=$('.health-panel',grid),update=$('.update-panel',grid)
 if(update)update.style.display='none'
 ;[server,services,traffic,health].filter(Boolean).forEach(el=>grid.appendChild(el))
}
function formatBootV5(v){const n=Number(v||0);return n?new Date(n).toLocaleString():'—'}
function renderServerDetailsV5(){
 const panel=$('.server-panel'),sys=state.overview?.system||{};if(!panel)return
 let box=$('#server-details')
 if(!box){box=document.createElement('div');box.id='server-details';box.className='server-details';panel.querySelector('.server-core')?.insertAdjacentElement('afterend',box)}
 const fields=[['发行版本',sys.distribution||sys.os||'—'],['内核版本',sys.kernelVersion||'—'],['系统类型',sys.systemType||sys.os||'—'],['主机地址',sys.hostAddress||'—'],['启动时间',formatBootV5(sys.bootTime)]]
 box.innerHTML=fields.map(([k,v],i)=>`<div class="server-detail ${i===4?'wide':''}"><small>${esc(k)}</small><b title="${esc(v)}">${esc(v)}</b></div>`).join('')
}
if(typeof loadOverview==='function'){
 const loadOverviewV5Base=loadOverview
 loadOverview=async function(...args){const out=await loadOverviewV5Base(...args);renderServerDetailsV5();return out}
}

document.addEventListener('DOMContentLoaded',()=>{arrangeOverviewV5();renderServerDetailsV5()})
