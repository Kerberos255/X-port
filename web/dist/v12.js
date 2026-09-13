/* X-port v0.1.16: reliable clone-account interception. */
async function openCloneModalV12(id){
 const a=state.accounts.find(x=>x.id===id)
 openModal(`<div class="modal-title"><span>CLONE ACCOUNT</span><h2>克隆 ${esc(a?.name||'账号')}</h2></div><form id="clone-form" class="form-grid"><label>新账号名<input name="name" value="${esc((a?.name||'account')+' copy')}" required></label><label>新端口<input id="clone-port" name="port" type="number" min="1" max="65535" placeholder="正在随机分配…"></label><div class="form-actions full"><button type="button" class="btn ghost" id="clone-cancel">取消</button><button class="btn primary" type="submit">克隆并应用</button></div></form>`)
 $('#clone-cancel').onclick=closeModal
 const portInput=$('#clone-port')
 try{
  const d=await api('/api/accounts/port-suggestion')
  if(!$('#clone-form'))return
  portInput.value=d.port||''
 }catch(err){toast(err.message,true)}
 $('#clone-form').onsubmit=async e=>{
  e.preventDefault()
  const form=e.currentTarget,f=new FormData(form),button=form.querySelector('button[type=submit]')
  button.disabled=true;button.textContent='克隆中…'
  try{
   await api(`/api/accounts/${id}/clone`,{method:'POST',body:JSON.stringify({name:f.get('name'),port:Number(f.get('port')||0)})})
   closeModal();await loadAccounts();await loadOverview();toast('账号已克隆')
  }catch(err){toast(err.message,true);button.disabled=false;button.textContent='克隆并应用'}
 }
}

document.addEventListener('click',e=>{
 const button=e.target.closest?.('button[data-act="clone"]')
 if(!button||button.disabled)return
 const row=button.closest('.account-row')
 if(!row)return
 e.preventDefault()
 e.stopPropagation()
 e.stopImmediatePropagation()
 void openCloneModalV12(Number(row.dataset.id))
},true)
