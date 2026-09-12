let xportOnlineRawConnectionsV4={}

// v0.1.1 displayed raw ESTABLISHED TCP sockets as "online", which made a
// single XHTTP client look like many devices. Prefer unique remote IPs as a
// practical client estimate, while retaining the raw socket count in tooltip.
if(typeof loadOnlineConnectionsV3==='function'){
 loadOnlineConnectionsV3=async function(render=true){
  if(state.page!=='accounts')return
  try{
   const d=await api('/api/accounts/online')
   xportOnlineConnections=d.peers||d.connections||{}
   xportOnlineRawConnectionsV4=d.connections||{}
   if(render)renderAccountsV3()
  }catch{}
 }
}

if(typeof onlinePillV3==='function'){
 onlinePillV3=function(a){
  const id=String(a.id),peers=Number(xportOnlineConnections[id]??0),connections=Number(xportOnlineRawConnectionsV4[id]??0)
  const title=peers>0
   ?`按 ESTABLISHED TCP 的远端 IP 去重，约 ${peers} 个活跃客户端；当前 ${connections} 条 TCP 连接。相同 NAT 下多个设备可能合并，UDP 不计入。`
   :'当前没有检测到 ESTABLISHED TCP 客户端；UDP 不计入在线判断。'
  return `<span class="online-pill ${peers>0?'on':''}" title="${esc(title)}"><i></i>${peers>0?`在线 ${peers}`:'离线'}</span>`
 }
}

function ensureThemeControlV4(){
 if(!$('#theme-toggle')&&typeof initThemeControl==='function')initThemeControl()
 const b=$('#theme-toggle')
 if(b)b.setAttribute('aria-label',b.title||'切换白天/夜间模式')
}

function polishAccountCopyV4(){
 const sum=$('#sum-monthly')?.closest('div')?.querySelector('small')
 if(sum)sum.textContent='累计流量'
 const list=$('#accounts-list')
 if(list&&!$('#online-note')){
  const note=document.createElement('p')
  note.id='online-note';note.className='online-note'
  note.textContent='在线数按当前 ESTABLISHED TCP 的远端 IP 去重，是设备数近似值；同一 NAT 下多个设备可能合并，UDP 不计入。'
  list.insertAdjacentElement('afterend',note)
 }
}

document.addEventListener('DOMContentLoaded',()=>{
 ensureThemeControlV4()
 polishAccountCopyV4()
})
