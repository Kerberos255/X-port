/* v0.1.4: Overview no longer owns Xray update controls. */
checkUpdate=async function(manual=false){
 renderXrayUpdateState(null)
 try{
  const d=await api('/api/xray/update');state.update=d
  renderXrayUpdateState(d)
  if(manual)toast(d.available?'发现可用更新':'Xray 已是最新')
 }catch(e){
  renderXrayUpdateState(null,e.message)
  if(manual)toast(e.message,true)
 }
 await checkXrayRollback()
}
