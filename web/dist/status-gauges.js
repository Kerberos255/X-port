/* X-port overview status: four SVG gauges, existing 10s polling only. */
(function () {
 'use strict';
 const metrics=[
  {key:'load',label:'负载',valueId:'status-load-value',hintId:'status-load-hint'},
  {key:'cpu',label:'CPU',valueId:'cpu',hintId:'status-cpu-hint'},
  {key:'memory',label:'内存',valueId:'memory',hintId:'memory-detail'},
  {key:'disk',label:'磁盘',valueId:'disk',hintId:'disk-detail'}
 ];
 const circumference=2*Math.PI*41;
 const units=['B','KB','MB','GB','TB','PB'];
 const fmt=n=>Number.isFinite(n)?n.toFixed(1):'—';
 function bytes(n){
  if(!Number.isFinite(n)||n<0)return '—';
  let i=0;while(n>=1024&&i<units.length-1){n/=1024;i++}
  return n.toFixed(i===0?0:n>=10?1:2)+' '+units[i];
 }
 const safe=n=>Number.isFinite(n)&&n>=0?n:NaN;
 const percent=(used,total)=>total>0?used/total*100:NaN;
 let root;
 function closePinned(except){
  root?.querySelectorAll('.status-gauge-card[data-pinned="true"]').forEach(el=>{
   if(el===except)return;
   el.dataset.pinned='false';el.setAttribute('aria-expanded','false');
  });
 }
 function init(){
  root=document.querySelector('#page-overview .metrics');
  if(!root||root.dataset.statusGauges==='ready')return Boolean(root);
  root.dataset.statusGauges='ready';
  root.classList.add('status-gauges');
  root.innerHTML=metrics.map(({key,label,valueId,hintId})=>`
   <button class="status-gauge-card" type="button" data-gauge="${key}" data-pinned="false" aria-expanded="false" aria-label="查看${label}详细信息">
    <span class="status-gauge-title">${label}</span>
    <span class="status-gauge-ring">
     <svg class="status-gauge-svg" viewBox="0 0 100 100" aria-hidden="true">
      <circle class="status-gauge-track" cx="50" cy="50" r="41"></circle>
      <circle class="status-gauge-progress" cx="50" cy="50" r="41" stroke-dasharray="0 ${circumference.toFixed(2)}"></circle>
     </svg>
     <strong id="${valueId}" class="status-gauge-number">—</strong>
     <span class="status-gauge-inner-label">${label}</span>
    </span>
    <span id="${hintId}" class="status-gauge-hint">—</span>
    <span class="status-gauge-popover" role="tooltip" aria-label="${label}详情">
     <span class="status-gauge-popover-title">${label}详情</span>
     <span class="status-gauge-rows"></span>
    </span>
   </button>`).join('')+'<span id="load" hidden></span><span id="listeners" hidden></span>';
  root.querySelectorAll('.status-gauge-card').forEach(card=>{
   card.addEventListener('click',()=>{
    const next=card.dataset.pinned!=='true';
    closePinned(card);card.dataset.pinned=String(next);card.setAttribute('aria-expanded',String(next));
   });
  });
  document.addEventListener('pointerdown',e=>{if(!root.contains(e.target))closePinned()});
  document.addEventListener('keydown',e=>{
   if(e.key==='Escape'){closePinned();root.querySelector(':focus')?.blur()}
  });
  return true;
 }
 function draw(key,value,hint,rows){
  const card=root.querySelector('[data-gauge="'+key+'"]');if(!card)return;
  const fraction=Number.isFinite(value)?Math.min(100,Math.max(0,value))/100:0;
  card.querySelector('.status-gauge-progress').style.strokeDasharray=(circumference*fraction).toFixed(2)+' '+circumference.toFixed(2);
  card.querySelector('.status-gauge-number').textContent=Number.isFinite(value)?fmt(value)+'%':'—';
  card.querySelector('.status-gauge-hint').textContent=hint;
  const lines=card.querySelector('.status-gauge-rows');
  if(lines.children.length!==rows.length){
   lines.replaceChildren(...rows.map(()=>{
    const row=document.createElement('span');row.className='status-gauge-row';
    row.append(document.createElement('span'),document.createElement('b'));return row;
   }));
  }
  rows.forEach(([label,val],i)=>{
   const el=lines.children[i];
   if(el.children[0].textContent!==label)el.children[0].textContent=label;
   if(el.children[1].textContent!==val)el.children[1].textContent=val;
  });
 }
 window.renderStatusGauges=function renderStatusGauges(sys={}){
  if(!init())return;
  const cores=safe(Number(sys.cpuCores))||1;
  const load1=safe(Number(sys.load1)),load5=safe(Number(sys.load5)),load15=safe(Number(sys.load15));
  const cpu=safe(Number(sys.cpuPercent)),memUsed=safe(Number(sys.memoryUsed)),memTotal=safe(Number(sys.memoryTotal));
  const diskUsed=safe(Number(sys.diskUsed)),diskTotal=safe(Number(sys.diskTotal));
  draw('load',load1/cores*100,'1 分钟 '+fmt(load1)+' / '+cores+' 核',[
   ['1 分钟平均负载',fmt(load1)],['5 分钟平均负载',fmt(load5)],
   ['15 分钟平均负载',fmt(load15)],['逻辑核心',String(cores)]
  ]);
  draw('cpu',cpu,cores+' 个逻辑核心',[
   ['CPU 使用率',fmt(cpu)+'%'],['CPU 空闲',fmt(Number.isFinite(cpu)?Math.max(0,100-cpu):NaN)+'%'],
   ['逻辑核心',String(cores)],['采样间隔','120 ms']
  ]);
  const memPct=percent(memUsed,memTotal);
  draw('memory',memPct,bytes(memUsed)+' / '+bytes(memTotal),[
   ['已使用',bytes(memUsed)],['可用（估算）',bytes(Math.max(0,memTotal-memUsed))],
   ['总内存',bytes(memTotal)],['使用率',fmt(memPct)+'%']
  ]);
  draw('disk',percent(diskUsed,diskTotal),bytes(diskUsed)+' / '+bytes(diskTotal),[
   ['挂载点','/'],['已使用',bytes(diskUsed)],['可用',bytes(Math.max(0,diskTotal-diskUsed))],
   ['总容量',bytes(diskTotal)]
  ]);
 };
 if(typeof loadOverview==='function'){
  const original=loadOverview;
  loadOverview=async function(...args){
   const result=await original.apply(this,args);
   if(typeof state!=='undefined'&&state.overview?.system)window.renderStatusGauges(state.overview.system);
   return result;
  };
 }
})();
