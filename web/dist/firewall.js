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
  function editable() { return state.data?.writable === true && (state.data?.backend === 'firewalld' || state.data?.backend === 'ufw') }
  function status(r) { return r.runtime !== r.permanent ? '需检查' : (r.runtime ? '已配置' : '未生效') }
  function stateBadge(r) { return '<span class="fw-state '+(r.runtime !== r.permanent ? 'warn' : '')+'">'+status(r)+'</span>' }
  function usageBadge(r) {
    if(r.usage==='used')return '<span class="fw-usage used"><span class="fw-usage-dot"></span>已使用</span>'
    if(r.usage==='unused')return '<span class="fw-usage idle"><span class="fw-usage-dot"></span>未发现监听</span>'
    return '<span class="fw-usage unknown"><span class="fw-usage-dot"></span>无法检测</span>'
  }
  function policy(r) { return r.action === 'deny' || r.action === 'drop' ? 'deny' : 'allow' }
  function policyBadge(r) {
    return '<span class="fw-policy '+policy(r)+'">'+(policy(r)==='deny'?'拒绝':'允许')+'</span>'
  }
  function accountCell(r) {
    const title=r.accountName ? html(r.accountName) : html(label(r))
    const subtitle=r.accountName ? ('Xray · '+html(r.purpose||'账号')+(r.accountEnabled?'':' · 账号已停用')) : (r.xray?'Xray · '+html(r.protocol.toUpperCase()):html(r.access||'系统端口'))
    const linked = r.accountName && Number.isInteger(r.accountId) && r.accountId>0
    return '<div class="fw-account-cell">'+(linked?'<button type="button" class="fw-account-link" data-fw-act="account" data-fw-account="'+r.accountId+'" title="前往 Xray 账号管理">'+title+'</button>':'<strong>'+title+'</strong>')+'<small>'+subtitle+'</small></div>'
  }
  function ruleToggle(r) {
    const active=Boolean(r.runtime&&r.permanent)
    const locked=Boolean(r.protected||r.runtime!==r.permanent)
    return '<button type="button" class="fw-switch '+(active?'on':'')+'" role="switch" aria-checked="'+active+'" aria-label="'+html(r.port+'/'+r.protocol)+' 规则生效" data-fw-act="toggle" data-fw-id="'+html(r.id)+'" '+(locked?'disabled title="受保护或状态不一致，暂不能切换"':'title="规则启停功能仍在隔离测试"')+'><span></span></button>'+(locked?'<small class="fw-switch-caption">保护中</small>':'')
  }
  function source(r) { return (r.managed ? 'X-port · ' : '外部 · ') + (r.backend === 'firewalld' ? 'firewalld · '+r.zone : r.backend) }
  function shortSource(r) { return (r.managed ? 'X-port · ' : '外部 · ') + r.backend }
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
    el('fw-engine-state').textContent = !data ? '—' : (data.writable ? '● 可管理' : (data.active ? '只读 · 检查' : '只读 / 未启用'))
    const diagnostic=el('fw-diagnostic')
    diagnostic.hidden = !data?.unsafeReason
    el('fw-diagnostic-reason').textContent = data?.unsafeReason || ''
    el('fw-total').textContent = rules().length
    el('fw-active').textContent = rules().filter(r=>r.usage==='used').length
    el('fw-xray-count').textContent = rules().filter(r=>r.xray).length
    el('fw-warn-count').textContent = rules().filter(r=>r.runtime !== r.permanent).length + (data?.unsafeReason ? 1 : 0)
    el('fw-add').disabled = !editable()
    el('fw-notice').textContent = data?.notice || '无法读取防火墙状态'
    const pending = Array.isArray(data?.pending) ? data.pending : []
    let pendingBox = el('fw-pending')
    if (!pendingBox) {
      pendingBox = document.createElement('div')
      pendingBox.id = 'fw-pending'
      el('fw-notice').before(pendingBox)
    }
    pendingBox.innerHTML = pending.map(p => {
      const active = Number(p.deadline) > Date.now()
      return '<div class="fw-pending-item"><span>↺ '+html(p.id)+' · '+(active ? '等待连接确认，超时自动恢复' : '恢复期限已过，请刷新检查规则')+'</span>' +
        (active ? '<button type="button" class="btn fw-mini" data-fw-act="confirm-pending" data-fw-id="'+html(p.id)+'" data-fw-token="'+html(p.token)+'">确认连接正常</button>' : '')+'</div>'
    }).join('')
    const filtered = shown()
    el('fw-count').textContent = '显示 '+filtered.length+' / '+rules().length+' 条规则；备注与用途保存在 X-port 数据库中'
    el('fw-rows').innerHTML = filtered.length ? filtered.map(r => '<tr>'+
      '<td data-label="端口 / 协议"><div class="fw-port-cell"><strong>'+html(r.port)+'</strong><small>/ '+html(r.protocol.toUpperCase())+'</small></div></td>'+
      '<td data-label="关联账号 / 用途">'+accountCell(r)+'</td>'+
      '<td data-label="使用情况">'+usageBadge(r)+'</td>'+
      '<td data-label="策略">'+policyBadge(r)+'</td>'+
      '<td data-label="来源"><span class="fw-source" title="'+html(source(r))+'">'+html(shortSource(r))+'</span></td>'+
      '<td data-label="规则生效"><div class="fw-toggle-cell">'+ruleToggle(r)+'</div></td>'+
      '<td data-label="备注">'+fieldButton(r,'note')+'</td>'+
      '<td data-label="操作"><div class="fw-actions"><button class="btn fw-mini ghost" data-fw-act="configure" data-fw-id="'+html(r.id)+'">编辑</button><button class="btn fw-mini ghost" data-fw-act="detail" data-fw-id="'+html(r.id)+'">详情</button></div></td>'+
      '</tr>').join('') : '<tr><td colspan="8" class="muted">暂无匹配的规则。</td></tr>'
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
    const badge=el('fw-detail-badge')
    badge.textContent=status(r)
    badge.classList.toggle('warn',r.runtime!==r.permanent)
    el('fw-detail-summary').textContent=r.managed
      ? '由 X-port 创建的端口规则；修改前仍需检查服务和网络连通性。'
      : '由系统或其他管理器维护。可修改用途和备注，删除操作保留在原管理器中。'
    const groups=[
      ['基本信息',[['用途',label(r)],['备注',r.note||'未填写'],['规则来源',source(r)],['访问来源',r.access||'未确认']]],
      ['规则状态',[['运行时',r.runtime?'已配置':'不存在'],['永久配置',r.permanent?'已配置':'不存在'],['地址族',r.backend==='ufw'?(r.ipv4?'IPv4 ':'')+(r.ipv6?'IPv6':''):'由 firewalld Zone 决定']]],
      ['安全保护',[['规则归属',r.managed?'X-port 管理':'外部或未知（不可删除）'],['端口保护',r.protected?'已启用':'未检测到本机监听']]]
    ]
    el('fw-detail-body').innerHTML=groups.map(([heading,fields])=>
      '<section class="fw-detail-section"><h3>'+html(heading)+'</h3><dl>'+
      fields.map(([name,value])=>'<div class="fw-detail-row"><dt>'+html(name)+'</dt><dd>'+html(value)+'</dd></div>').join('')+
      '</dl></section>'
    ).join('')
    el('fw-detail-remove').hidden = !editable() || r.protected || !r.managed
    el('fw-detail-dialog').returnValue = ''
    el('fw-detail-dialog').showModal()
  }
  function openRuleEditor(id){
    const r=rules().find(row=>row.id===id)
    if(!r)return
    el('fw-policy-title').textContent='编辑 '+r.port+'/'+r.protocol.toUpperCase()
    el('fw-policy-subtitle').textContent=r.managed?'X-port 管理的规则':'外部规则 · 与当前 firewalld 共用'
    el('fw-policy-port').value=r.port
    el('fw-policy-proto').value=r.protocol
    el('fw-policy-action').value=policy(r)
    el('fw-policy-zone').value=r.zone||'UFW'
    el('fw-policy-protection').textContent=r.protected
      ? '此端口检测到管理服务或监听，系统会禁止危险改动。'
      : '预览阶段：后端安全编辑及 120 秒回退仍在联调，此处只展示字段，不会修改系统规则。'
    el('fw-policy-save').disabled=true
    el('fw-policy-save').title='待隔离安全验证通过后开放'
    el('fw-policy-dialog').showModal()
  }
  function openAdd() {
    if(!editable())return
    el('fw-add-port').value=''
    const zone=el('fw-add-zone'),isUfw=state.data?.backend==='ufw'
    zone.innerHTML = isUfw ? '<option value="">UFW（默认）</option>' : (state.data?.zones||[]).map(z=>'<option value="'+html(z)+'">'+html(z)+'</option>').join('')
    zone.disabled=isUfw
    if(!isUfw)zone.value=state.data.defaultZone
    el('fw-add-dialog').returnValue = ''
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
    if(act==='confirm-pending') {
      if (!window.confirm('已确认仍能正常访问管理面板？确认后将取消定时恢复。')) return
      void request('/api/firewall/rules/confirm','POST',{id,token:button.dataset.fwToken}).then(async()=>{
        toast('连接已确认，定时恢复已取消')
        await refresh()
      }).catch(err=>toast(err.message,true))
      return
    }
    if(act==='account'){document.querySelector('#nav [data-page="accounts"]')?.click();return}
    if(act==='toggle'){toast('预览阶段：规则启停正在验证断线恢复与规则冲突，暂不执行系统修改。');return}
    if(act==='configure'){openRuleEditor(id);return}
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
    catch(err){el('fw-raw').textContent='读取失败：'+err.message+'。请在系统页检查 X-port 服务单元兼容状态；不要直接覆盖现有自定义服务文件。'}
    finally{el('fw-load-raw').disabled=false}
  })
  el('fw-add-dialog').addEventListener('close',async()=>{
    const action = el('fw-add-dialog').returnValue
    el('fw-add-dialog').returnValue = ''
    if(action!=='apply')return
    const port=Number(el('fw-add-port').value)
    const protocol=el('fw-add-proto').value,zone=el('fw-add-zone').value
    if(!Number.isInteger(port)||port<1||port>65535)return toast('端口必须是 1–65535 的整数',true)
    const policy=el('fw-add-policy').value
    if(policy!=='allow')return toast('拒绝策略仍在隔离验证中，当前不会错误地创建允许规则',true)
    if(!window.confirm('确认在系统防火墙中放行 '+port+'/'+protocol+' 吗？'))return
    try{await request('/api/firewall/rules','POST',{action:'add',port,protocol,zone});toast('规则已添加');await refresh()}
    catch(err){toast(err.message,true)}
  })
  el('fw-detail-dialog').addEventListener('close',async()=>{
    const r=state.detail
    const action = el('fw-detail-dialog').returnValue
    el('fw-detail-dialog').returnValue = ''
    if(action!=='remove'||!r)return
    if(!r.managed||r.protected||!editable())return toast('规则受保护或不属于 X-port，不能删除',true)
    if(!window.confirm('删除 X-port 创建的 '+r.port+'/'+r.protocol+' 放行规则？请先确认该端口没有正在运行的服务。'))return
    try{
      const result=await request('/api/firewall/rules','POST',{action:'remove',port:r.port,protocol:r.protocol,zone:r.zone||''})
      await refresh()
      if(result?.pending) {
        if(window.confirm('端口规则已临时关闭，约 120 秒后自动恢复。请确认仍能访问管理面板，然后点「确定」永久确认删除；取消则等待自动恢复。')) {
          await request('/api/firewall/rules/confirm','POST',{id:result.id,token:result.token})
          toast('删除已确认，恢复任务已取消');await refresh()
        }else toast('未确认：恢复任务将在期限到达后重新开放该端口')
      }
    }
    catch(err){toast(err.message,true)}
  })
  const nav=document.querySelector('#nav [data-page="firewall"]')
  if(nav)nav.addEventListener('click',()=>{if(!document.getElementById('app').classList.contains('hidden'))void refresh()})
  setTab('rules')
})()
