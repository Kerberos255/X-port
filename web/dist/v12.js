(() => {
  function toastV12(msg, bad = false) {
    const e = $('#toast')
    if (!e) return
    e.replaceChildren()
    const icon = document.createElement('span')
    icon.className = 'toast-icon'
    icon.textContent = bad ? '!' : '✓'
    const text = document.createElement('span')
    text.className = 'toast-message'
    text.textContent = String(msg ?? '')
    e.append(icon, text)
    e.classList.remove('hidden')
    e.classList.toggle('bad', bad)
    clearTimeout(toastV12.t)
    toastV12.t = setTimeout(() => e.classList.add('hidden'), bad ? 6500 : 3000)
  }

  async function openCloneModalV12(id) {
    const a = state.accounts.find(x => x.id === id)
    openModal(`<div class="modal-title"><span>CLONE ACCOUNT</span><h2>克隆 ${esc(a?.name || '账号')}</h2></div><form id="clone-form" class="form-grid"><label>新账号名<input name="name" value="${esc((a?.name || 'account') + ' copy')}" required></label><label>新端口<input id="clone-port" name="port" type="number" min="1" max="65535" placeholder="正在随机分配…"></label><p id="clone-port-note" class="muted tiny full">正在从设定端口范围内随机选择一个空闲端口…</p><p class="muted tiny full">协议与传输参数会复制；UUID/密码自动重新生成，本期与累计流量清零。</p><div class="form-actions full"><button type="button" class="btn ghost" id="clone-cancel">取消</button><button class="btn primary" type="submit">克隆并应用</button></div></form>`)
    $('#clone-cancel').onclick = closeModal

    const portInput = $('#clone-port')
    const portNote = $('#clone-port-note')
    try {
      const d = await api('/api/accounts/port-suggestion')
      if (!$('#clone-form')) return
      portInput.value = d.port || ''
      portNote.textContent = `已自动从 ${d.min}-${d.max} 随机选择空闲端口；也可以手动修改。`
    } catch (x) {
      if ($('#clone-form')) {
        portNote.textContent = '自动分配失败，可手动填写；留空提交时后端仍会尝试自动分配。'
      }
      toastV12(x.message, true)
    }

    $('#clone-form').onsubmit = async e => {
      e.preventDefault()
      const form = e.currentTarget
      const f = new FormData(form)
      const button = form.querySelector('button[type=submit]')
      button.disabled = true
      button.textContent = '克隆中…'
      try {
        await api(`/api/accounts/${id}/clone`, {
          method: 'POST',
          body: JSON.stringify({name: f.get('name'), port: Number(f.get('port') || 0)})
        })
        closeModal()
        await loadAccounts()
        await loadOverview()
        toastV12('账号已克隆')
      } catch (x) {
        toastV12(x.message, true)
        button.disabled = false
        button.textContent = '克隆并应用'
      }
    }
  }

  toast = toastV12
  openCloneModal = openCloneModalV12
})()
