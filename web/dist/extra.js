let xportGeodata=null

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
 }catch(e){latest.textContent='检查失败';stateEl.className='status bad';stateEl.innerHTML='<i></i>检查失败';if(manual)toast(e.message,true)}
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

function bindAdvancedControls(){
 $('#export-accounts')?.addEventListener('click',exportAccounts)
 $('#check-geodata')?.addEventListener('click',()=>checkGeodata(true))
 $('#update-geodata')?.addEventListener('click',updateGeodata)
 $('#restart-xport')?.addEventListener('click',restartXport)
 $('#import-backup')?.addEventListener('click',()=>$('#backup-file')?.click())
 $('#backup-file')?.addEventListener('change',e=>importBackupFile(e.target.files?.[0]))
 const xrayNav=$('#nav button[data-page="xray"]');xrayNav?.addEventListener('click',()=>void checkGeodata(false))
}

document.addEventListener('DOMContentLoaded',bindAdvancedControls)
