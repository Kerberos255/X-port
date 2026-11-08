#!/usr/bin/env node
'use strict'
const assert=require('node:assert/strict')
const fs=require('node:fs')
const vm=require('node:vm')

const monitorSource=fs.readFileSync('web/dist/version-monitor.js','utf8')
const progressSource=fs.readFileSync('web/dist/operation-progress.js','utf8')
const extraSource=fs.readFileSync('web/dist/extra.js','utf8')
const updateSource=extraSource.slice(extraSource.indexOf('async function updateXport(){'),extraSource.indexOf('\nfunction bindMetricRings(){'))
const restartSource=extraSource.slice(extraSource.indexOf('async function restartXport(){'),extraSource.indexOf('\nfunction ensureSelfUpdateCard(){'))
const sleep=ms=>new Promise(resolve=>setTimeout(resolve,ms))

function harness({missingPagehide=false}={}){
 let currentVersion='0.2.17',currentInstance='instance-A',pending=false
 let guardStatus=200,dialogOpen=false,editDialog=false
 const navigations=[],stages=[],toasts=[],apiCalls=[],events=new Map()
 const elements={}
 const mk=(name,extra={})=>Object.assign({id:name,textContent:'',hidden:false,dataset:{},addEventListener(){},removeEventListener(){}},extra)
 const modal=mk('operation-progress',{
  showModal(){dialogOpen=true},
  close(){dialogOpen=false}
 })
 Object.defineProperty(modal,'open',{get:()=>dialogOpen})
 const actions=mk('operation-failure-actions',{hidden:true})
 let dismissHandler=null
 const dismiss=mk('operation-failure-dismiss',{
  addEventListener(name,fn){if(name==='click')dismissHandler=fn},
  click(){if(dismissHandler)dismissHandler()}
 })
 const title=mk('operation-progress-title'),detail=mk('operation-progress-detail')
 const stage=mk('operation-progress-stage')
 elements['operation-progress']=modal
 elements['operation-progress-title']=title
 elements['operation-progress-detail']=detail
 elements['operation-progress-stage']=stage
 elements['operation-failure-actions']=actions
 elements['operation-failure-dismiss']=dismiss
 const fakeBanner=mk('version-banner',{setAttribute(){},innerHTML:'',
  querySelector(id){
   if(id==='#xport-version-reason')return mk('banner-reason')
   return mk('banner-button')
  }
 })
 const document={
  getElementById:id=>elements[id]||null,
  activeElement:{closest(){return null},matches(){return false}},
  body:{appendChild(){}},
  createElement(){return fakeBanner},
  addEventListener(){},
  querySelector(selector){
   if(selector==='meta[name="xport-build-version"]')return {content:'0.2.17'}
   if(selector==='#app')return {classList:{contains:()=>false}}
   if(selector==='dialog[open]:not(#operation-progress), #modal:not(.hidden), .fw-edit-input')
    return editDialog?mk('editing'):null
   if(selector==='#operation-progress[open]')return dialogOpen?modal:null
   return null
  }
 }
 const window={
  document,
  addEventListener(type,listener){
   if(!events.has(type))events.set(type,new Set())
   events.get(type).add(listener)
  },
  removeEventListener(type,listener){events.get(type)?.delete(listener)}
 }
 const location={
  href:'http://localhost:12345/',
  replace(dest){
   navigations.push(dest)
   if(!missingPagehide)for(const fn of [...(events.get('pagehide')||[])])fn()
  }
 }
 const fetch=async path=>{
  if(path==='/api/xport/version')return {ok:true,json:async()=>({version:currentVersion,instance:currentInstance})}
  if(path==='/api/xport/update-state')
   return guardStatus===200
    ? {ok:true,status:200,json:async()=>({firewallPending:pending})}
    : {ok:false,status:guardStatus}
  throw Error('unexpected fetch '+path)
 }
 const context=vm.createContext({
  window,document,location,fetch,
  URL,Date,Promise,console,
  setInterval:()=>0,
  setTimeout,clearTimeout,
  toast:(message,bad)=>toasts.push({message,bad}),
  $:()=>({disabled:false,textContent:''}),
  xportSelfUpdate:{current:'0.2.17',latest:'0.2.18',available:true}
 })
 vm.runInContext(progressSource,context,{filename:'operation-progress.js'})
 vm.runInContext(monitorSource,context,{filename:'version-monitor.js'})
 context.XportOperation={
  run:context.window.XportOperation.run,
  ask:async()=>true,
  setStage:context.window.XportOperation.setStage
 }
 vm.runInContext(updateSource,context,{filename:'extra-update.js'})
 vm.runInContext(restartSource,context,{filename:'extra-restart.js'})
 context.api=async(path,options)=>{
  apiCalls.push({path,method:options?.method||'GET'})
  if(path==='/api/xport/version')return {version:currentVersion,instance:currentInstance}
  if(path==='/api/xport/update-state')return {firewallPending:pending}
  if(path==='/api/xport/update'&&options?.method==='POST')return {scheduled:true,update:{latest:'0.2.18'}}
  if(path==='/api/xport/restart'&&options?.method==='POST')return {scheduled:true,ok:true}
  throw Error('unexpected api '+path)
 }
 const op=context.window.XportOperation
 return {
  context,window,op,dialog:modal,actions,stage,title,detail,dismiss,navigations,stages,toasts,apiCalls,
  setVersion:v=>{currentVersion=v},setInstance:v=>{currentInstance=v},
  setPending:v=>{pending=v},setGuardStatus:v=>{guardStatus=v},setEditDialog:v=>{editDialog=v},
  call:expression=>vm.runInContext(expression,context)
 }
}
async function waitUntil(fn,label){
 for(let n=0;n<80;n++){if(fn())return;await sleep(5)}
 throw Error('timeout waiting for '+label)
}
async function run(){
 let passed=0
 const test=async(name,fn)=>{await fn();passed++;process.stdout.write('OK '+name+'\n')}

 await test('progress dialog stays open until perform finishes',async()=>{
  const h=harness()
  let done
  const gate=new Promise(resolve=>{done=resolve})
  const task=h.op.run({title:'Updating',detail:'Waiting',perform:()=>gate})
  assert.equal(h.dialog.open,true)
  assert.equal(h.op.busy,true)
  done({ok:true})
  await task
  assert.equal(h.dialog.open,false)
 })
 await test('progress error stays visible until explicitly acknowledged',async()=>{
  const h=harness()
  const task=h.op.run({title:'Updating',failureAck:true,
   perform:async()=>{throw Error('failed to start')}}).then(()=>null,error=>error)
  await waitUntil(()=>!h.actions.hidden,'ack button')
  assert.equal(h.dialog.open,true)
  assert.equal(h.dialog.dataset.phase,'error')
  assert.match(h.detail.textContent,/failed to start/)
  h.dismiss.click()
  const err=await task
  assert.equal(err.progressAcknowledged,true)
  assert.equal(h.dialog.open,false)
 })
 await test('update keeps real progress open while old version still answers',async()=>{
  const h=harness()
  const action=h.call('updateXport()')
  await waitUntil(()=>h.apiCalls.some(c=>c.path==='/api/xport/update'&&c.method==='POST'),'update submission')
  await sleep(22)
  assert.equal(h.dialog.open,true)
  assert.equal(h.op.busy,true)
  assert.equal(h.navigations.length,0)
  assert.match(h.stage.textContent,/等待新版本启动/)
  h.setVersion('0.2.18')
  await action
  assert.equal(h.navigations.length,1)
  assert.match(h.navigations[0],/_xport_ui=0.2.18/)
  assert.equal(h.dialog.open,false)
  assert.equal(h.apiCalls.filter(x=>x.method==='POST').length,1)
 })
 await test('restart keeps progress open until process generation changes at same version',async()=>{
  const h=harness()
  const action=h.call('restartXport()')
  await waitUntil(()=>h.apiCalls.some(c=>c.path==='/api/xport/restart'&&c.method==='POST'),'restart submission')
  await sleep(22)
  assert.equal(h.dialog.open,true)
  assert.equal(h.navigations.length,0)
  h.setInstance('instance-B')
  await action
  assert.equal(h.navigations.length,1)
  assert.match(h.navigations[0],/_xport_ui=0.2.17/)
  assert.equal(h.apiCalls.filter(x=>x.method==='POST').length,1)
 })
 await test('wrong target version never reports successful update',async()=>{
  const h=harness()
  h.setVersion('0.2.19')
  const err=await h.window.XportVersionMonitor.waitFor('0.2.18',{
   timeoutMs:45,pollMs:5
  }).then(()=>null,e=>e)
  assert.match(err.message,/超时/)
  assert.equal(h.navigations.length,0)
 })
 await test('same instance never confirms a restart',async()=>{
  const h=harness()
  const err=await h.window.XportVersionMonitor.waitFor('0.2.17',{
   mode:'restart',previousInstance:'instance-A',timeoutMs:40,pollMs:5
  }).then(()=>null,e=>e)
  assert.match(err.message,/超时/)
  assert.equal(h.navigations.length,0)
 })
 await test('firewall pending blocks a changed version even during a handoff',async()=>{
  const h=harness()
  h.setVersion('0.2.18')
  h.setPending(true)
  const err=await h.window.XportVersionMonitor.waitFor('0.2.18',{
   timeoutMs:50,pollMs:5
  }).then(()=>null,e=>e)
  assert.match(err.message,/防火墙/)
  assert.equal(h.navigations.length,0)
 })
 await test('other open editors still block self-update refresh',async()=>{
  const h=harness()
  h.setEditDialog(true)
  h.setVersion('0.2.18')
  const err=await h.window.XportVersionMonitor.waitFor('0.2.18',{
   timeoutMs:50,pollMs:5
  }).then(()=>null,e=>e)
  assert.match(err.message,/未关闭的编辑界面/)
  assert.equal(h.navigations.length,0)
 })
 await test('expired session still allows navigating to new binary',async()=>{
  const h=harness()
  h.setGuardStatus(401)
  h.setVersion('0.2.18')
  await h.window.XportVersionMonitor.waitFor('0.2.18',{timeoutMs:80,pollMs:5})
  assert.equal(h.navigations.length,1)
 })
 await test('navigation timeout reports error and does not claim completion',async()=>{
  const h=harness({missingPagehide:true})
  h.setVersion('0.2.18')
  const err=await h.window.XportVersionMonitor.waitFor('0.2.18',{
   timeoutMs:100,pollMs:4,navigationTimeoutMs:12
  }).then(()=>null,e=>e)
  assert.match(err.message,/未能自动刷新/)
  assert.equal(h.navigations.length,1)
 })
 await test('version monitor rejects duplicate handoff requests',async()=>{
  const h=harness()
  const first=h.window.XportVersionMonitor.waitFor('0.2.18',{
   timeoutMs:30,pollMs:4
  }).catch(e=>e)
  await assert.rejects(h.window.XportVersionMonitor.waitFor('0.2.18'),/正在等待完成/)
  await first
 })
 await test('restarted instance identifier must exist before POST',async()=>{
  const h=harness()
  h.setInstance('')
  const task=h.call('restartXport()')
  await waitUntil(()=>!h.actions.hidden,'restart error acknowledgment')
  assert.equal(h.apiCalls.filter(x=>x.method==='POST').length,0)
  h.dismiss.click()
  await task
  assert.equal(h.navigations.length,0)
 })
 await test('update return status does not close progress on missing version',async()=>{
  const h=harness()
  h.context.api=async(path,options)=>{
   h.apiCalls.push({path,method:options?.method||'GET'})
   if(path==='/api/xport/update-state')return {firewallPending:false}
   return {scheduled:true,update:{}}
  }
  const task=h.call('updateXport()')
  await waitUntil(()=>!h.actions.hidden,'unverified version acknowledgment')
  assert.equal(h.dialog.open,true)
  assert.equal(h.navigations.length,0)
  h.dismiss.click()
  await task
  assert.equal(h.dialog.open,false)
 })
 console.log('Panel update/restart handoff regression: '+passed+' scenarios passed')
}
run().catch(err=>{console.error(err);process.exitCode=1})
