/* Shared, non-dismissible progress UI for account and firewall writes.
   Confirmation and safety-recovery prompts deliberately remain separate. */
(() => {
  'use strict'
  let busy=false
  let confirming=false
  const dialog=()=>document.getElementById('operation-progress')
  const title=()=>document.getElementById('operation-progress-title')
  const detail=()=>document.getElementById('operation-progress-detail')
  const stage=()=>document.getElementById('operation-progress-stage')
  function setStage(text){
    const node=stage()
    if(node)node.textContent=String(text||'')
  }
  function start(name,subject){
    if(busy||confirming)throw new Error('已有操作正在执行，请勿重复提交')
    const node=dialog()
    if(!node || !node.showModal)throw new Error('操作进度弹窗不可用')
    busy=true
    node.dataset.phase='busy'
    const actions=document.getElementById('operation-failure-actions')
    if(actions)actions.hidden=true
    title().textContent=String(name||'正在处理')
    detail().textContent=String(subject||'正在应用配置，请勿重复操作')
    setStage('正在提交并应用配置…')
    try{node.showModal()}catch(err){busy=false;throw err}
  }
  function stop(){
    const node=dialog()
    if(node?.open)node.close()
    busy=false
  }
  async function acknowledgeFailure(err){
    const node=dialog()
    if(!node?.open)return
    node.dataset.phase='error'
    const uncertain=Boolean(err?.applied||err?.uncertain||!err?.status||
      ([502,503,504].includes(Number(err.status))&&!err?.serverError))
    title().textContent=uncertain?'操作结果未确认':'操作失败'
    detail().textContent=String(err?.message||'请检查服务状态后再操作')
    setStage(uncertain?'未确认成功，请先核对当前状态，避免重复提交。':
      '服务器已拒绝操作，请确认错误原因后再尝试。')
    const actions=document.getElementById('operation-failure-actions')
    const button=document.getElementById('operation-failure-dismiss')
    if(!actions||!button)return
    actions.hidden=false
    await new Promise(resolve=>button.addEventListener('click',resolve,{once:true}))
    err.progressAcknowledged=true
  }
  async function run({title:name,detail:subject,perform,verify,failureAck=false}){
    if(typeof perform!=='function')throw new TypeError('perform is required')
    start(name,subject)
    try{
      const result=await perform()
      if(typeof verify==='function'){
        setStage('正在核对最新状态…')
        try{await verify(result)}catch(err){
          const wrapped=new Error('操作请求已返回，但无法核对最新状态；请刷新列表确认后再操作。'+(err?.message?' '+err.message:''))
          wrapped.applied=true
          throw wrapped
        }
      }
      return result
    }catch(err){
      if(failureAck)await acknowledgeFailure(err)
      throw err
    }finally{stop()}
  }
  function ask({title:name,detail:message,confirmText='确认',cancelText='取消',danger=false}){
    if(busy||confirming)return Promise.reject(new Error('已有操作尚未完成'))
    const modal=document.getElementById('operation-confirm')
    if(!modal)return Promise.reject(new Error('确认弹窗不可用'))
    confirming=true
    document.getElementById('operation-confirm-title').textContent=String(name||'确认操作')
    document.getElementById('operation-confirm-detail').textContent=String(message||'')
    const yes=document.getElementById('operation-confirm-apply')
    const no=document.getElementById('operation-confirm-cancel')
    yes.textContent=String(confirmText)
    no.textContent=String(cancelText)
    yes.className=danger?'btn danger':'btn primary'
    return new Promise((resolve,reject)=>{
      let done=false
      const finish=accepted=>{
        if(done)return
        done=true
        yes.removeEventListener('click',accept)
        no.removeEventListener('click',rejectClick)
        modal.removeEventListener('cancel',cancel)
        if(modal.open)modal.close()
        confirming=false
        resolve(accepted)
      }
      const accept=()=>finish(true)
      const rejectClick=()=>finish(false)
      const cancel=event=>{event.preventDefault();finish(false)}
      yes.addEventListener('click',accept)
      no.addEventListener('click',rejectClick)
      modal.addEventListener('cancel',cancel)
      try{modal.showModal()}catch(err){
        yes.removeEventListener('click',accept)
        no.removeEventListener('click',rejectClick)
        modal.removeEventListener('cancel',cancel)
        confirming=false
        reject(err)
      }
    })
  }
  function input({title:name,detail:message,defaultValue='',placeholder='',confirmText='确认',cancelText='取消'}){
    if(busy||confirming)return Promise.reject(new Error('已有操作尚未完成'))
    const modal=document.getElementById('operation-confirm')
    if(!modal)return Promise.reject(new Error('输入弹窗不可用'))
    confirming=true
    document.getElementById('operation-confirm-title').textContent=String(name||'请输入')
    const description=document.getElementById('operation-confirm-detail')
    description.textContent=String(message||'')
    const field=document.createElement('input')
    field.type='text'
    field.className='operation-confirm-input'
    field.setAttribute('aria-label',String(name||'请输入'))
    field.placeholder=String(placeholder||'')
    field.value=String(defaultValue||'')
    description.after(field)
    const yes=document.getElementById('operation-confirm-apply')
    const no=document.getElementById('operation-confirm-cancel')
    yes.textContent=String(confirmText)
    no.textContent=String(cancelText)
    yes.className='btn primary'
    return new Promise((resolve,reject)=>{
      let done=false
      const finish=value=>{
        if(done)return
        done=true
        yes.removeEventListener('click',accept)
        no.removeEventListener('click',dismiss)
        modal.removeEventListener('cancel',cancel)
        field.removeEventListener('keydown',keydown)
        field.remove()
        if(modal.open)modal.close()
        confirming=false
        resolve(value)
      }
      const accept=()=>finish(field.value)
      const dismiss=()=>finish(null)
      const cancel=event=>{event.preventDefault();dismiss()}
      const keydown=event=>{if(event.key==='Enter'){event.preventDefault();accept()}}
      yes.addEventListener('click',accept)
      no.addEventListener('click',dismiss)
      modal.addEventListener('cancel',cancel)
      field.addEventListener('keydown',keydown)
      try{modal.showModal();field.focus();field.select()}catch(err){
        yes.removeEventListener('click',accept)
        no.removeEventListener('click',dismiss)
        modal.removeEventListener('cancel',cancel)
        field.removeEventListener('keydown',keydown)
        field.remove();confirming=false;reject(err)
      }
    })
  }
  async function refreshAccounts(){
    // An actual read is required before dismissing progress: caller should
    // not mistake a successful HTTP write for a refreshed account list.
    const data=await api('/api/accounts')
    state.accounts=data.accounts||[]
    renderAccounts()
    // A failed overview/online status read must not invalidate a successful
    // authoritative account-list read during an Xray restart.
    if(typeof loadOverview==='function'){
      try{await loadOverview(false,true)}catch{}
    }
    return state.accounts
  }
  document.addEventListener('DOMContentLoaded',()=>{
    const node=dialog()
    if(node)node.addEventListener('cancel',event=>event.preventDefault())
  })
  window.XportOperation=Object.freeze({
    run,ask,input,setStage,refreshAccounts,get busy(){return busy},get confirming(){return confirming}
  })
})()
