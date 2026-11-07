/* Shared, non-dismissible progress UI for account and firewall writes.
   Confirmation and safety-recovery prompts deliberately remain separate. */
(() => {
  'use strict'
  let busy=false
  const dialog=()=>document.getElementById('operation-progress')
  const title=()=>document.getElementById('operation-progress-title')
  const detail=()=>document.getElementById('operation-progress-detail')
  const stage=()=>document.getElementById('operation-progress-stage')
  function setStage(text){
    const node=stage()
    if(node)node.textContent=String(text||'')
  }
  function start(name,subject){
    if(busy)throw new Error('已有操作正在执行，请勿重复提交')
    const node=dialog()
    if(!node || !node.showModal)throw new Error('操作进度弹窗不可用')
    busy=true
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
  async function run({title:name,detail:subject,perform,verify}){
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
    }finally{stop()}
  }
  document.addEventListener('DOMContentLoaded',()=>{
    const node=dialog()
    if(node)node.addEventListener('cancel',event=>event.preventDefault())
  })
  window.XportOperation=Object.freeze({
    run,setStage,get busy(){return busy}
  })
})()
