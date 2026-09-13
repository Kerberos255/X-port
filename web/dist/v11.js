/* X-port next: structured fallbacks + editable global Xray sections. */
const fallbackKnownKeysV11=new Set(['name','alpn','path','dest','xver'])

function fallbackRuleHTMLV11(rule={}){
 const extras={}
 for(const [k,v] of Object.entries(rule||{}))if(!fallbackKnownKeysV11.has(k))extras[k]=v
 const dest=rule?.dest??''
 return `<div class="fallback-rule" data-extra="${esc(encodeURIComponent(JSON.stringify(extras)))}">
  <label>Name<input data-fallback-key="name" value="${esc(String(rule?.name??''))}" placeholder="可选"></label>
  <label>ALPN<input data-fallback-key="alpn" value="${esc(String(rule?.alpn??''))}" placeholder="例如 h2"></label>
  <label>Path<input data-fallback-key="path" value="${esc(String(rule?.path??''))}" placeholder="例如 /ws"></label>
  <label>Dest <span class="required-mark">*</span><input data-fallback-key="dest" value="${esc(String(dest))}" placeholder="80 / 127.0.0.1:8080 / @socket" required></label>
  <label>xver<input data-fallback-key="xver" type="number" min="0" max="2" step="1" value="${Number(rule?.xver||0)}"></label>
  <button type="button" class="icon-btn danger-icon fallback-remove" title="删除 Fallback">×</button>
 </div>`
}

function fallbackSectionHTMLV11(raw){
 let rules=[]
 try{const parsed=JSON.parse(raw||'[]');if(Array.isArray(parsed))rules=parsed}catch{}
 return `<div class="expert-section fallback-section"><div class="expert-section-title">Fallbacks <small>RAW/TCP + TLS</small></div>
  <textarea id="expert-fallbacks" class="fallback-json-source" spellcheck="false">${esc(raw||'[]')}</textarea>
  <p class="muted tiny full expert-note">按顺序匹配；Dest 必填。xver=1/2 仅在下游支持 PROXY Protocol 时开启。</p>
  <div class="fallback-rules">${rules.map(fallbackRuleHTMLV11).join('')}</div>
  <div class="fallback-actions"><button type="button" class="btn ghost fallback-add">＋ 添加 Fallback</button></div>
 </div>`
}

function upgradeFallbackEditorV11(html){
 const template=document.createElement('template')
 template.innerHTML=html
 const source=template.content.querySelector('#expert-fallbacks')
 if(!source)return html
 const raw=source.value||source.textContent||'[]'
 const oldContainer=source.closest('label')||source
 oldContainer.remove()
 const sections=[...template.content.querySelectorAll('.expert-section')]
 const tls=sections.find(s=>String(s.querySelector('.expert-section-title')?.textContent||'').trim().startsWith('TLS'))
 const common=sections.find(s=>String(s.querySelector('.expert-section-title')?.textContent||'').trim().startsWith('通用'))
 const holder=document.createElement('template')
 holder.innerHTML=fallbackSectionHTMLV11(raw)
 const node=holder.content.firstElementChild
 if(tls)tls.after(node)
 else if(common)common.after(node)
 else template.content.append(node)
 return template.innerHTML
}

const renderExpertV3BeforeV11=renderExpertV3
renderExpertV3=(...args)=>upgradeFallbackEditorV11(renderExpertV3BeforeV11(...args))

function fallbackExtraV11(row){
 try{return JSON.parse(decodeURIComponent(row.dataset.extra||'%7B%7D'))||{}}catch{return {}}
}
function fallbackValueV11(row,key){return String(row.querySelector(`[data-fallback-key="${key}"]`)?.value||'').trim()}
function collectFallbacksV11(section){
 const rows=[...section.querySelectorAll('.fallback-rule')]
 if(rows.length>64)throw new Error('Fallback 最多 64 条')
 return rows.map((row,index)=>{
  const rule=fallbackExtraV11(row)
  for(const key of ['name','alpn','path']){
   const value=fallbackValueV11(row,key)
   if(value)rule[key]=value;else delete rule[key]
  }
  const dest=fallbackValueV11(row,'dest')
  if(!dest)throw new Error(`第 ${index+1} 条 Fallback 的 Dest 不能为空`)
  if(/^\d+$/.test(dest)){
   const port=Number(dest)
   if(port<1||port>65535)throw new Error(`第 ${index+1} 条 Fallback 的端口必须在 1-65535`)
   rule.dest=port
  }else rule.dest=dest
  const xverText=fallbackValueV11(row,'xver')
  const xver=xverText===''?0:Number(xverText)
  if(!Number.isInteger(xver)||xver<0||xver>2)throw new Error(`第 ${index+1} 条 Fallback 的 xver 只能是 0、1、2`)
  if(xver)rule.xver=xver;else delete rule.xver
  return rule
 })
}
function syncFallbackSourceV11(section,showError=false){
 const source=section?.querySelector('#expert-fallbacks')
 if(!source)return true
 try{source.value=JSON.stringify(collectFallbacksV11(section),null,2);return true}
 catch(err){if(showError)toast(err.message,true);return false}
}

document.addEventListener('click',e=>{
 const add=e.target.closest('.fallback-add')
 if(add){
  const section=add.closest('.fallback-section'),list=section?.querySelector('.fallback-rules')
  if(!list)return
  if(list.querySelectorAll('.fallback-rule').length>=64){toast('Fallback 最多 64 条',true);return}
  list.insertAdjacentHTML('beforeend',fallbackRuleHTMLV11({xver:0}));syncFallbackSourceV11(section);return
 }
 const remove=e.target.closest('.fallback-remove')
 if(remove){const section=remove.closest('.fallback-section');remove.closest('.fallback-rule')?.remove();syncFallbackSourceV11(section)}
})
document.addEventListener('input',e=>{const section=e.target.closest?.('.fallback-section');if(section)syncFallbackSourceV11(section)})
document.addEventListener('click',e=>{
 if(!e.target.closest('#save-expert'))return
 const section=document.querySelector('.fallback-section')
 if(section&&!syncFallbackSourceV11(section,true)){e.preventDefault();e.stopImmediatePropagation()}
},true)

let xrayGlobalLoadedV11=false
function injectGlobalXrayEditorV11(){
 if(document.querySelector('#xray-global-config-panel'))return
 const page=document.querySelector('#page-xray'),log=page?.querySelector('.log-panel')
 if(!page||!log)return
 const panel=document.createElement('article')
 panel.className='panel global-xray-panel'
 panel.id='xray-global-config-panel'
 panel.innerHTML=`<div class="panel-head"><div><span class="kicker">ADVANCED CONFIG</span><h2>全局 Xray 高级配置</h2></div><span id="xray-global-state" class="status"><i></i>未读取</span></div>
  <p class="muted tiny">api / stats / inbounds 由 X-port 管理，不允许在该入口覆盖；可编辑 routing / dns / outbounds / policy。</p>
  <textarea id="xray-global-config" class="global-config-editor" spellcheck="false" placeholder='{"routing":{},"dns":{},"outbounds":[],"policy":{}}'></textarea>
  <div class="button-row global-config-actions"><button id="reload-xray-global" class="btn ghost" type="button">重新读取</button><button id="save-xray-global" class="btn primary" type="button">验证并应用</button></div>`
 log.parentNode.insertBefore(panel,log)
 panel.querySelector('#reload-xray-global').onclick=()=>loadGlobalXrayConfigV11(true)
 panel.querySelector('#save-xray-global').onclick=saveGlobalXrayConfigV11
}
function globalStateV11(text,ok=false){
 const el=document.querySelector('#xray-global-state');if(!el)return
 el.className=`status ${ok?'ok':''}`;el.innerHTML=`<i></i>${esc(text)}`
}
async function loadGlobalXrayConfigV11(force=false){
 injectGlobalXrayEditorV11()
 if(xrayGlobalLoadedV11&&!force)return
 const editor=document.querySelector('#xray-global-config');if(!editor)return
 globalStateV11('读取中')
 try{
  const d=await api('/api/xray/config')
  editor.value=JSON.stringify(d.config||{},null,2)
  xrayGlobalLoadedV11=true;globalStateV11('已读取',true)
 }catch(err){globalStateV11('读取失败');toast(err.message,true)}
}
function parseGlobalXrayEditorV11(){
 const raw=document.querySelector('#xray-global-config')?.value||'{}'
 let value
 try{value=JSON.parse(raw)}catch(err){throw new Error(`JSON 格式错误：${err.message}`)}
 if(!value||Array.isArray(value)||typeof value!=='object')throw new Error('全局 Xray 高级配置必须是 JSON 对象')
 const allowed=new Set(['routing','dns','outbounds','policy'])
 for(const key of Object.keys(value))if(!allowed.has(key))throw new Error(`${key} 不允许在这里编辑；仅支持 routing / dns / outbounds / policy`)
 return value
}
async function saveGlobalXrayConfigV11(){
 const button=document.querySelector('#save-xray-global'),editor=document.querySelector('#xray-global-config')
 let config
 try{config=parseGlobalXrayEditorV11()}catch(err){toast(err.message,true);return}
 if(!confirm('验证并应用全局 Xray 高级配置？\nX-port 会先执行 xray run -test；重启失败会自动恢复旧配置。'))return
 button.disabled=true;button.textContent='验证并应用中…';globalStateV11('验证中')
 try{
  const result=await api('/api/xray/config',{method:'PUT',body:JSON.stringify({config})})
  editor.value=JSON.stringify(result.config||{},null,2)
  xrayGlobalLoadedV11=true;globalStateV11('已应用',true);toast('全局 Xray 高级配置已应用')
 }catch(err){globalStateV11('应用失败');toast(err.message,true)}
 finally{button.disabled=false;button.textContent='验证并应用'}
}

function initV11(){
 injectGlobalXrayEditorV11()
 document.addEventListener('click',e=>{
  if(e.target.closest?.('#nav [data-page="xray"]'))setTimeout(()=>loadGlobalXrayConfigV11(false),0)
 })
 if(window.state?.page==='xray')void loadGlobalXrayConfigV11(false)
}
initV11()

;(() => {
 function toastEmphasisV11(msg,bad=false){
  const e=$('#toast')
  if(!e)return
  e.replaceChildren()
  const icon=document.createElement('span')
  icon.className='toast-icon'
  icon.textContent=bad?'!':'✓'
  const text=document.createElement('span')
  text.className='toast-message'
  text.textContent=String(msg??'')
  e.append(icon,text)
  e.classList.remove('hidden')
  e.classList.toggle('bad',bad)
  clearTimeout(toastEmphasisV11.t)
  toastEmphasisV11.t=setTimeout(()=>e.classList.add('hidden'),bad?6500:3000)
 }
 async function openCloneModalRandomPortV11(id){
  const a=state.accounts.find(x=>x.id===id)
  openModal(`<div class="modal-title"><span>CLONE ACCOUNT</span><h2>克隆 ${esc(a?.name||'账号')}</h2></div><form id="clone-form" class="form-grid"><label>新账号名<input name="name" value="${esc((a?.name||'account')+' copy')}" required></label><label>新端口<input id="clone-port" name="port" type="number" min="1" max="65535" placeholder="正在随机分配…"></label><p id="clone-port-note" class="muted tiny full">正在从设定端口范围内随机选择一个空闲端口…</p><p class="muted tiny full">协议与传输参数会复制；UUID/密码自动重新生成，本期与累计流量清零。</p><div class="form-actions full"><button type="button" class="btn ghost" id="clone-cancel">取消</button><button class="btn primary" type="submit">克隆并应用</button></div></form>`)
  $('#clone-cancel').onclick=closeModal
  const portInput=$('#clone-port'),portNote=$('#clone-port-note')
  try{
   const d=await api('/api/accounts/port-suggestion')
   if(!$('#clone-form'))return
   portInput.value=d.port||''
   portNote.textContent=`已自动从 ${d.min}-${d.max} 随机选择空闲端口；也可以手动修改。`
  }catch(err){
   if($('#clone-form'))portNote.textContent='自动分配失败，可手动填写；留空提交时后端仍会尝试自动分配。'
   toastEmphasisV11(err.message,true)
  }
  $('#clone-form').onsubmit=async e=>{
   e.preventDefault()
   const form=e.currentTarget,f=new FormData(form),button=form.querySelector('button[type=submit]')
   button.disabled=true;button.textContent='克隆中…'
   try{
    await api(`/api/accounts/${id}/clone`,{method:'POST',body:JSON.stringify({name:f.get('name'),port:Number(f.get('port')||0)})})
    closeModal();await loadAccounts();await loadOverview();toastEmphasisV11('账号已克隆')
   }catch(err){toastEmphasisV11(err.message,true);button.disabled=false;button.textContent='克隆并应用'}
  }
 }
 toast=toastEmphasisV11
 openCloneModal=openCloneModalRandomPortV11
})()
