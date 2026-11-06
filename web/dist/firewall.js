/* X-port firewall UI. Real API only; no browser-maintained firewall state. */
(() => {
  'use strict'
  const root = document.getElementById('page-firewall')
  if (!root) return
  const el = id => document.getElementById(id)
  const html = value => String(value == null ? '' : value).replace(/[&<>"']/g, ch => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[ch]))
  const state = { data: null, filter: 'all', term: '', tab: 'rules', editing: null, detail: null, sequence: 0 }
  const request = (url, method, body) => api(url, {method, body: JSON.stringify(body)})

  function rules() { return Array.isArray(state.data?.rules) ? state.data.rules : [] }
  function label(r) { return r.purpose || '未识别' }
  function editable() { return state.data?.backend === 'firewalld' || state.data?.backend === 'ufw' }
  function status(r) { return r.runtime !== r.permanent ? '需检查' : (r.runtime ? '已配置' : '未生效') }
  function stateBadge(r) { return '<span class="fw-state '+(r.runtime !== r.permanent ? 'warn' : '')+'">'+status(r)+'</span>' }
  function source(r) { return r.backend === 'firewalld' ? 'firewalld · '+r.zone : r.backend }
  function shown() {
    return rules().filter(r => {
      if (state.filter === 'xray' && !r.xray) return false
      if (state.filter === 'warn' && r.runtime === r.permanent) return false
      const term = state.term.toLowerCase().trim()
      return !term || [r.port,r.protocol,r.purpose,r.note,r.zone,r.access,r.backend].join(' ').toLowerCase().includes(term)
    })
  }
  function fieldButton(r, key) {
    const content = key === 'note' ? r.note : r.purpose
    const empty = key === 'note' ? '添加备注' : '未识别'
    if (state.editing && state.editing.id === r.id && state.editing.key === key) {
      return '<span class="fw-inline"><input class="fw-edit-input" aria-label="编辑'+(key==='note'?'备注':'用途')+'" maxlength="'+(key==='note'?120:60)+'" value="'+html(content || '')+'"><button type="button" class="btn fw-mini" data-fw-act="save">保存</button><button type="button" class="btn ghost fw-mini" data-fw-act="cancel">取消</button></span>'
    }
    return '<button type="button" class="fw-remark" data-fw-act="edit" data-fw-id="'+html(r.id)+'" data-fw-key="'+key+'" title="点击编辑"><span>'+html(content || empty)+'</span><small aria-hidden="true">✎</small></button>'
  }
  function render() {
    const data = state.data
    el('fw-backend').textContent = data ? ({firewalld:'firewalld',ufw:'UFW',nftables:'nftables（只读）',iptables:'iptables（只读）',none:'未检测到'}[data.backend] || data.backend) : '检测中'
    el('fw-zone').textContent = data?.defaultZone || (data?.backend === 'ufw' ? 'UFW 规则' : '—')
    el('fw-engine-state').textContent = !data ? '—' : (data.active ? '● 运行中' : '只读 / 未启用')
    el('fw-total').textContent = rules().length
    el('fw-active').textContent = rules().filter(r=>r.runtime).length
    el('fw-xray-count').textContent = rules().filter(r=>r.xray).length
    el('fw-warn-count').textContent = rules().filter(r=>r.runtime !== r.permanent).length
    el('fw-add').disabled = !editable()
    el('fw-notice').textContent = data?.notice || '无法读取防火墙状态'
    const filtered = shown()
    el('fw-count').textContent = '显示 '+filtered.length+' / '+rules().length+' 条规则；备注与用途保存在 X-port 数据库中'
    el('fw-rows').innerHTML = filtered.length ? filtered.map(r => '<tr>'+
      '<td data-label="端口">'+html(r.port)+'</td>'+
      '<td data-label="用途">'+fieldButton(r,'purpose')+'</td>'+
      '<td data-label="协议">'+html(r.protocol.toUpperCase())+'</td>'+
      '<td data-label="规则来源"><span class="fw-source">'+html(source(r))+'</span></td>'+
      '<td data-label="访问来源">'+html(r.access || '未确认')+'</td>'+
      '<td data-label="状态">'+stateBadge(r)+'</td>'+
      '<td data-label="备注">'+fieldButton(r,'note')+'</td>'+
      '<td data-label="操作"><button class="btn fw-mini ghost" data-fw-act="detail" data-fw-id="'+html(r.id)+'">详情</button></td>'+
      '</tr>').join('') : '<tr><td colspan="8" class="muted">暂无匹配的端口规则。更多服务或富规则，请切换「底层规则」查看。</td></tr>'
    el('fw-xray-list').innerHTML = rules().filter(r=>r.xray).map(r => '<div class="fw-xray-line"><div><b>'+html(label(r))+'</b><small class="fw-sub">'+html(r.port)+' / '+html(r.protocol.toUpperCase())+' · '+html(source(r))+'</small></div>'+stateBadge(r)+'</div>').join('') || '<p class="fw-note">目前没有匹配到 Xray 账号的简单端口规则。</p>'
    if (state.editing) { const input = root.querySelector('.fw-edit-input'); if (input) {input.focus(); input.select()} }
  }
  async function refresh() {
    const seq = ++state.sequence
    el('fw-refresh').disabled = true
    try {
      const data = await api('/api/firewall')
      if (seq !== state.sequence) return
      state.data = data
      render()
    } catch (err) {
      if (seq !== state.sequence) return
      state.data = null
      render()
      el('fw-notice').textContent = '读取失败：'+err.message
      toast('防火墙状态读取失败：'+err.message,true)
    } finally { if (seq === state.sequence) el('fw-refresh').disabled = false }
  }
  function setTab(tab) {
    state.tab=tab
    for (const name of ['rules','raw','xray']) el('fw-'+name+'-view').hidden=name!==tab
    root.querySelectorAll('[data-fw-tab]').forEach(button=>button.classList.toggle('active',button.dataset.fwTab===tab))
  }
  async function saveEdit() {
    const edit=state.editing
    if (!edit) return
    const input=root.querySelector('.fw-edit-input')
    if (!input) return
    const value=input.value.trim()
    const endpoint=edit.key==='note'?'notes':'purpose'
    if (edit.key==='note' && ([...value].length>120 || new TextEncoder().encode(value).length>360)) return toast('备注最多 120 字',true)
    if (edit.key==='purpose' && ([...value].length>60 || new TextEncoder().encode(value).length>180)) return toast('用途最多 60 字',true)
    try {
      await request('/api/firewall/'+endpoint,'PUT',{id:edit.id,[edit.key]:value})
      state.editing=null
      toast('保存成功')
      await refresh()
    } catch(err) {toast(err.message,true)}
  }
  function openDetail(id) {
    const r=rules().find(x=>x.id===id)
    if(!r)return
    state.detail=r
    el('fw-detail-title').textContent=r.port+' / '+r.protocol.toUpperCase()
    el('fw-detail-body').innerHTML=[
      ['用途',r.purpose],['备注',r.note||'未填写'],['来源',source(r)],['访问来源',r.access],['运行时规则',r.runtime?'存在':'不存在'],['永久规则',r.permanent?'存在':'不存在']
    ].map(([name,val])=>'<div style="display:flex;justify-content:space-between;gap:12px;border-bottom:1px solid var(--line);padding:10px 0"><span>'+html(name)+'</span><b>'+html(val)+'</b></div>').join('')
    el('fw-detail-remove').hidden = !editable() || r.protected
    el('fw-detail-dialog').showModal()
  }
  function openAdd() {
    if(!editable())return
    el('fw-add-port').value=''
    const zone=el('fw-add-zone'),isUfw=state.data?.backend==='ufw'
    zone.innerHTML = isUfw ? '<option value="">UFW（默认）</option>' : (state.data?.zones||[]).map(z=>'<option value="'+html(z)+'">'+html(z)+'</option>').join('')
    zone.disabled=isUfw
    if(!isUfw)zone.value=state.data.defaultZone
    el('fw-add-dialog').showModal()
    el('fw-add-port').focus()
  }
  root.addEventListener('click',ev=>{
    const button=ev.target.closest('button')
    if(!button)return
    if(button.dataset.fwTab){setTab(button.dataset.fwTab);return}
    if(button.dataset.fwFilter){
      state.filter=button.dataset.fwFilter
      state.editing=null
      root.querySelectorAll('[data-fw-filter]').forEach(x=>x.classList.toggle('active',x===button))
      render();return
    }
    const act=button.dataset.fwAct,id=button.dataset.fwId
    if(act==='edit'){state.editing={id,key:button.dataset.fwKey};render()}
    if(act==='cancel'){state.editing=null;render()}
    if(act==='save')void saveEdit()
    if(act==='detail')openDetail(id)
  })
  root.addEventListener('keydown',ev=>{
    if(!ev.target.matches('.fw-edit-input'))return
    if(ev.key==='Enter'){ev.preventDefault();void saveEdit()}
    if(ev.key==='Escape'){ev.preventDefault();state.editing=null;render()}
  })
  el('fw-search').addEventListener('input',ev=>{state.term=ev.target.value;state.editing=null;render()})
  el('fw-refresh').addEventListener('click',()=>void refresh())
  el('fw-add').addEventListener('click',openAdd)
  el('fw-load-raw').addEventListener('click',async ()=>{
    el('fw-load-raw').disabled=true
    el('fw-raw').textContent='读取中…'
    try{const data=await api('/api/firewall/raw');el('fw-raw').textContent=data.ruleset||'规则集为空'}
    catch(err){el('fw-raw').textContent='读取失败：'+err.message}
    finally{el('fw-load-raw').disabled=false}
  })
  el('fw-add-dialog').addEventListener('close',async()=>{
    if(el('fw-add-dialog').returnValue!=='apply')return
    const port=Number(el('fw-add-port').value)
    const protocol=el('fw-add-proto').value,zone=el('fw-add-zone').value
    if(!Number.isInteger(port)||port<1||port>65535)return toast('端口必须是 1–65535 的整数',true)
    if(!window.confirm('确认在系统防火墙中放行 '+port+'/'+protocol+' 吗？'))return
    try{await request('/api/firewall/rules','POST',{action:'add',port,protocol,zone});toast('规则已添加');await refresh()}
    catch(err){toast(err.message,true)}
  })
  el('fw-detail-dialog').addEventListener('close',async()=>{
    const r=state.detail
    if(el('fw-detail-dialog').returnValue!=='remove'||!r)return
    if(!window.confirm('确认删除 '+r.port+'/'+r.protocol+' 的简单端口放行规则吗？此操作可能导致现有连接中断。'))return
    try{await request('/api/firewall/rules','POST',{action:'remove',port:r.port,protocol:r.protocol,zone:r.zone||''});toast('规则已删除');await refresh()}
    catch(err){toast(err.message,true)}
  })
  const nav=document.querySelector('#nav [data-page="firewall"]')
  if(nav)nav.addEventListener('click',()=>{if(!document.getElementById('app').classList.contains('hidden'))void refresh()})
  setTab('rules')
})()
