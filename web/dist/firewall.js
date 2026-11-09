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
  function status(r) { return r.pausedDrift?'状态冲突':r.paused?'已暂停':r.runtime !== r.permanent ? '需检查' : (r.runtime ? '已配置' : '未生效') }
  function stateBadge(r) { return '<span class="fw-state '+(r.runtime !== r.permanent ? 'warn' : '')+'">'+status(r)+'</span>' }
  function portText(r){return r.rangeStart && r.rangeEnd ? r.rangeStart+'–'+r.rangeEnd : String(r.port)}
  function addressFamily(r) {
    if(r.family==='ipv4'||r.family==='ipv6')return r.family
    if(r.backend==='ufw'&&r.ipv4&&!r.ipv6)return 'ipv4'
    if(r.backend==='ufw'&&r.ipv6&&!r.ipv4)return 'ipv6'
    return 'both'
  }
  function addressText(r){
    const family=addressFamily(r)
    return family==='ipv4'?'0.0.0.0/0':family==='ipv6'?'::/0':'0.0.0.0/0, ::/0'
  }
  function ipCell(r){
    const family=addressFamily(r)
    const ips=family==='ipv4'?['0.0.0.0/0']:family==='ipv6'?['::/0']:['0.0.0.0/0','::/0']
    return '<div class="fw-ip-cell" aria-label="来源地址；不代表本机绑定 IP">'+
      ips.map(ip=>'<span title="'+html(ip)+'">'+html(ip)+'</span>').join('')+'</div>'
  }
  function usageDescription(d){return d.container ? 'Docker: '+(d.name||'容器')+' ('+d.port+')' :
    (d.name || '进程未识别')+(d.pid ? ' ('+d.pid+')':'')+' · '+d.port+'/'+d.protocol}
  function usageBadge(r) {
    const entries=Array.isArray(r.usageDetails)?r.usageDetails:[]
    if(entries.length){
      const first=entries[0]
      const dockerOnly=entries.every(d=>d.container)
      const title=entries.map(usageDescription).join('；')
      const full=title+(r.usageOverflow ? '；另外 '+r.usageOverflow+' 条' : '')
      return '<button class="fw-usage fw-usage-link used" type="button" data-fw-act="detail" data-fw-id="'+html(r.id)+'" title="'+html(full)+'">'+
        '<span class="fw-usage-dot"></span><span class="fw-usage-owner">'+html(dockerOnly?'Docker: '+first.name:(first.name||'进程未识别'))+
        '</span><small>('+html(first.pid||first.port)+')</small></button>'+
        (entries.length+r.usageOverflow>1?'<span class="fw-usage-extra">+'+(entries.length+r.usageOverflow-1)+'</span>':'')
    }
    if(r.usage==='used')return '<span class="fw-usage used"><span class="fw-usage-dot"></span>已监听 · 进程未识别</span>'
    if(r.usage==='unused')return '<span class="fw-usage idle"><span class="fw-usage-dot"></span>未发现监听</span>'
    return '<span class="fw-usage unknown"><span class="fw-usage-dot"></span>无法检测</span>'
  }
  function needsCheck(r) {
    return Boolean(r.conflict)||Boolean(r.pausedDrift)|| r.runtime !== r.permanent ||
      r.usage === 'unknown' ||
      Boolean(r.accountAmbiguous) ||
      Boolean(r.xray && r.accountName && r.accountEnabled && r.usage === 'unused') ||
      Boolean(r.xray && r.accountName && !r.accountEnabled && r.usage === 'used')
  }
  function policy(r) {
    if(r.conflict)return 'conflict'
    if(r.action==='deny'||r.action==='drop')return 'deny'
    if(r.action==='reject')return 'reject'
    if(r.kind==='rich'&&r.action!=='accept'&&r.action!=='allow')return 'unknown'
    return 'allow'
  }
  function policyBadge(r) {
    const p=policy(r)
    const labels={allow:'允许',deny:'拒绝 · DROP',reject:'拒绝 · REJECT',conflict:'策略冲突',unknown:'需检查'}
    return '<span class="fw-policy '+p+'">'+(labels[p]||labels.unknown)+'</span>'
  }
  function accountCell(r) {
    const title=r.accountName ? html(r.accountName) : html(label(r))
    const subtitle=r.accountAmbiguous ? '多个账号匹配 · 请检查传输与端口'
      : r.accountName ? ('Xray · '+html(r.purpose||'账号')+(r.accountEnabled?'':' · 账号已停用'))
      : (r.xray?'Xray · '+html(r.protocol.toUpperCase()):html(r.access||'系统端口'))
    const linked = r.accountName && Number.isInteger(r.accountId) && r.accountId>0
    return '<div class="fw-account-cell">'+(linked?'<button type="button" class="fw-account-link" data-fw-act="account" data-fw-account="'+r.accountId+'" title="前往 Xray 账号管理">'+title+'</button>':'<strong>'+title+'</strong>')+'<small>'+subtitle+'</small></div>'
  }
  function ruleToggle(r) {
    const active=Boolean(!r.paused&&r.runtime&&r.permanent)
    const locked=Boolean(!r.canToggle||r.conflict||r.pausedDrift||!editable())
    return '<button type="button" class="fw-switch '+(active?'on':'')+'" role="switch" aria-checked="'+active+'" aria-label="'+html(portText(r)+'/'+r.protocol)+' 规则生效" data-fw-act="toggle" data-fw-id="'+html(r.id)+'" aria-disabled="'+locked+'" '+(locked?'disabled title="外部、管理端口、范围或冲突规则受保护"':'title="切换此规则的生效状态（120 秒安全恢复）"')+'><span></span></button>'+(locked?'<small class="fw-switch-caption">'+(r.rangeStart?'范围只读':'保护中')+'</small>':'')
  }
  function source(r) { return (r.managed ? 'X-port · ' : '外部 · ') + (r.backend === 'firewalld' ? 'firewalld · '+r.zone : r.backend) }
  function shortSource(r) { return (r.managed ? 'X-port · ' : '外部 · ') + r.backend }
  function shown() {
    return rules().filter(r => {
      if (state.filter === 'xray' && !r.xray) return false
      if (state.filter === 'warn' && !needsCheck(r)) return false
      const term = state.term.toLowerCase().trim()
      return !term || [r.port,r.rangeStart,r.rangeEnd,r.protocol,addressText(r),r.family,r.purpose,r.accountName,r.note,r.zone,r.access,r.backend,r.usage,
        ...(r.usageDetails||[]).map(d=>d.name)].join(' ').toLowerCase().includes(term)
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
    el('fw-engine-state').textContent = !data ? '—' : (!data.writable ? (data.active ? '只读 · 检查' : '只读 / 未启用') : (data.discoveryWarning ? '部分危险操作受限' : '● 可管理'))
    const diagnostic=el('fw-diagnostic')
    diagnostic.hidden = !data?.unsafeReason && !data?.discoveryWarning
    el('fw-diagnostic-title').textContent = data?.unsafeReason ? '规则修改已暂停' : '危险操作已锁定'
    el('fw-diagnostic-reason').textContent = data?.unsafeReason || data?.discoveryWarning || ''
    el('fw-total').textContent = rules().length
    el('fw-active').textContent = rules().filter(r=>r.usage==='used').length
    el('fw-xray-count').textContent = rules().filter(r=>r.xray).length
    el('fw-warn-count').textContent = rules().filter(needsCheck).length + (data?.unsafeReason || data?.discoveryWarning ? 1 : 0)
    el('fw-add').disabled = !editable()
    el('fw-notice').textContent = (data?.notice || '无法读取防火墙状态')+
      (data?.discoveryWarning ? ' · '+data.discoveryWarning : '')
    const pending = Array.isArray(data?.pending) ? data.pending : []
    let pendingBox = el('fw-pending')
    if (!pendingBox) {
      pendingBox = document.createElement('div')
      pendingBox.id = 'fw-pending'
      el('fw-notice').before(pendingBox)
    }
    pendingBox.innerHTML = pending.map(p => {
      const active = Number(p.deadline) > Date.now()
      return '<div class="fw-pending-item"><span>↺ '+html(p.id)+' · '+(active ? ((p.kind==='add-drop'?'拒绝策略未确认，届时自动撤销':p.kind==='pause-drop'?'拒绝暂停待确认，届时自动恢复拒绝':p.kind==='pause-allow'?'放行暂停待确认，届时自动恢复放行':'等待连接确认，超时自动恢复')) : '恢复期限已过，请刷新检查规则')+'</span>' +
        (active ? '<button type="button" class="btn fw-mini" data-fw-act="confirm-pending" data-fw-id="'+html(p.id)+'" data-fw-token="'+html(p.token)+'">确认连接正常</button>' : '')+'</div>'
    }).join('')
    const filtered = shown()
    el('fw-count').textContent = '显示 '+filtered.length+' / '+rules().length+' 条规则；备注与用途保存在 X-port 数据库中'
    el('fw-rows').innerHTML = filtered.length ? filtered.map(r => '<tr>'+
      '<td data-label="端口 / 协议"><div class="fw-port-cell"><strong title="'+html(portText(r))+'">'+html(portText(r))+'</strong><small>'+html(r.protocol.toUpperCase())+'</small></div></td>'+
      '<td data-label="IP / 地址族">'+ipCell(r)+'</td>'+
      '<td data-label="关联账号 / 用途">'+accountCell(r)+'</td>'+
      '<td data-label="使用情况">'+usageBadge(r)+'</td>'+
      '<td data-label="策略">'+policyBadge(r)+'</td>'+
      '<td data-label="来源"><span class="fw-source" title="'+html(source(r))+'">'+html(shortSource(r))+'</span></td>'+
      '<td data-label="规则生效"><div class="fw-toggle-cell">'+ruleToggle(r)+'</div></td>'+
      '<td data-label="备注">'+fieldButton(r,'note')+'</td>'+
      '<td data-label="操作"><div class="fw-actions"><button class="btn fw-mini ghost" data-fw-act="configure" data-fw-id="'+html(r.id)+'">编辑</button><button class="btn fw-mini ghost" data-fw-act="detail" data-fw-id="'+html(r.id)+'">详情</button></div></td>'+
      '</tr>').join('') : '<tr><td colspan="9" class="muted">暂无匹配的规则。</td></tr>'
    el('fw-xray-list').innerHTML = rules().filter(r=>r.xray).map(r => '<div class="fw-xray-line"><div><b>'+html(label(r))+'</b><small class="fw-sub">'+html(r.port)+' / '+html(r.protocol.toUpperCase())+' · '+html(source(r))+'</small></div>'+stateBadge(r)+'</div>').join('') || '<p class="fw-note">目前没有匹配到 Xray 账号的简单端口规则。</p>'
    if (state.editing) { const input = root.querySelector('.fw-edit-input'); if (input) {input.focus(); input.select()} }
  }
  async function refresh(strict=false) {
    const seq = ++state.sequence
    el('fw-refresh').disabled = true
    try {
      const data = await api('/api/firewall')
      if (seq !== state.sequence) return
      state.data = data
      render()
      return data
    } catch (err) {
      if (seq !== state.sequence) return
      state.data = null
      render()
      el('fw-notice').textContent = '读取失败：'+err.message
      if(strict)throw err
      toast('防火墙状态读取失败：'+err.message,true)
    } finally { if (seq === state.sequence) el('fw-refresh').disabled = false }
  }
  // Keep the progress dialog active until the new firewall snapshot is read.
  // A returned write is not proof that the visible rule list is up to date.
  const applyRule=(title,detail,perform,check)=>XportOperation.run({
    title,detail,perform,
    verify:async result=>{
      await readConfirmedStateV3(async()=>{
        const data=await refresh(true)
        return Boolean(data&&(!check||check(data,result)))
      },{attempts:5,delayMs:650})
    }
  })
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
      await applyRule(edit.key==='note'?'正在保存规则备注':'正在更新规则用途','规则信息 · '+edit.id,
        async()=>{
          const result=await request('/api/firewall/'+endpoint,'PUT',{id:edit.id,[edit.key]:value})
          state.editing=null
          return result
        },data=>data.rules?.some(r=>r.id===edit.id&&String(r[edit.key]||'')===value))
      toast('保存成功')
    } catch(err) {toast(err.message,true)}
  }
  function canDeleteExternal(r){
    return Boolean(r&&!r.managed&&!r.protected&&!r.rangeStart&&
      r.kind==='port'&&r.action==='allow'&&r.runtime&&r.permanent&&
      !r.conflict&&!r.paused&&r.backend==='firewalld'&&
      !state.data?.opaqueRichZones?.[r.zone]&&
      typeof r.revision==='string'&&r.revision.length===24&&editable())
  }
  function openDetail(id) {
    const r=rules().find(x=>x.id===id)
    if(!r)return
    state.detail=r
    el('fw-detail-title').textContent=portText(r)+' / '+r.protocol.toUpperCase()
    const badge=el('fw-detail-badge')
    badge.textContent=status(r)
    badge.classList.toggle('warn',r.runtime!==r.permanent)
    el('fw-detail-summary').textContent=r.rangeStart
      ? '这是端口范围规则，仅展示配置和监听情况。范围规则不能通过当前页面删除或修改。'
      : r.managed
      ? '由 X-port 创建的端口规则；修改前仍需检查服务和网络连通性。'
      : canDeleteExternal(r)?'外部普通放行规则；确认后可删除，120 秒内未确认将自动恢复。':'由系统或其他管理器维护，当前规则类型或状态不满足安全删除条件。'
    const groups=[
      ['基本信息',[['用途',label(r)],['来源 IP',addressText(r)],['关联账号',r.accountAmbiguous?'多个候选，无法唯一定位':(r.accountName||'无关联')],['监听状态',r.usage==='used'?'检测到相关监听或发布端口':r.usage==='unused'?'未检测到同协议监听':'无法确认'],['备注',r.note||'未填写'],['规则来源',source(r)],['访问来源',r.access||'未确认']]],
      ['规则状态',[['策略',policy(r)==='deny'?'拒绝 · DROP':policy(r)==='reject'?'拒绝 · REJECT':policy(r)==='conflict'?'存在重叠策略':policy(r)==='allow'?'允许':'需检查'],['运行时',r.runtime?'已配置':'不存在'],['永久配置',r.permanent?'已配置':'不存在'],['地址族',r.family==='both'?'IPv4 + IPv6':r.family==='ipv4'?'IPv4':r.family==='ipv6'?'IPv6':r.backend==='ufw'?(r.ipv4?'IPv4 ':'')+(r.ipv6?'IPv6':''):'由 firewalld Zone 决定']]],
      ['关联进程与容器',[['已识别',(r.usageDetails||[]).map(usageDescription).join('；')||'暂无'],['其他关联',r.usageOverflow ? '另外 '+r.usageOverflow+' 条' : '无'],['端口范围',r.rangeStart ? portText(r)+'/'+r.protocol : '单端口']]],
      ['安全保护',[['规则归属',r.managed?'X-port 管理':canDeleteExternal(r)?'外部规则（允许删除）':'外部或未知（受保护）'],['端口保护',r.protected?'已启用':'未检测到本机监听']]]
    ]
    el('fw-detail-body').innerHTML=groups.map(([heading,fields])=>
      '<section class="fw-detail-section"><h3>'+html(heading)+'</h3><dl>'+
      fields.map(([name,value])=>'<div class="fw-detail-row"><dt>'+html(name)+'</dt><dd>'+html(value)+'</dd></div>').join('')+
      '</dl></section>'
    ).join('')
    el('fw-detail-remove').hidden = !editable() || r.protected || Boolean(r.rangeStart) || (!r.managed&&!canDeleteExternal(r))
    el('fw-detail-dialog').returnValue = ''
    el('fw-detail-dialog').showModal()
  }
  async function openLinkedAccount(rawId){
    const id=Number(rawId)
    const nav=document.querySelector('#nav [data-page="accounts"]')
    if(!Number.isSafeInteger(id)||id<=0||!nav)return toast('账号关联已失效，请刷新防火墙页面',true)
    nav.click()
    // Navigation asynchronously reloads account data. Resolve the specific
    // account by immutable ID, never by display name or an old row index.
    await loadAccounts()
    const row=document.querySelector('#accounts-list .account-row[data-id="'+id+'"]')
    const edit=row?.querySelector('button[data-act="edit"]')
    if(!edit)return toast('未找到关联账号，可能已被删除或迁移',true)
    row.scrollIntoView({block:'nearest'})
    edit.click()
  }
  let policyChangeBusy=false
  function policyChangeConfirmed(data,result,action,rule){
    if(result?.pending)
      return Boolean(result.id&&result.token&&data.pending?.some(
        item=>item.id===result.id&&item.token===result.token))
    const actual=data.rules?.find(item=>item.id===rule.id)
    if(action==='resume-allow')return Boolean(actual&&!actual.paused&&actual.runtime&&actual.permanent&&actual.action==='allow')
    if(action==='resume-drop'||action==='add-drop')
      return Boolean(actual&&!actual.paused&&actual.runtime&&actual.permanent&&actual.action==='deny'&&actual.kind==='rich')
    if(action==='remove-drop')return !actual
    // Mutations that delete connectivity must have a persisted native recovery timer.
    return false
  }
  async function runPolicyControl(action,r){
    if(policyChangeBusy||!editable())return
    const warning=action==='add-drop'||action==='resume-drop'
      ? '拒绝策略会影响端口访问。确认当前不是管理入口，且已有备用 SSH 连接。'
      : action==='pause-allow'
      ? '暂停允许规则可能中断该端口的服务。120 秒内未确认将自动恢复原放行规则。'
      : action==='pause-drop'
      ? '暂停拒绝规则将临时开放访问。120 秒内未确认将自动恢复拒绝。'
      : action==='remove-drop'
      ? '永久删除拒绝规则可能扩大外部访问。确认仍要继续？'
      : '确认恢复这条规则的原有策略？'
    if(!await XportOperation.ask({title:'确认修改防火墙策略',detail:warning,
      confirmText:'继续修改',danger:true}))return
    policyChangeBusy=true
    try{
      const result=await applyRule('正在调整防火墙策略',portText(r)+' / '+r.protocol.toUpperCase(),
        ()=>request('/api/firewall/control','POST',{
          action,id:r.id,port:r.port,protocol:r.protocol,zone:r.zone||''
        }),(data,result)=>policyChangeConfirmed(data,result,action,r))
      if(result?.pending){
        const yes=await XportOperation.ask({title:'请确认连接仍然正常',
          detail:'规则已临时修改，请先验证管理面板仍可访问。\n若不确认，系统将在约 120 秒后自动恢复。',
          confirmText:'保留修改',cancelText:'等待恢复'})
        if(yes){
          await applyRule('正在确认防火墙规则','取消定时恢复任务',
            ()=>request('/api/firewall/rules/confirm','POST',{id:result.id,token:result.token}),
            data=>!data.pending?.some(item=>item.id===result.id))
          toast('规则已确认，外部恢复计时器已取消')
        }else toast('未确认，系统将按计时器恢复原规则')
      }else toast('规则已恢复')
    }catch(err){toast('规则操作未确认：'+err.message,true);await refresh()}
    finally{policyChangeBusy=false}
  }
  let selectedEditRule=null
  let externalEditBusy=false
  function canEditExternal(r){
    return Boolean(r&&!r.managed&&!r.rangeStart&&r.kind==='port'&&r.action==='allow'&&
      r.runtime&&r.permanent&&!r.conflict&&!r.protected&&
      !r.paused&&r.backend==='firewalld'&&
      !state.data?.opaqueRichZones?.[r.zone]&&
      typeof r.revision==='string'&&r.revision.length===24&&editable())
  }
  function updateExternalSaveState(){
    const r=selectedEditRule
    const allowed=canEditExternal(r)
    const port=Number(el('fw-policy-port').value)
    const protocol=el('fw-policy-proto').value
    const different=r&&(port!==r.port||protocol!==r.protocol)
    el('fw-policy-save').disabled=!(allowed&&el('fw-external-confirm').checked&&
      Number.isInteger(port)&&port>=1&&port<=65535&&different&&
      el('fw-policy-action').value==='allow')
  }
  async function saveExternalEdit(r){
    if(externalEditBusy||!canEditExternal(r))return
    const port=Number(el('fw-policy-port').value),proto=el('fw-policy-proto').value
    if(!Number.isInteger(port)||port<1||port>65535||!['tcp','udp'].includes(proto)||
      el('fw-policy-action').value!=='allow'||!el('fw-external-confirm').checked){
      return toast('外部规则只支持明确确认后的简单端口/协议修改',true)
    }
    if(!await XportOperation.ask({title:'确认修改外部规则',
      detail:'这条规则由其他管理器维护。\n修改真实 firewalld 后，原管理器的备注不会自动迁移。',
      confirmText:'继续修改',danger:true}))return
    externalEditBusy=true
    try{
      const result=await applyRule('正在修改防火墙端口',port+' / '+proto.toUpperCase(),
        ()=>request('/api/firewall/edit','POST',{
          id:r.id,revision:r.revision,newPort:port,newProtocol:proto,
          externalAcknowledged:true
        }),(data,result)=>result?.pending
          ?data.pending?.some(p=>p.id===result.id)
          :data.rules?.some(row=>Number(row.port)===port&&row.protocol===proto&&
            (row.zone||'')===(r.zone||'')))
      if(result?.pending){
        if(await XportOperation.ask({title:'请确认新端口可用',
          detail:'端口已临时修改。请确认仍能访问管理面板。\n不确认将在约 120 秒后自动恢复原端口。',
          confirmText:'保留新端口',cancelText:'等待恢复'})){
          await applyRule('正在确认端口修改','取消定时恢复任务',
            ()=>request('/api/firewall/rules/confirm','POST',{id:result.id,token:result.token}),
            data=>!data.pending?.some(item=>item.id===result.id))
          toast('外部规则已修改。请刷新原防火墙管理器确认新端口。')
        }else toast('未确认：系统将自动恢复原端口并撤销新端口')
      }else toast('外部规则已修改并核对新端口，请在原管理器中确认')
    }catch(err){toast('外部规则编辑未完成：'+err.message,true);await refresh()}
    finally{externalEditBusy=false;selectedEditRule=null}
  }
  function openRuleEditor(id){
    const r=rules().find(row=>row.id===id)
    if(!r)return
    selectedEditRule=r
    const allowed=canEditExternal(r)
    el('fw-policy-title').textContent='编辑 '+portText(r)+'/'+r.protocol.toUpperCase()
    el('fw-policy-subtitle').textContent=r.managed?'X-port 自管规则（端口请在原账号配置中调整）':'外部规则 · 共享 firewalld'
    el('fw-policy-port').value=r.rangeStart?'':r.port
    el('fw-policy-port').placeholder=r.rangeStart?portText(r)+'（范围只读）':'端口号'
    el('fw-policy-port').disabled=!allowed
    el('fw-policy-proto').disabled=!allowed
    el('fw-policy-proto').value=r.protocol
    el('fw-policy-action').value=policy(r)
    el('fw-policy-action').disabled=true
    el('fw-policy-zone').value=r.zone||'UFW'
    el('fw-external-confirm-label').hidden=!allowed
    el('fw-external-confirm').checked=false
    el('fw-policy-protection').textContent=allowed
      ? '仅支持无监听的简单允许规则修改端口与协议。120 秒内必须确认连接；原管理器备注不会自动迁移。'
      : r.protected?'检测到实际监听或管理端口，危险编辑已锁定。':
        r.managed?'Xray/自管规则应通过其原有配置界面修改端口。':
        '端口范围、复杂策略、端口冲突或无法验证状态，暂不支持直接修改。'
    el('fw-policy-save').disabled=true
    el('fw-policy-save').title=allowed?'勾选外部规则确认后保存':'此规则不符合安全编辑条件'
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
  root.addEventListener('click',async ev=>{
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
      if (!await XportOperation.ask({title:'确认管理连接正常',
        detail:'请确认当前仍可访问管理面板。确认后将取消防火墙定时恢复。',
        confirmText:'确认连接正常'})) return
      void applyRule('正在确认防火墙规则','取消定时恢复任务',
        ()=>request('/api/firewall/rules/confirm','POST',{id,token:button.dataset.fwToken}),
        data=>!data.pending?.some(item=>item.id===id))
        .then(()=>toast('连接已确认，定时恢复已取消'))
        .catch(err=>toast(err.message,true))
      return
    }
    if(act==='account'){void openLinkedAccount(button.dataset.fwAccount);return}
    if(act==='toggle'){
      const rule=rules().find(x=>x.id===id)
      if(!rule||!rule.canToggle||!editable())return toast('此规则由外部管理或状态不确定，无法切换',true)
      const action=rule.paused
        ? (rule.action==='deny'?'resume-drop':'resume-allow')
        : (rule.action==='deny'?'pause-drop':rule.kind==='port'?'pause-allow':'')
      if(!action)return toast('当前规则类型暂不支持启停',true)
      void runPolicyControl(action,rule);return
    }
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
  for(const id of ['fw-policy-port','fw-policy-proto','fw-policy-action','fw-external-confirm']){
    el(id).addEventListener('input',updateExternalSaveState)
    el(id).addEventListener('change',updateExternalSaveState)
  }
  el('fw-policy-dialog').querySelector('form').addEventListener('submit',ev=>{
    ev.preventDefault()
    const action=ev.submitter?.value||''
    const rule=selectedEditRule
    el('fw-policy-dialog').close()
    if(action!=='save'){selectedEditRule=null;return}
    if(rule)void saveExternalEdit(rule)
  })
  el('fw-load-raw').addEventListener('click',async ()=>{
    el('fw-load-raw').disabled=true
    el('fw-raw').textContent='读取中…'
    try{const data=await api('/api/firewall/raw');el('fw-raw').textContent=data.ruleset||'规则集为空'}
    catch(err){el('fw-raw').textContent='读取失败：'+err.message+'。请在系统页检查 X-port 服务单元兼容状态；不要直接覆盖现有自定义服务文件。'}
    finally{el('fw-load-raw').disabled=false}
  })
  // Handle the actual form submission rather than the deferred dialog close
  // event. This keeps confirmation and validation tied to the user's click.
  el('fw-add-dialog').querySelector('form').addEventListener('submit',async ev=>{
    ev.preventDefault()
    const action=ev.submitter?.value||''
    el('fw-add-dialog').close()
    if(action!=='apply')return
    const port=Number(el('fw-add-port').value)
    const protocol=el('fw-add-proto').value,zone=el('fw-add-zone').value
    if(!Number.isInteger(port)||port<1||port>65535)return toast('端口必须是 1–65535 的整数',true)
    const policy=el('fw-add-policy').value
    if(policy!=='allow'){
      if(state.data?.backend!=='firewalld')return toast('拒绝策略目前仅支持经过验证的 firewalld 后端',true)
      const id='firewalld:'+zone+':rich:'+port+'/'+protocol+':deny:both'
      void runPolicyControl('add-drop',{id,port,protocol,zone,action:'deny'})
      return
    }
    if(!await XportOperation.ask({title:'确认添加放行规则',
      detail:'将在系统防火墙放行 '+port+'/'+protocol.toUpperCase()+' · '+zone+'。',
      confirmText:'确认添加'}))return
    try{
      await applyRule('正在添加防火墙规则',port+' / '+protocol.toUpperCase()+' · '+zone,
        ()=>request('/api/firewall/rules','POST',{action:'add',port,protocol,zone}),
        data=>data.rules?.some(r=>Number(r.port)===port&&r.protocol===protocol&&
          (r.zone||'')===zone&&(r.action==='allow'||r.action==='accept'||!r.action)))
      toast('规则已添加')
    }catch(err){toast('添加结果未确认：'+err.message,true);await refresh()}
  })
  el('fw-detail-dialog').querySelector('form').addEventListener('submit',async ev=>{
    ev.preventDefault()
    const r=state.detail
    const action=ev.submitter?.value||''
    el('fw-detail-dialog').close()
    if(action!=='remove'||!r)return
    if(r.protected||!editable()||(!r.managed&&!canDeleteExternal(r)))return toast('规则受保护或不符合安全删除条件',true)
    if(r.kind==='rich'&&r.action==='deny'){
      void runPolicyControl('remove-drop',r)
      return
    }
    if(!await XportOperation.ask({title:'确认删除放行规则',
      detail:(r.managed?'将删除 X-port 创建的':'将删除外部管理器创建的')+' '+portText(r)+'/'+r.protocol.toUpperCase()+' 放行规则。\n未确认则约 120 秒后自动恢复。',
      confirmText:'确认删除',danger:true}))return
    try{
      const result=await applyRule('正在删除防火墙规则',portText(r)+' / '+r.protocol.toUpperCase(),
        ()=>r.managed
          ?request('/api/firewall/rules','POST',{action:'remove',port:r.port,protocol:r.protocol,zone:r.zone||''})
          :request('/api/firewall/external/remove','POST',{id:r.id,revision:r.revision,externalAcknowledged:true}),
        (data,result)=>result?.pending
          ?data.pending?.some(p=>p.id===result.id)
          :!data.rules?.some(x=>x.id===r.id&&x.runtime&&x.permanent))
      if(result?.pending) {
        if(await XportOperation.ask({title:'请确认删除后仍可连接',
          detail:'端口规则已临时关闭。请先确认管理面板仍可访问。\n若不确认，系统约 120 秒后自动恢复放行。',
          confirmText:'确认删除',cancelText:'等待恢复',danger:true})) {
          await applyRule('正在确认删除规则','取消定时恢复任务',
            ()=>request('/api/firewall/rules/confirm','POST',{id:result.id,token:result.token}),
            data=>!data.pending?.some(item=>item.id===result.id))
          toast('删除已确认，恢复任务已取消')
        }else toast('未确认：恢复任务将在期限到达后重新开放该端口')
      }
    }
    catch(err){toast(err.message,true)}
  })
  const nav=document.querySelector('#nav [data-page="firewall"]')
  if(nav)nav.addEventListener('click',()=>{if(!document.getElementById('app').classList.contains('hidden'))void refresh()})
  setTab('rules')
})()
