let xportGeodata=null
let xportSelfUpdate=null

async function loadSettings(){
 try{
  const d=await api('/api/settings'),f=$('#settings-form')
  for(const [k,v] of Object.entries(d)){const el=f.elements.namedItem(k);if(el)el.value=v??''}
  f.elements.currentPassword.value='';f.elements.newPassword.value=''
  const note=$('#panel-mode-note');if(note){const scheme=d.panelTLS?'HTTPS':'HTTP',base=d.panelBasePath||'/';note.textContent=`当前面板：${scheme} · ${d.panelListen||'—'} · Base Path ${base}。监听、Base Path 或 HTTPS 变更后需要重启 X-port；防火墙仍由你手动放行。`}
 }catch(e){toast(e.message,true)}
}

async function saveSettings(e){
 e.preventDefault();const f=e.currentTarget
 const body={panelListen:formValue(f,'panelListen'),panelBasePath:formValue(f,'panelBasePath'),panelDomain:formValue(f,'panelDomain'),panelCertFile:formValue(f,'panelCertFile'),panelKeyFile:formValue(f,'panelKeyFile'),xrayApiPort:Number(formValue(f,'xrayApiPort')),portMin:Number(formValue(f,'portMin')),portMax:Number(formValue(f,'portMax')),defaultRealitySni:formValue(f,'defaultRealitySni'),defaultRealityDest:formValue(f,'defaultRealityDest'),adminUsername:formValue(f,'adminUsername'),currentPassword:formValue(f,'currentPassword'),newPassword:formValue(f,'newPassword')}
 try{const d=await api('/api/settings',{method:'PUT',body:JSON.stringify(body)});f.elements.currentPassword.value='';f.elements.newPassword.value='';await loadSettings();toast(d.restartRequired?'设置已保存；请检查新地址后重启 X-port 生效':'设置已保存')}catch(x){toast(x.message,true)}
}

function exportAccounts(){
 const host=prompt('批量导出使用哪个服务器域名 / IP？',location.hostname||'')
 if(host===null)return
 if(!host.trim()){toast('服务器域名 / IP 不能为空',true);return}
 location.href=`/api/accounts/export?host=${encodeURIComponent(host.trim())}`
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
 const maintenance=$('#page-system > article.panel:last-of-type');if(!maintenance)return
 maintenance.insertAdjacentHTML('beforeend',`<div id="xport-self-update" class="maintenance-update"><div class="panel-head"><div><span class="kicker">X-PORT UPDATE</span><h2>面板更新</h2></div><span id="xport-update-state" class="status"><i></i>未检查</span></div><div class="version-line"><div><small>当前版本</small><b id="xport-update-current">${esc(state.overview?.xportVersion||'0.1.0-alpha')}</b></div><div><small>最新版本</small><b id="xport-update-latest">—</b></div></div><p id="xport-update-note" class="muted tiny">私有仓库需要服务器环境变量 XPORT_GITHUB_TOKEN；token 不会写入数据库或显示在 WebUI。</p><div class="button-row"><button id="check-xport-update" class="btn ghost">检查 X-port</button><button id="do-xport-update" class="btn primary" disabled>更新 X-port</button></div></div>`)
 $('#check-xport-update').onclick=()=>checkXportUpdate(true)
 $('#do-xport-update').onclick=updateXport
}

async function checkXportUpdate(manual=false){
 ensureSelfUpdateCard();const cur=$('#xport-update-current'),latest=$('#xport-update-latest'),stateEl=$('#xport-update-state'),note=$('#xport-update-note'),btn=$('#do-xport-update');if(!btn)return
 latest.textContent='检查中';btn.disabled=true
 try{
  const d=await api('/api/xport/update');xportSelfUpdate=d;cur.textContent=d.current||'—';latest.textContent=d.latest||'—';stateEl.className=`status ${d.available?'':'ok'}`;stateEl.innerHTML=`<i></i>${d.available?'可更新':'已是最新'}`;note.textContent=d.authMode==='token'?'通过服务器环境 token 检查私有 Release；token 不进入 WebUI。':'使用公开 GitHub Release 通道。';btn.disabled=!d.available;btn.textContent=d.available?'更新 X-port':'已是最新';if(manual)toast(d.available?'发现 X-port 更新':'X-port 已是最新')
 }catch(e){xportSelfUpdate=null;latest.textContent='暂不可用';stateEl.className='status';stateEl.innerHTML='<i></i>不可用';note.textContent=e.message;btn.disabled=true;btn.textContent='更新 X-port';if(manual)toast(e.message,true)}
}

async function updateXport(){
 if(!xportSelfUpdate?.available){await checkXportUpdate(true);if(!xportSelfUpdate?.available)return}
 if(!confirm(`将 X-port 从 ${xportSelfUpdate.current} 更新到 ${xportSelfUpdate.latest}？\n候选二进制会校验 SHA-256 和版本；新服务启动失败自动恢复旧版。`))return
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

function bindAdvancedControls(){
 $('#export-accounts')?.addEventListener('click',exportAccounts)
 $('#check-geodata')?.addEventListener('click',()=>checkGeodata(true))
 $('#update-geodata')?.addEventListener('click',updateGeodata)
 $('#restart-xport')?.addEventListener('click',restartXport)
 $('#import-backup')?.addEventListener('click',()=>$('#backup-file')?.click())
 $('#backup-file')?.addEventListener('change',e=>importBackupFile(e.target.files?.[0]))
 const xrayNav=$('#nav button[data-page="xray"]');xrayNav?.addEventListener('click',()=>void checkGeodata(false))
 const systemNav=$('#nav button[data-page="system"]');systemNav?.addEventListener('click',()=>{ensureSelfUpdateCard();void checkXportUpdate(false)})
 bindMetricRings()
 ensureSelfUpdateCard()
}

document.addEventListener('DOMContentLoaded',bindAdvancedControls)