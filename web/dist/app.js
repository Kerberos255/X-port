const $ = (s, root=document) => root.querySelector(s)
const $$ = (s, root=document) => [...root.querySelectorAll(s)]
const state = { accounts: [], overview: null, update: null, networkSample: null, page: 'overview' }

function fmtBytes(n, rate=false){
  n = Number(n||0); const units=['B','KB','MB','GB','TB']; let i=0
  while(n>=1024 && i<units.length-1){n/=1024;i++}
  const digits = n>=100 || i===0 ? 0 : n>=10 ? 1 : 2
  return `${n.toFixed(digits)} ${units[i]}${rate?'/s':''}`
}
function fmtPercent(used,total){ return total ? Math.round(used/total*100) : 0 }
function fmtUptime(sec){ sec=Math.max(0,Number(sec||0)); const d=Math.floor(sec/86400), h=Math.floor(sec%86400/3600), m=Math.floor(sec%3600/60); return `${d} 天 ${h} 小时 ${m} 分` }
function esc(s){ return String(s??'').replace(/[&<>'"]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;',"'":'&#39;','"':'&quot;'}[c])) }
function toast(msg, bad=false){ const el=$('#toast'); el.textContent=msg; el.style.borderColor=bad?'rgba(238,124,137,.3)':''; el.classList.remove('hidden'); clearTimeout(toast.t); toast.t=setTimeout(()=>el.classList.add('hidden'),2600) }

async function api(path, options={}){
  const opts={credentials:'same-origin',...options,headers:{'Content-Type':'application/json',...(options.headers||{})}}
  const res=await fetch(path,opts)
  let data=null; const ct=res.headers.get('content-type')||''
  if(ct.includes('application/json')){ try{data=await res.json()}catch{} }
  if(!res.ok){ const err=new Error(data?.error||`HTTP ${res.status}`); err.status=res.status; throw err }
  return data
}

async function boot(){
  bind()
  try{ await loadOverview(); showApp() }catch(e){ if(e.status===401) showLogin(); else { showLogin(); $('#login-error').textContent=e.message; $('#login-error').classList.remove('hidden') } }
}
function showLogin(){ $('#login').classList.remove('hidden'); $('#app').classList.add('hidden'); setTimeout(()=>$('#login-pass').focus(),20) }
function showApp(){ $('#login').classList.add('hidden'); $('#app').classList.remove('hidden'); loadAccounts(); checkUpdate(); }

function bind(){
  $('#login-form').addEventListener('submit',async e=>{e.preventDefault();const err=$('#login-error');err.classList.add('hidden');try{await api('/api/login',{method:'POST',body:JSON.stringify({username:$('#login-user').value,password:$('#login-pass').value})});$('#login-pass').value='';await loadOverview();showApp()}catch(ex){err.textContent=ex.message;err.classList.remove('hidden')}})
  $$('#nav button').forEach(b=>b.addEventListener('click',()=>switchPage(b.dataset.page)))
  $('#refresh-overview').addEventListener('click',()=>loadOverview(true))
  $('#add-account').addEventListener('click',()=>openAccountModal())
  $('#modal-close').addEventListener('click',closeModal)
  $('#modal').addEventListener('click',e=>{if(e.target.id==='modal')closeModal()})
  $('#check-update').addEventListener('click',()=>checkUpdate(true))
  $('#do-update').addEventListener('click',updateXray)
  $('#xray-page-update').addEventListener('click',async()=>{await checkUpdate(true);if(state.update?.available)await updateXray()})
  setInterval(()=>{if(!$('#app').classList.contains('hidden')) loadOverview(false,true)},4000)
}
function switchPage(page){state.page=page;$$('#nav button').forEach(b=>b.classList.toggle('active',b.dataset.page===page));$$('.page').forEach(p=>p.classList.toggle('active',p.id===`page-${page}`));if(page==='accounts')loadAccounts();if(page==='overview')loadOverview();if(page==='xray')loadOverview()}

async function loadOverview(manual=false,silent=false){
  try{
    const data=await api('/api/overview'); const now=Date.now(); const sys=data.system||{}; const prev=state.networkSample
    state.overview=data; state.networkSample={t:now,rx:Number(sys.networkRx||0),tx:Number(sys.networkTx||0)}
    $('#hostname').textContent=sys.hostname||'Linux node'; $('#uptime-days').textContent=Math.floor(Number(sys.uptimeSeconds||0)/86400); $('#uptime-text').textContent=fmtUptime(sys.uptimeSeconds)
    $('#server-os').textContent=`${sys.os||'Linux'} · ${data.accounts?.enabled||0} 个独立监听端口`
    $('#cpu').textContent=`${Number(sys.cpuPercent||0).toFixed(1)}%`; $('#load').textContent=`load ${Number(sys.load1||0).toFixed(2)}`
    const mp=fmtPercent(sys.memoryUsed,sys.memoryTotal), dp=fmtPercent(sys.diskUsed,sys.diskTotal); $('#memory').textContent=`${mp}%`;$('#memory-detail').textContent=`${fmtBytes(sys.memoryUsed)} / ${fmtBytes(sys.memoryTotal)}`;$('#disk').textContent=`${dp}%`;$('#disk-detail').textContent=`${fmtBytes(sys.diskUsed)} / ${fmtBytes(sys.diskTotal)}`;$('#listeners').textContent=data.accounts?.enabled??0
    $('#xray-version').textContent=sys.xrayVersion?`v${String(sys.xrayVersion).replace(/^v/,'')}`:'—';$('#xport-version').textContent=data.xportVersion||'—';$('#xray-desc').textContent=`代理核心 · ${data.accounts?.enabled||0} 个 inbound`
    setServiceStatus(sys.xrayActive)
    if(prev){const sec=Math.max(.2,(now-prev.t)/1000);const rx=Math.max(0,(Number(sys.networkRx||0)-prev.rx)/sec),tx=Math.max(0,(Number(sys.networkTx||0)-prev.tx)/sec);$('#rx-rate').textContent=fmtBytes(rx,true);$('#tx-rate').textContent=fmtBytes(tx,true);$('#traffic-total').textContent=fmtBytes(rx+tx,true)}
    if(manual)toast('服务器状态已刷新')
  }catch(e){if(e.status===401){showLogin();return}if(!silent)toast(e.message,true);throw e}
}
function setServiceStatus(active){const a=$('#xray-status'),h=$('#health-xray');a.className=`status ${active?'ok':'bad'}`;a.innerHTML=`<i></i>${active?'运行中':'异常'}`;h.className=active?'ok-text':'bad-text';h.textContent=`● ${active?'正常':'异常'}`;$('#health-listeners').textContent=`${state.overview?.accounts?.enabled||0} / ${state.overview?.accounts?.total||0}`;$('#health-listeners').className='ok-text';$('#xray-page-status').textContent=active?'运行中':'异常';$('#xray-page-version').textContent=state.overview?.system?.xrayVersion?`Xray ${state.overview.system.xrayVersion}`:'版本未知';$('#xray-page-dot').style.background=active?'var(--green)':'var(--red)'}

async function loadAccounts(){
  try{const d=await api('/api/accounts');state.accounts=d.accounts||[];renderAccounts()}catch(e){if(e.status===401)showLogin();else toast(e.message,true)}
}
function renderAccounts(){
  const list=$('#accounts-list'); const arr=state.accounts; $('#sum-total').textContent=arr.length;$('#sum-active').textContent=arr.filter(a=>a.enabled).length;$('#sum-traffic').textContent=fmtBytes(arr.reduce((n,a)=>n+Number(a.upBytes||0)+Number(a.downBytes||0),0));
  if(!arr.length){list.innerHTML='<div class="empty"><b>还没有账号</b><span>新增第一个独立端口账号，或通过迁移脚本导入 X-Panel / 3x-ui。</span></div>';return}
  list.innerHTML=arr.map(a=>{const used=Number(a.upBytes||0)+Number(a.downBytes||0),quota=Number(a.quotaBytes||0),pct=quota?Math.min(100,Math.round(used/quota*100)):0;return `<article class="account-row" data-id="${a.id}">
    <div class="account-name"><i class="state ${a.enabled?'on':''}"></i><div><b>${esc(a.name)}</b><small>${a.enabled?'运行中':'已停用'} · ID ${a.id}</small></div></div>
    <div class="port-cell"><span class="tag">:${a.port}</span></div>
    <div class="protocol-cell hide-mid"><span class="tag">${esc((a.protocol||'').toUpperCase())}${a.security?` · ${esc(a.security.toUpperCase())}`:''}</span></div>
    <div class="usage-cell hide-mid"><small>流量 ${quota?`/ ${fmtBytes(quota)}`:'· 不限'}</small><b>${fmtBytes(used)}</b><div class="mini-bar"><i style="width:${pct}%"></i></div></div>
    <div class="hide-mid"><small class="muted tiny">${a.expiryTime?new Date(a.expiryTime).toLocaleDateString():'永久'}</small></div>
    <div class="row-actions"><button class="icon-btn" data-act="share" title="分享">⌁</button><button class="icon-btn" data-act="clone" title="克隆">⧉</button><button class="icon-btn" data-act="edit" title="编辑">✎</button><button class="icon-btn" data-act="delete" title="删除">×</button></div>
  </article>`}).join('')
  $$('.account-row').forEach(row=>row.addEventListener('click',e=>{const b=e.target.closest('button[data-act]');if(!b)return;const id=Number(row.dataset.id);if(b.dataset.act==='edit')openAccountModal(state.accounts.find(a=>a.id===id));if(b.dataset.act==='clone')openCloneModal(id);if(b.dataset.act==='delete')deleteAccount(id);if(b.dataset.act==='share')openShareModal(id)}))
}

function openModal(html){$('#modal-body').innerHTML=html;$('#modal').classList.remove('hidden')}
function closeModal(){$('#modal').classList.add('hidden');$('#modal-body').innerHTML=''}
function accountFormHTML(a={}){
  const expiry=a.expiryTime?new Date(Number(a.expiryTime)-new Date().getTimezoneOffset()*60000).toISOString().slice(0,16):'';const qgb=a.quotaBytes?(Number(a.quotaBytes)/1073741824).toFixed(2):'';const isVless=!a.id||String(a.protocol).toLowerCase()==='vless'
  const advanced=isVless?`<label>REALITY SNI<input name="serverName" value="${esc(a.serverName||'')}" placeholder="例如 www.microsoft.com" required></label><label>目标地址<input name="dest" value="${esc(a.dest||'')}" placeholder="留空自动使用 SNI:443"></label>
<details class="advanced"><summary>高级参数</summary><div class="advanced-grid"><label>UUID<input name="credential" value="${esc(a.credential||'')}" placeholder="留空自动生成"></label><label>Flow<input name="flow" value="${esc(a.flow||'xtls-rprx-vision')}"></label><label>Network<select name="network"><option value="tcp" ${(a.network||'tcp')==='tcp'?'selected':''}>tcp</option><option value="raw" ${a.network==='raw'?'selected':''}>raw</option></select></label><label>Security<select name="security"><option value="reality" ${(a.security||'reality')==='reality'?'selected':''}>reality</option><option value="none" ${a.security==='none'?'selected':''}>none</option></select></label><label>REALITY Private Key<input name="privateKey" value="${esc(a.privateKey||'')}" placeholder="留空自动生成"></label><label>Short ID<input name="shortId" value="${esc(a.shortId||'')}" placeholder="留空自动生成"></label></div></details>`:`<p class="muted tiny full">该账号是从旧面板导入的 ${esc(String(a.protocol||'').toUpperCase())} 配置。当前仅编辑名称、端口、启停、流量与到期时间；底层协议配置保持原样。</p>`
  return `<div class="modal-title"><span>${a.id?'EDIT ACCOUNT':'NEW ACCOUNT'}</span><h2>${a.id?'编辑账号':'新增账号'}</h2></div><form id="account-form" class="form-grid">
<label>账号名<input name="name" value="${esc(a.name||'')}" required maxlength="80"></label><label>独立端口<input name="port" type="number" min="1" max="65535" value="${a.port||''}" placeholder="留空自动分配"></label>
<label>协议<select name="protocol" disabled><option value="${esc(a.protocol||'vless')}">${esc(String(a.protocol||'vless').toUpperCase())}</option></select></label><label>流量上限 / GB<input name="quota" type="number" min="0" step="0.1" value="${qgb}" placeholder="0 = 不限"></label>
<label>到期时间<input name="expiry" type="datetime-local" value="${expiry}"></label><label class="checkline"><input name="enabled" type="checkbox" ${a.id?(a.enabled?'checked':''):'checked'}>启用账号</label>${advanced}
<div class="form-actions full"><button type="button" class="btn ghost" id="cancel-form">取消</button><button type="submit" class="btn primary">${a.id?'保存并应用':'创建并应用'}</button></div></form>`}
async function openAccountModal(a=null){
  let full=a;if(a?.id){try{full=await api(`/api/accounts/${a.id}`)}catch(e){toast(e.message,true);return}}
  openModal(accountFormHTML(full||{}));$('#cancel-form').onclick=closeModal;$('#account-form').onsubmit=async e=>{e.preventDefault();const f=new FormData(e.currentTarget);const body={name:f.get('name'),enabled:f.get('enabled')==='on',port:Number(f.get('port')||0),protocol:full?.protocol||'vless',credential:f.get('credential'),flow:f.get('flow'),network:f.get('network'),security:f.get('security'),serverName:f.get('serverName'),dest:f.get('dest'),privateKey:f.get('privateKey'),shortId:f.get('shortId'),quotaBytes:Math.round(Number(f.get('quota')||0)*1073741824),expiryTime:f.get('expiry')?new Date(f.get('expiry')).getTime():0};const submit=e.currentTarget.querySelector('button[type=submit]');submit.disabled=true;submit.textContent='应用中…';try{if(full?.id)await api(`/api/accounts/${full.id}`,{method:'PUT',body:JSON.stringify(body)});else await api('/api/accounts',{method:'POST',body:JSON.stringify(body)});closeModal();await loadAccounts();await loadOverview();toast(full?.id?'账号已更新':'账号已创建')}catch(ex){toast(ex.message,true);submit.disabled=false;submit.textContent=full?.id?'保存并应用':'创建并应用'}}
}
function openCloneModal(id){const a=state.accounts.find(x=>x.id===id);openModal(`<div class="modal-title"><span>CLONE ACCOUNT</span><h2>克隆 ${esc(a?.name||'账号')}</h2></div><form id="clone-form" class="form-grid"><label>新账号名<input name="name" value="${esc((a?.name||'account')+' copy')}" required></label><label>新端口<input name="port" type="number" min="1" max="65535" placeholder="留空自动分配"></label><p class="muted tiny full">协议与 REALITY 参数会复制；UUID 自动重新生成，流量清零。</p><div class="form-actions full"><button type="button" class="btn ghost" id="clone-cancel">取消</button><button class="btn primary" type="submit">克隆并应用</button></div></form>`);$('#clone-cancel').onclick=closeModal;$('#clone-form').onsubmit=async e=>{e.preventDefault();const f=new FormData(e.currentTarget);try{await api(`/api/accounts/${id}/clone`,{method:'POST',body:JSON.stringify({name:f.get('name'),port:Number(f.get('port')||0)})});closeModal();await loadAccounts();await loadOverview();toast('账号已克隆')}catch(ex){toast(ex.message,true)}}}
async function deleteAccount(id){const a=state.accounts.find(x=>x.id===id);if(!confirm(`删除账号「${a?.name||id}」？\n对应 inbound 会从 Xray 配置移除。`))return;try{await api(`/api/accounts/${id}`,{method:'DELETE'});await loadAccounts();await loadOverview();toast('账号已删除')}catch(e){toast(e.message,true)}}
async function openShareModal(id){const a=state.accounts.find(x=>x.id===id);const defaultHost=location.hostname;openModal(`<div class="modal-title"><span>SHARE</span><h2>${esc(a?.name||'账号')} · 分享</h2></div><div class="share-wrap"><div class="qr-box"><img id="share-qr" alt="QR code"></div><div class="share-side"><label>服务器域名 / IP<input id="share-host" value="${esc(defaultHost)}"></label><label>VLESS 链接<textarea id="share-link" class="share-link" readonly></textarea></label><div class="button-row"><button id="copy-share" class="btn primary">复制链接</button><button id="reload-share" class="btn ghost">刷新二维码</button></div></div></div>`);async function refresh(){const host=$('#share-host').value.trim();try{const d=await api(`/api/accounts/${id}/share?host=${encodeURIComponent(host)}`);$('#share-link').value=d.uri;$('#share-qr').src=`/api/accounts/${id}/qr?host=${encodeURIComponent(host)}&t=${Date.now()}`}catch(e){toast(e.message,true)}};$('#reload-share').onclick=refresh;$('#share-host').addEventListener('change',refresh);$('#copy-share').onclick=async()=>{await navigator.clipboard.writeText($('#share-link').value);toast('分享链接已复制')};await refresh()}

async function checkUpdate(manual=false){
  const latest=$('#update-latest'),note=$('#update-note'),btn=$('#do-update'); latest.textContent='检查中';btn.disabled=true
  try{const d=await api('/api/xray/update');state.update=d;$('#update-current').textContent=d.current?`v${String(d.current).replace(/^v/,'')}`:'未安装';latest.textContent=d.latest?`v${String(d.latest).replace(/^v/,'')}`:'—';note.textContent=d.available?`发现${d.prerelease?'预发布':''}新版本 · ${d.asset}`:'当前已是最新版本';btn.disabled=!d.available;btn.textContent=d.available?'更新 Xray':'已是最新';if(manual)toast(d.available?'发现可用更新':'Xray 已是最新')}catch(e){latest.textContent='检查失败';note.textContent=e.message;if(manual)toast(e.message,true)}
}
async function updateXray(){if(!state.update?.available){await checkUpdate(true);if(!state.update?.available)return}if(!confirm(`将 Xray 从 ${state.update.current||'当前版本'} 更新到 ${state.update.latest}。\n新二进制会先校验当前配置；失败会自动回滚。继续吗？`))return;const btn=$('#do-update');btn.disabled=true;btn.textContent='更新中…';try{const d=await api('/api/xray/update',{method:'POST',body:'{}'});state.update=d;toast(`Xray 已更新到 ${d.current}`);await loadOverview();await checkUpdate()}catch(e){toast(e.message,true);btn.disabled=false;btn.textContent='重试更新'}}

document.addEventListener('DOMContentLoaded',boot)
