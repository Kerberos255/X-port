#!/usr/bin/env node
'use strict'
// Isolation-only mutation contracts. Never points at a real service.
const fs=require('node:fs')
const vm=require('node:vm')
const assert=require('node:assert/strict')
const v3=fs.readFileSync('web/dist/v3.js','utf8')
const app=fs.readFileSync('web/dist/app.js','utf8')
const appBackup=app.slice(app.indexOf('async function readBackupsStrict()'),
 app.indexOf("\ndocument.addEventListener('DOMContentLoaded',boot)"))
const appRestart=app.slice(app.indexOf('async function restartXray(){'),
 app.indexOf('\nasync function loadLogs(){'))
const extra=fs.readFileSync('web/dist/extra.js','utf8')
const panelRestart=extra.slice(extra.indexOf('async function restartXport(){'),
 extra.indexOf('\nfunction ensureSelfUpdateCard(){'))

function page(handler) {
 const calls=[], messages=[]
 const context=vm.createContext({
  document:{addEventListener(){}},
  state:{accounts:[],page:'system'},
  api:async (path,opts)=>{
   calls.push({path,method:opts?.method||'GET'})
   return handler(path,opts)
  },
  toast:(msg,bad)=>messages.push({msg,bad:Boolean(bad)}),
  setTimeout:()=>0,clearTimeout:()=>{},confirm:()=>true,
  XportOperation:{
   busy:false,
   ask:async()=>true,
   run:async ({perform,verify})=>{
    const result=await perform()
    if(verify)await verify(result)
    return result
   },
   refreshAccounts:async()=>[]
  },
  loadBackups:async()=>{},
  loadOverview:async()=>{},
  showLogin:()=>{},
  encodeURIComponent
 })
 vm.runInContext(v3,context,{filename:'v3.js'})
 vm.runInContext(appBackup,context,{filename:'app-backups.js'})
 vm.runInContext(appRestart,context,{filename:'app-restart.js'})
 vm.runInContext(panelRestart,context,{filename:'panel-restart.js'})
 vm.runInContext('sleepV3=async()=>{};waitForPanelV3=async()=>true;waitForXrayV3=async()=>false',context)
 return {context,calls,messages,call:expr=>vm.runInContext(expr,context)}
}
function err(status,msg='network error'){
 const e=new Error(msg);e.status=status;return e
}
function writes(page,method){
 return page.calls.filter(c=>c.method===method).length
}
async function run(){
 let count=0
 const test=async(name,fn)=>{await fn();count++;process.stdout.write('OK '+name+'\n')}

 await test('transient result: read-only confirmation, no replay',async()=>{
  let reads=0
  const p=page(async(path,opt)=>{
   if(opt?.method==='POST')throw err(502)
   if(path==='/api/health')return {ok:true}
   if(path==='/api/state')return {applied:++reads===3}
   throw Error('unexpected '+path)
  })
  const x=await p.call("applyConfirmedMutationV3(()=>api('/api/write',{method:'POST'}),async()=>Boolean((await api('/api/state')).applied))")
  assert.equal(x.recovered,true)
  assert.equal(writes(p,'POST'),1)
 })
 await test('HTTP 200 without state change: no success',async()=>{
  const p=page(async(path,opt)=>{
   if(opt?.method==='POST')return {ok:true}
   if(path==='/api/state')return {applied:false}
   throw Error('unexpected '+path)
  })
  const e=await p.call("applyConfirmedMutationV3(()=>api('/api/write',{method:'POST'}),async()=>Boolean((await api('/api/state')).applied))").then(()=>null,e=>e)
  assert.equal(e.applied,true)
  assert.equal(writes(p,'POST'),1)
 })
 await test('readback failure after applied request: uncertain, not replayed',async()=>{
  const p=page(async(path,opt)=>{
   if(opt?.method==='POST')return {ok:true}
   if(path==='/api/state')throw err(503,'read failed')
   throw Error('unexpected '+path)
  })
  const e=await p.call("applyConfirmedMutationV3(()=>api('/api/write',{method:'POST'}),()=>api('/api/state'))").then(()=>null,e=>e)
  assert.equal(e.uncertain,true)
  assert.equal(writes(p,'POST'),1)
 })
 await test('400 server validation error remains failure',async()=>{
  const p=page(async(path,opt)=>{if(opt?.method==='POST')throw err(400,'invalid input');throw Error('read should not occur')})
  await assert.rejects(p.call("applyConfirmedMutationV3(()=>api('/api/write',{method:'POST'}),async()=>true)"),/invalid input/)
  assert.equal(writes(p,'POST'),1)
 })
 await test('backup create confirms exact returned name',async()=>{
  const rows=[{name:'old'}]
  const p=page(async(path,opt)=>{
   if(path==='/api/backups'&&opt?.method==='POST'){rows.push({name:'created'});return {name:'created'}}
   if(path==='/api/backups')return {backups:rows}
   throw Error('unexpected '+path)
  })
  await p.call('createBackup()')
  assert.equal(writes(p,'POST'),1)
  assert(p.messages.some(m=>m.msg==='备份已创建'))
 })
 await test('backup create HTTP 201 but missing entry is not success',async()=>{
  const p=page(async(path,opt)=>{
   if(opt?.method==='POST')return {name:'missing'}
   if(path==='/api/backups')return {backups:[]}
   throw Error('unexpected '+path)
  })
  await p.call('createBackup()')
  assert.equal(writes(p,'POST'),1)
  assert(!p.messages.some(m=>m.msg==='备份已创建'))
  assert(p.messages.some(m=>m.bad))
 })
 await test('backup create lost response confirmed by newly listed identity',async()=>{
  let committed=false
  const p=page(async(path,opt)=>{
   if(opt?.method==='POST'){committed=true;throw err(502)}
   if(path==='/api/backups')return {backups:committed?[{name:'created'}]:[]}
   throw Error('unexpected '+path)
  })
  await p.call('createBackup()')
  assert.equal(writes(p,'POST'),1)
  assert(p.messages.some(m=>m.msg==='备份已创建'))
 })
 await test('backup delete confirms target absence',async()=>{
  let rows=[{name:'remove'},{name:'keep'}]
  const p=page(async(path,opt)=>{
   if(opt?.method==='DELETE'){rows=rows.filter(x=>x.name!=='remove');return {ok:true}}
   if(path==='/api/backups')return {backups:rows}
   throw Error('unexpected '+path)
  })
  await p.call("deleteBackup('remove')")
  assert.equal(writes(p,'DELETE'),1)
  assert(p.messages.some(m=>m.msg==='备份已删除'))
 })
 await test('backup delete false HTTP success rejected',async()=>{
  const p=page(async(path,opt)=>{
   if(opt?.method==='DELETE')return {ok:true}
   if(path==='/api/backups')return {backups:[{name:'remove'}]}
   throw Error('unexpected '+path)
  })
  await p.call("deleteBackup('remove')")
  assert.equal(writes(p,'DELETE'),1)
  assert(!p.messages.some(m=>m.msg==='备份已删除'))
 })
 await test('backup delete lost response reads removal, no replay',async()=>{
  let removed=false
  const p=page(async(path,opt)=>{
   if(opt?.method==='DELETE'){removed=true;throw err(502)}
   if(path==='/api/backups')return {backups:removed?[]:[{name:'remove'}]}
   throw Error('unexpected '+path)
  })
  await p.call("deleteBackup('remove')")
  assert.equal(writes(p,'DELETE'),1)
  assert(p.messages.some(m=>m.msg==='备份已删除'))
 })
 await test('Xray restart HTTP 200 but core inactive is not success',async()=>{
  const p=page(async(path,opt)=>{
   if(opt?.method==='POST')return {ok:true}
   if(path==='/api/overview')return {system:{xrayActive:false}}
   throw Error('unexpected '+path)
  })
  await p.call('restartXray()')
  assert.equal(writes(p,'POST'),1)
  assert(!p.messages.some(m=>m.msg==='Xray 已重启并确认运行'))
  assert(p.messages.some(m=>m.bad))
 })
 await test('Xray restart completes with observed active state',async()=>{
  const p=page(async(path,opt)=>{
   if(opt?.method==='POST')return {ok:true}
   if(path==='/api/overview')return {system:{xrayActive:true}}
   throw Error('unexpected '+path)
  })
  await p.call('restartXray()')
  assert.equal(writes(p,'POST'),1)
  assert(p.messages.some(m=>m.msg==='Xray 已重启并确认运行'))
 })
 await test('Xray version must match target AND active state',async()=>{
  const p=page(async()=>({system:{xrayVersion:'26.11.0',xrayActive:false}}))
  await assert.rejects(p.call("confirmXrayVersionV3('26.11.0')"),/状态确认/)
  const p2=page(async()=>({system:{xrayVersion:'26.11.0',xrayActive:true}}))
  await p2.call("confirmXrayVersionV3('v26.11.0')")
 })
 await test('panel restart requires scheduled=true before reporting success',async()=>{
  const p=page(async(path,opts)=>{
   if(path==='/api/xport/restart'&&opts?.method==='POST')return {ok:true}
   throw Error('unexpected '+path)
  })
  await p.call('restartXport()')
  assert.equal(writes(p,'POST'),1)
  assert(!p.messages.some(m=>m.msg.includes('已安排重启')))
  assert(p.messages.some(m=>m.bad))
 })
 await test('panel restart accepted exactly once',async()=>{
  const p=page(async(path,opts)=>{
   if(path==='/api/xport/restart'&&opts?.method==='POST')return {ok:true,scheduled:true}
   throw Error('unexpected '+path)
  })
  await p.call('restartXport()')
  assert.equal(writes(p,'POST'),1)
  assert(p.messages.some(m=>m.msg.includes('已安排重启')))
 })
 await test('expert readback checks values, not only GET 200',async()=>{
  const p=page(async()=>({}))
  const a=p.call("expertConfigMatchesV3({acceptProxyProtocol:true},{acceptProxyProtocol:false,supportsReality:false,supportsTLS:false,supportsXhttp:false})")
  assert.equal(a,false)
 })
 console.log('X-port operation-state regression: '+count+' scenarios passed')
}
run().catch(e=>{console.error(e);process.exitCode=1})
