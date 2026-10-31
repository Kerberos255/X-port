/* X-port overview: native SVG gauges with one body-level detail tooltip.
   Existing /api/overview polling remains unchanged; no extra requests. */
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
 const finite=n=>Number.isFinite(n)&&n>=0?n:NaN;
 const percent=(used,total)=>total>0?used/total*100:NaN;
 function bytes(n){
  if(!Number.isFinite(n)||n<0)return '—';
  let i=0;while(n>=1024&&i<units.length-1){n/=1024;i++}
  return n.toFixed(i===0?0:n>=10?1:2)+' '+units[i];
 }
 let root,tooltip,activeRing=null,pinnedRing=null;
 const rows=new Map();
 function hide(){
  if(tooltip)tooltip.hidden=true;
  if(activeRing)activeRing.setAttribute('aria-expanded','false');
  activeRing=null;
 }
 function unpin(){
  pinnedRing=null;
  root?.querySelectorAll('.status-gauge-ring[aria-expanded="true"]').forEach(r=>r.setAttribute('aria-expanded','false'));
  hide();
 }
 function position(){
  if(!tooltip||!activeRing||tooltip.hidden)return;
  if(!document.querySelector('#page-overview')?.classList.contains('active')){unpin();return}
  const rect=activeRing.getBoundingClientRect(),vw=window.innerWidth,vh=window.innerHeight;
  // A programmatic focus or small viewport may place the ring off-screen.
  // Clamp the tooltip to the viewport instead of covering adjacent cards.
  tooltip.style.maxHeight=Math.max(100,vh-16)+'px';
  if(vw<=480){
   tooltip.style.width=Math.max(180,vw-24)+'px';
   tooltip.style.left='12px';
   tooltip.style.top='auto';
   tooltip.style.bottom='max(92px, env(safe-area-inset-bottom))';
   return;
  }
  tooltip.style.width='236px';
  tooltip.style.bottom='auto';
  const width=tooltip.getBoundingClientRect().width;
  tooltip.style.left=Math.max(10,Math.min(rect.left+rect.width/2-width/2,vw-width-10))+'px';
  const height=tooltip.getBoundingClientRect().height,below=rect.bottom+8;
  const preferred=below+height+10<=vh?below:rect.top-height-8;
  tooltip.style.top=Math.max(8,Math.min(preferred,vh-height-8))+'px';
 }
 function paintTooltip(key){
  if(!tooltip)return;
  const metric=metrics.find(m=>m.key===key);
  tooltip.querySelector('.status-gauge-popover-title').textContent=metric.label+'详情';
  const body=tooltip.querySelector('.status-gauge-rows');
  body.replaceChildren(...(rows.get(key)||[]).map(([name,value])=>{
   const line=document.createElement('div'),label=document.createElement('span'),number=document.createElement('b');
   line.className='status-gauge-row';label.textContent=name;number.textContent=value;
   line.append(label,number);return line;
  }));
 }
 function show(ring){
  if(!tooltip||!ring)return;
  if(activeRing&&activeRing!==ring)activeRing.setAttribute('aria-expanded','false');
  activeRing=ring;ring.setAttribute('aria-expanded','true');
  paintTooltip(ring.closest('.status-gauge-card').dataset.gauge);
  tooltip.hidden=false;position();
 }
 function init(){
  root=document.querySelector('#page-overview .metrics');
  if(!root||root.dataset.statusGauges==='ready')return Boolean(root);
  root.dataset.statusGauges='ready';root.classList.add('status-gauges');
  root.innerHTML=metrics.map(({key,label,valueId,hintId})=>`
   <div class="status-gauge-card" data-gauge="${key}">
    <span class="status-gauge-title">${label}</span>
    <button type="button" class="status-gauge-ring" aria-expanded="false" aria-label="查看${label}详情">
     <svg class="status-gauge-svg" viewBox="0 0 100 100" aria-hidden="true">
      <circle class="status-gauge-track" cx="50" cy="50" r="41"></circle>
      <circle class="status-gauge-progress" cx="50" cy="50" r="41" stroke-dasharray="0 ${circumference.toFixed(2)}"></circle>
     </svg>
     <strong id="${valueId}" class="status-gauge-number">—</strong>
     <span class="status-gauge-inner-label">${label}</span>
    </button>
    <span id="${hintId}" class="status-gauge-hint">—</span>
   </div>`).join('')+'<span id="load" hidden></span><span id="listeners" hidden></span>';
  tooltip=document.createElement('div');tooltip.className='status-gauge-floating';
  tooltip.setAttribute('role','tooltip');tooltip.hidden=true;
  const title=document.createElement('strong'),body=document.createElement('div');
  title.className='status-gauge-popover-title';body.className='status-gauge-rows';
  tooltip.append(title,body);document.body.appendChild(tooltip);
  root.querySelectorAll('.status-gauge-ring').forEach(ring=>{
   ring.addEventListener('pointerenter',e=>{if(e.pointerType==='mouse'||e.pointerType==='pen')show(ring)});
   ring.addEventListener('pointerleave',()=>{if(pinnedRing!==ring){if(pinnedRing)show(pinnedRing);else hide()}});
   ring.addEventListener('focus',()=>show(ring));
   ring.addEventListener('blur',()=>{if(pinnedRing!==ring){if(pinnedRing)show(pinnedRing);else hide()}});
   ring.addEventListener('click',()=>{
    if(pinnedRing===ring){unpin();return}
    pinnedRing=ring;show(ring);
   });
  });
  document.addEventListener('pointerdown',e=>{if(!e.target.closest('.status-gauge-ring'))unpin()});
  document.addEventListener('keydown',e=>{if(e.key==='Escape')unpin()});
  window.addEventListener('resize',position,{passive:true});
  window.addEventListener('scroll',position,{passive:true,capture:true});
  return true;
 }
 function draw(key,value,hint,detail){
  const card=root.querySelector('[data-gauge="'+key+'"]');if(!card)return;
  const frac=Number.isFinite(value)?Math.min(100,Math.max(0,value))/100:0;
  card.querySelector('.status-gauge-progress').style.strokeDasharray=(circumference*frac).toFixed(2)+' '+circumference.toFixed(2);
  card.querySelector('.status-gauge-number').textContent=Number.isFinite(value)?fmt(value)+'%':'—';
  card.querySelector('.status-gauge-hint').textContent=hint;
  rows.set(key,detail);
 }
 window.renderStatusGauges=function renderStatusGauges(sys={}){
  if(!init())return;
  const cores=finite(Number(sys.cpuCores))||1;
  const load1=finite(Number(sys.load1)),load5=finite(Number(sys.load5)),load15=finite(Number(sys.load15));
  const cpu=finite(Number(sys.cpuPercent)),memUsed=finite(Number(sys.memoryUsed)),memTotal=finite(Number(sys.memoryTotal));
  const diskUsed=finite(Number(sys.diskUsed)),diskTotal=finite(Number(sys.diskTotal));
  draw('load',load1/cores*100,'1 分钟 '+fmt(load1)+' / '+cores+' 核',[
   ['1 分钟平均负载',fmt(load1)],['5 分钟平均负载',fmt(load5)],['15 分钟平均负载',fmt(load15)],['逻辑核心',String(cores)]
  ]);
  draw('cpu',cpu,cores+' 个逻辑核心',[
   ['CPU 使用率',fmt(cpu)+'%'],['CPU 空闲',fmt(Number.isFinite(cpu)?Math.max(0,100-cpu):NaN)+'%'],['逻辑核心',String(cores)],['采样间隔','120 ms']
  ]);
  const mp=percent(memUsed,memTotal);
  draw('memory',mp,bytes(memUsed)+' / '+bytes(memTotal),[
   ['已使用',bytes(memUsed)],['可用（估算）',bytes(Math.max(0,memTotal-memUsed))],['总内存',bytes(memTotal)],['使用率',fmt(mp)+'%']
  ]);
  draw('disk',percent(diskUsed,diskTotal),bytes(diskUsed)+' / '+bytes(diskTotal),[
   ['挂载点','/'],['已使用',bytes(diskUsed)],['可用',bytes(Math.max(0,diskTotal-diskUsed))],['总容量',bytes(diskTotal)]
  ]);
  if(activeRing&&!tooltip.hidden){paintTooltip(activeRing.closest('.status-gauge-card').dataset.gauge);position()}
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
