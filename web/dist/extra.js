let xportGeodata=null
let xportSelfUpdate=null
let xrayRollback=null
const xportThemeKey='xport-theme'

function applyTheme(theme){
 const value=theme==='light'?'light':'dark'
 document.documentElement.dataset.theme=value
 document.documentElement.style.colorScheme=value
 localStorage.setItem(xportThemeKey,value)
 const meta=document.querySelector('meta[name="theme-color"]');if(meta)meta.content=value==='light'?'#f4f7fb':'#070b12'
 const b=$('#theme-toggle');if(b){b.querySelector('.theme-icon').textContent=value==='light'?'☾':'☀';b.querySelector('.theme-label').textContent=value==='light'?'夜间模式':'白天模式';b.title=value==='light'?'切换到夜间模式':'切换到白天模式'}
}
applyTheme(localStorage.getItem(xportThemeKey)||'dark')

async function loadSettings(){
 try{
  const d=await api('/api/settings'),f=$('#settings-form')
  for(const [k,v] of Object.entries(d)){const el=f.elements.namedItem(k);if(el)el.value=v??''}
  f.elements.currentPassword.value='';f.elements.newPassword.value=''
 }catch(e){toast(e.message,true)}
}

async function saveSettings(e){
 e.preventDefault();const f=e.currentTarget
 const body={panelListen:formValue(f,'panelListen'),panelBasePath:formValue(f,'panelBasePath'),panelDomain:formValue(f,'panelDomain'),panelCertFile:formValue(f,'panelCertFile'),panelKeyFile:formValue(f,'panelKeyFile'),xrayApiPort:Number(formValue(f,'xrayApiPort')),portMin:Number(formValue(f,'portMin')),portMax:Number(formValue(f,'portMax')),defaultRealitySni:formValue(f,'defaultRealitySni'),defaultRealityDest:formValue(f,'defaultRealityDest'),adminUsername:formValue(f,'adminUsername'),currentPassword:formValue(f,'currentPassword'),newPassword:formValue(f,'newPassword')}
 try{const d=await api('/api/settings',{method:'PUT',body:JSON.stringify(body)});f.elements.currentPassword.value='';f.elements.newPassword.value='';await loadSettings();toast(d.restartRequired?'设置已保存；重启 X-port 后生效':'设置已保存')}catch(x){toast(x.message,true)}
}

function exportAccounts(){
 const host=prompt('批量导出使用哪个服务器域名 / IP？',location.hostname||'')
 if(host===null)return
 if(!host.trim()){toast('服务器域名 / IP 不能为空',true);return}
 location.href=`/api/accounts/export?host=${encodeURIComponent(host.trim())}`
}

function renderAccounts(){
 const arr=state.accounts,list=$('#accounts-list')
 const up=arr.reduce((n,a)=>n+Number(a.upBytes||0),0),down=arr.reduce((n,a)=>n+Number(a.downBytes||0),0),all=arr.reduce((n,a)=>n+Number(a.allTimeBytes||0),0)
 $('#sum-total').textContent=arr.length
 $('#sum-active').textContent=arr.filter(a=>a.enabled).length
 $('#sum-traffic').textContent=`↑ ${fmtBytes(up)} · ↓ ${fmtBytes(down)}`
 $('#sum-monthly').textContent=fmtBytes(all)
 if(!arr.length){list.innerHTML='<div class="empty"><b>还没有账号</b><span>新增账号，或通过迁移脚本导入 X-Panel / 3x-ui。</span></div>';return}
 list.innerHTML=arr.map(a=>{
  const used=Number(a.upBytes||0)+Number(a.downBytes||0),quota=Number(a.quotaBytes||0),pct=quota?Math.min(100,Math.round(used/quota*100)):0,editable=Boolean(a.editable)
  const toggleTitle=a.enabled?'停用账号':a.disabledReason==='quota'?'额度已用完':a.disabledReason==='expiry'?'账号已到期':'启用账号'
  return `<article class="account-row" data-id="${a.id}"><div class="account-name"><i class="state ${a.enabled?'on':''}"></i><div><b>${esc(a.name)}</b><small>${esc(reasonText(a))} · ID ${a.id}</small></div></div><div class="enable-cell"><button class="account-toggle ${a.enabled?'on':''}" data-act="toggle" role="switch" aria-checked="${a.enabled?'true':'false'}" title="${esc(toggleTitle)}"><i></i></button></div><div class="port-cell"><span class="tag">:${a.port}</span></div><div class="protocol-cell hide-mid"><span class="tag">${esc(String(a.protocol||'').toUpperCase())}${a.security?` · ${esc(String(a.security).toUpperCase())}`:''}</span></div><div class="usage-cell hide-mid"><small>本期 ${quota?`/ ${fmtBytes(quota)}`:'· 不限'}</small><b>${fmtBytes(used)}</b><div class="mini-bar"><i style="width:${pct}%"></i></div></div><div class="hide-mid"><small class="muted tiny">${a.expiryTime?new Date(a.expiryTime).toLocaleDateString():'永久'}</small></div><div class="row-actions"><button class="icon-btn" data-act="edit" title="编辑">✎</button><button class="icon-btn" data-act="clone" title="克隆" ${editable?'':'disabled'}>⧉</button><button class="icon-btn" data-act="share" title="分享" ${editable?'':'disabled'}>⌁</button><button class="icon-btn" data-act="reset" title="重置本期流量">↺</button><button class="icon-btn danger-icon" data-act="delete" title="删除">×</button></div></article>`
 }).join('')
 $$('.account-row').forEach(row=>row.onclick=e=>{
  const b=e.target.closest('button[data-act]');if(!b||b.disabled)return
  const id=Number(row.dataset.id),a=state.accounts.find(x=>x.id===id)
  if(b.dataset.act==='toggle'){void toggleAccount(id,!a.enabled,b);return}
  ;({edit:()=>openAccountModal(a),clone:()=>openCloneModal(id),delete:()=>deleteAccount(id),share:()=>openShareModal(id),reset:()=>resetTraffic(id)})[b.dataset.act]?.()
 })
}

async function toggleAccount(id,enabled,button){
 button.disabled=true
 try{await api(`/api/accounts/${id}/enabled`,{method:'PATCH',body:JSON.stringify({enabled})});await loadAccounts();await loadOverview();toast(enabled?'账号已启用':'账号已停用')}catch(e){toast(e.message,true);button.disabled=false}
}

async function checkGeodata(manual=false){
 const cur=$('#geodata-current'),latest=$('#geodata-latest'),stateEl=$('#geodata-state'),btn=$('#update-geodata')
 if(!cur||!latest||!stateEl||!btn)return
 latest.textContent='检查中';btn.disabled=true
 try{
  const d=await api('/api/xray/geodata');xportGeodata=d
  cur.textContent=d.current||'未由 X-port 管理';latest.textContent=d.latest||'—'
  stateEl.className=`status ${d.available?'':'ok'}`;stateEl.innerHTML=`<i></i>${d.available?'可更新':'已是最新'}`
  btn.disabled=!d.available;btn.textContent=d.available?'更新 GeoData':'已是最新'
  if(manual)toast(d.available?'发现 GeoData 更新':'GeoData 已是最新')
 }catch(e){xportGeodata=null;latest.textContent='检查失败';stateEl.className='status bad';stateEl.innerHTML='<i></i>检查失败';if(manual)toast(e.message,true)}
}

async function updateGeodata(){
 if(!xportGeodata?.available){await checkGeodata(true);if(!xportGeodata?.available)return}
 if(!confirm(`将 geoip.dat / geosite.dat 更新到 ${xportGeodata.latest}？\n两个文件会一起替换，Xray 重启失败自动回滚。`))return
 const b=$('#update-geodata');b.disabled=true;b.textContent='更新中…'
 try{const d=await api('/api/xray/geodata',{method:'POST',body:'{}'});xportGeodata=d;toast(`GeoData 已更新到 ${d.current}`);await checkGeodata()}catch(e){toast(e.message,true);b.disabled=false;b.textContent='重试更新'}
}

function buildXrayUpdatePanel(){
 const panels=$$('#page-xray .two-col .panel'),panel=panels[1];if(!panel||$('#xray-page-update-state'))return
 panel.innerHTML=`<div class="panel-head"><div><span class="kicker">UPDATE CHANNEL</span><h2>官方 Release</h2></div><span id="xray-page-update-state" class="status"><i></i>检查中</span></div><div class="version-line"><div><small>当前版本</small><b id="xray-page-update-current">—</b></div><div><small>官方最新</small><b id="xray-page-update-latest">—</b></div></div><p id="xray-page-update-note" class="muted tiny">与概览共用同一次检查结果，不重复请求。</p><div class="button-row"><button id="xray-page-check" class="btn ghost">重新检查</button><button id="xray-page-do-update" class="btn primary" disabled>更新 Xray</button><button id="xray-page-rollback" class="btn ghost" disabled>暂无上一版</button></div>`
 $('#xray-page-check').onclick=()=>checkUpdate(true)
 $('#xray-page-do-update').onclick=updateXray
 $('#xray-page-rollback').onclick=rollbackXray
}

function renderXrayUpdateState(d,error=''){
 buildXrayUpdatePanel()
 const stateEl=$('#xray-page-update-state'),cur=$('#xray-page-update-current'),latest=$('#xray-page-update-latest'),note=$('#xray-page-update-note'),btn=$('#xray-page-do-update')
 if(!stateEl||!cur||!latest||!note||!btn)return
 if(error){stateEl.className='status bad';stateEl.innerHTML='<i></i>检查失败';latest.textContent='—';note.textContent=error;btn.disabled=true;btn.textContent='更新 Xray';return}
 if(!d){stateEl.className='status';stateEl.innerHTML='<i></i>检查中';note.textContent='与概览共用同一次检查结果，不重复请求。';return}
 cur.textContent=d.current?`v${String(d.current).replace(/^v/,'')}`:'未安装'
 latest.textContent=d.latest?`v${String(d.latest).replace(/^v/,'')}`:'—'
 stateEl.className=`status ${d.available?'':'ok'}`;stateEl.innerHTML=`<i></i>${d.available?'可更新':'已是最新'}`
 note.textContent=d.available?`发现${d.prerelease?'预发布':''}新版本 · ${d.asset}`:'当前已是最新版本'
 btn.disabled=!d.available;btn.textContent=d.available?'更新 Xray':'已是最新'
}

function ensureXrayRollbackButtons(){
 const overview=$('#do-update')?.parentElement
 if(overview&&!$('#rollback-xray'))overview.insertAdjacentHTML('beforeend','<button id="rollback-xray" class="btn ghost" disabled>暂无上一版</button>')
 buildXrayUpdatePanel()
 $('#rollback-xray')?.addEventListener('click',rollbackXray)
}

async function checkXrayRollback(){
 ensureXrayRollbackButtons()
 try{xrayRollback=await api('/api/xray/rollback')}catch{xrayRollback=null}
 for(const b of [$('#rollback-xray'),$('#xray-page-rollback')]){
  if(!b)continue
  b.disabled=!xrayRollback?.available
  b.textContent=xrayRollback?.available?`回退到 ${xrayRollback.previous}`:'暂无上一版'
 }
}

async function rollbackXray(){
 if(!xrayRollback?.available){await checkXrayRollback();if(!xrayRollback?.available)return}
 if(!confirm(`将 Xray 从 ${xrayRollback.current} 回退到 ${xrayRollback.previous}？\n旧 core 会先验证当前配置；失败自动保持当前版本。`))return
 const buttons=[$('#rollback-xray'),$('#xray-page-rollback')].filter(Boolean);buttons.forEach(b=>b.disabled=true)
 try{const d=await api('/api/xray/rollback',{method:'POST',body:'{}'});toast(`Xray 已切换到 ${d.current}`);await loadOverview();await checkUpdate();await checkXrayRollback()}catch(e){toast(e.message,true);await checkXrayRollback()}
}

async function checkUpdate(manual=false){
 const latest=$('#update-latest'),note=$('#update-note'),btn=$('#do-update');latest.textContent='检查中';btn.disabled=true;renderXrayUpdateState(null)
 try{
  const d=await api('/api/xray/update');state.update=d
  $('#update-current').textContent=d.current?`v${String(d.current).replace(/^v/,'')}`:'未安装';latest.textContent=d.latest?`v${String(d.latest).replace(/^v/,'')}`:'—';note.textContent=d.available?`发现${d.prerelease?'预发布':''}新版本 · ${d.asset}`:'当前已是最新版本';btn.disabled=!d.available;btn.textContent=d.available?'更新 Xray':'已是最新';renderXrayUpdateState(d);if(manual)toast(d.available?'发现可用更新':'Xray 已是最新')
 }catch(e){latest.textContent='检查失败';note.textContent=e.message;renderXrayUpdateState(null,e.message);if(manual)toast(e.message,true)}
 await checkXrayRollback()
}

async function updateXray(){
 if(!state.update?.available){await checkUpdate(true);if(!state.update?.available)return}
 if(!confirm(`将 Xray 从 ${state.update.current||'当前版本'} 更新到 ${state.update.latest}？\n更新失败自动恢复旧版本。`))return
 const buttons=[$('#do-update'),$('#xray-page-do-update')].filter(Boolean);buttons.forEach(b=>{b.disabled=true;b.textContent='更新中…'})
 try{const d=await api('/api/xray/update',{method:'POST',body:'{}'});state.update=d;toast(`Xray 已更新到 ${d.current}`);await loadOverview();await checkUpdate()}catch(e){toast(e.message,true);renderXrayUpdateState(state.update);const b=$('#do-update');if(b){b.disabled=false;b.textContent='重试更新'}}
}

async function importBackupFile(file){
 if(!file)return
 if(file.size>64*1024*1024){toast('备份文件不能超过 64 MiB',true);return}
 try{
  const res=await fetch('/api/backups/import',{method:'POST',credentials:'same-origin',headers:{'Content-Type':'application/gzip'},body:file})
  let d=null;try{d=await res.json()}catch{}
  if(!res.ok)throw new Error(d?.error||`HTTP ${res.status}`)
  toast('备份已导入；确认内容后再选择恢复');await loadBackups()
 }catch(e){toast(e.message,true)}finally{$('#backup-file').value=''}
}

async function restartXport(){
 if(!confirm('重启 X-port 管理面板？\n页面会短暂断开；Xray 代理核心不会因此停止。'))return
 try{await api('/api/xport/restart',{method:'POST',body:'{}'});toast('已安排重启，几秒后自动刷新');setTimeout(()=>location.reload(),3500)}catch(e){toast(e.message,true)}
}

function ensureSelfUpdateCard(){
 if($('#xport-self-update'))return
 const page=$('#page-system'),twoCol=page?.querySelector('.two-col');if(!page||!twoCol)return
 twoCol.insertAdjacentHTML('afterend',`<article id="xport-self-update" class="panel xport-update-panel"><div class="panel-head"><div><span class="kicker">X-PORT UPDATE</span><h2>面板更新</h2></div><span id="xport-update-state" class="status"><i></i>未检查</span></div><div class="version-line"><div><small>当前版本</small><b id="xport-update-current">${esc(state.overview?.xportVersion||'0.1.0-alpha')}</b></div><div><small>最新版本</small><b id="xport-update-latest">—</b></div></div><p id="xport-update-note" class="muted tiny">从 GitHub Release 更新 X-port，更新失败自动恢复旧版本。</p><div class="button-row"><button id="check-xport-update" class="btn ghost">检查 X-port</button><button id="do-xport-update" class="btn primary" disabled>更新 X-port</button></div></article>`)
 $('#check-xport-update').onclick=()=>checkXportUpdate(true)
 $('#do-xport-update').onclick=updateXport
}

async function checkXportUpdate(manual=false){
 ensureSelfUpdateCard();const cur=$('#xport-update-current'),latest=$('#xport-update-latest'),stateEl=$('#xport-update-state'),note=$('#xport-update-note'),btn=$('#do-xport-update');if(!btn)return
 latest.textContent='检查中';btn.disabled=true
 try{
  const d=await api('/api/xport/update');xportSelfUpdate=d;cur.textContent=d.current||'—';latest.textContent=d.latest||'—';stateEl.className=`status ${d.available?'':'ok'}`;stateEl.innerHTML=`<i></i>${d.available?'可更新':'已是最新'}`;note.textContent='从 GitHub Release 更新 X-port，更新失败自动恢复旧版本。';btn.disabled=!d.available;btn.textContent=d.available?'更新 X-port':'已是最新';if(manual)toast(d.available?'发现 X-port 更新':'X-port 已是最新')
 }catch(e){xportSelfUpdate=null;latest.textContent='暂不可用';stateEl.className='status';stateEl.innerHTML='<i></i>不可用';note.textContent=e.message;btn.disabled=true;btn.textContent='更新 X-port';if(manual)toast(e.message,true)}
}

async function updateXport(){
 if(!xportSelfUpdate?.available){await checkXportUpdate(true);if(!xportSelfUpdate?.available)return}
 if(!confirm(`将 X-port 从 ${xportSelfUpdate.current} 更新到 ${xportSelfUpdate.latest}？\n更新失败自动恢复旧版本。`))return
 const b=$('#do-xport-update');b.disabled=true;b.textContent='更新中…'
 try{const d=await api('/api/xport/update',{method:'POST',body:'{}'}),info=d.update||d;toast(d.scheduled?`X-port ${info.latest} 已安装，正在验证重启`:'X-port 已是最新');if(d.scheduled)setTimeout(()=>location.reload(),4500);else await checkXportUpdate()}catch(e){toast(e.message,true);b.disabled=false;b.textContent='重试更新'}
}

function bindMetricRings(){
 for(const id of ['cpu','memory','disk']){
  const el=$(`#${id}`);if(!el)continue
  const sync=()=>{const pct=Math.max(0,Math.min(100,Number.parseFloat(el.textContent)||0));el.style.setProperty('--metric-pct',`${pct}%`)}
  sync();new MutationObserver(sync).observe(el,{childList:true,subtree:true,characterData:true})
 }
}

function initThemeControl(){
 const foot=$('.side-foot');if(!foot)return
 foot.innerHTML=`<button id="theme-toggle" class="theme-toggle" type="button"><span class="theme-icon">☀</span><span class="theme-label">白天模式</span></button><div class="node-badge"><span class="pulse-dot"></span><span>PRIVATE NODE</span></div>`
 $('#theme-toggle').onclick=()=>applyTheme(document.documentElement.dataset.theme==='light'?'dark':'light')
 applyTheme(localStorage.getItem(xportThemeKey)||'dark')
}

async function enhanceAccountEditor(a){
 const form=$('#account-form');if(!form)return
 const protocol=$('#protocol-select')?.closest('label')
 if(protocol){protocol.classList.add('protocol-first','full');form.prepend(protocol);if(a?.id&&!protocol.querySelector('.field-hint'))protocol.insertAdjacentHTML('beforeend','<small class="field-hint">创建后不可修改</small>')}
 const proto=$('#proto-fields');if(!proto||$('#account-advanced'))return
 const details=document.createElement('details');details.id='account-advanced';details.className='advanced account-advanced full'
 details.innerHTML=`<summary><span>高级</span><small>Fallbacks · PROXY Protocol · HTTP 伪装</small></summary><div class="advanced-loading muted tiny">${a?.id?'正在读取高级配置…':'创建账号后可配置高级选项。'}</div>`
 proto.insertAdjacentElement('afterend',details)
 if(!a?.id)return
 try{
  const d=await api(`/api/accounts/${a.id}/advanced`)
  const fallback=d.supportsFallbacks?`<label class="full advanced-fallbacks">Fallbacks JSON<textarea id="advanced-fallbacks" spellcheck="false" placeholder='[{"dest":80}]'>${esc(d.fallbacksJson||'[]')}</textarea><small class="field-hint">仅 VLESS/Trojan + TCP/RAW + TLS 生效</small></label>`:''
  const httpHeader=d.supportsHttpHeader?`<label class="full advanced-http-header">HTTP 伪装 Header JSON<textarea id="advanced-http-header" spellcheck="false" placeholder='{"type":"http","request":{},"response":{}}'>${esc(d.httpHeaderJson||'')}</textarea><small class="field-hint">留空表示不使用；TCP/RAW 客户端需保持一致</small></label>`:''
  details.innerHTML=`<summary><span>高级</span><small>Fallbacks · PROXY Protocol · HTTP 伪装</small></summary><div class="advanced-body"><label class="checkline advanced-proxy"><input id="advanced-proxy" type="checkbox" ${d.acceptProxyProtocol?'checked':''}>接收 PROXY Protocol</label>${fallback}${httpHeader}<p class="muted tiny full">未列出的迁移参数继续原样保留；以后新增的 Xray 高级项也放在这里。</p><div class="advanced-actions full"><button id="save-advanced" type="button" class="btn ghost">保存高级设置</button></div></div>`
  $('#save-advanced').onclick=async()=>{
   const b=$('#save-advanced');b.disabled=true;b.textContent='应用中…'
   try{
    await api(`/api/accounts/${a.id}/advanced`,{method:'PUT',body:JSON.stringify({acceptProxyProtocol:$('#advanced-proxy')?.checked||false,fallbacksJson:$('#advanced-fallbacks')?.value||'',httpHeaderJson:$('#advanced-http-header')?.value||''})})
    toast('高级设置已应用');b.textContent='已保存';setTimeout(()=>{if(b.isConnected){b.disabled=false;b.textContent='保存高级设置'}},900)
   }catch(e){toast(e.message,true);b.disabled=false;b.textContent='保存高级设置'}
  }
 }catch(e){details.innerHTML=`<summary><span>高级</span></summary><p class="error tiny">${esc(e.message)}</p>`}
}

function polishStaticCopy(){
 const labels=$$('.summary-strip small');['总账号','已启用','总上传 / 下载','总用量'].forEach((v,i)=>{if(labels[i])labels[i].textContent=v})
 $('#page-accounts > p.muted.tiny')?.remove()
 $('#panel-mode-note')?.remove()
 const maintenance=$$('#page-system > article.panel').find(p=>p.querySelector('h2')?.textContent.trim()==='运维边界');maintenance?.remove()
 const unit=$('#uptime-days')?.nextElementSibling;if(unit)unit.textContent='天'
 buildXrayUpdatePanel();ensureXrayRollbackButtons();renderXrayUpdateState(state.update)
}

function bindAdvancedControls(){
 polishStaticCopy();initThemeControl()
 const baseOpen=openAccountModal;openAccountModal=async function(a=null){await baseOpen(a);await enhanceAccountEditor(a)}
 $('#export-accounts')?.addEventListener('click',exportAccounts)
 $('#check-geodata')?.addEventListener('click',()=>checkGeodata(true))
 $('#update-geodata')?.addEventListener('click',updateGeodata)
 $('#restart-xport')?.addEventListener('click',restartXport)
 $('#import-backup')?.addEventListener('click',()=>$('#backup-file')?.click())
 $('#backup-file')?.addEventListener('change',e=>importBackupFile(e.target.files?.[0]))
 const xrayNav=$('#nav button[data-page="xray"]');xrayNav?.addEventListener('click',()=>{renderXrayUpdateState(state.update);if(!xportGeodata)void checkGeodata(false);if(!xrayRollback)void checkXrayRollback()})
 const systemNav=$('#nav button[data-page="system"]');systemNav?.addEventListener('click',()=>{ensureSelfUpdateCard();if(!xportSelfUpdate)void checkXportUpdate(false)})
 bindMetricRings();ensureSelfUpdateCard();void checkXrayRollback()
}

document.addEventListener('DOMContentLoaded',bindAdvancedControls)