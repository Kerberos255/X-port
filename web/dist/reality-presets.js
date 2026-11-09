/* REALITY target presets only fill editable target/SNI fields; never rotate keys. */
const realityPresetsV1=[
  {id:'microsoft',label:'Microsoft · www.microsoft.com',name:'www.microsoft.com'},
  {id:'apple',label:'Apple · www.apple.com',name:'www.apple.com'},
  {id:'mozilla',label:'Mozilla · www.mozilla.org',name:'www.mozilla.org'}
]
const renderExpertBeforeRealityPresets=renderExpertV3
renderExpertV3=(...args)=>{
  const markup=renderExpertBeforeRealityPresets(...args)
  const template=document.createElement('template')
  template.innerHTML=markup
  const target=template.content.querySelector('#expert-reality-target')
  const names=template.content.querySelector('#expert-reality-names')
  if(!target||!names)return markup
  const field=template.content.createElement?.('label')||document.createElement('label')
  field.className='reality-template-field full'
  field.textContent='伪装目标模板'
  const select=document.createElement('select')
  select.id='expert-reality-template'
  const initialName=String(names.getAttribute('value')||'').trim()
  const initialTarget=String(target.getAttribute('value')||'').trim()
  const initial=realityPresetsV1.find(p=>initialTarget===p.name+':443'&&initialName===p.name)
  select.innerHTML='<option value="">自定义 / 当前配置</option>'+
    realityPresetsV1.map(p=>'<option value="'+p.id+'">'+p.label+'</option>').join('')
  select.value=initial?.id||''
  field.append(select)
  target.closest('label')?.before(field)
  return template.innerHTML
}
document.addEventListener('change',e=>{
  if(e.target?.id!=='expert-reality-template')return
  const preset=realityPresetsV1.find(p=>p.id===e.target.value)
  if(!preset)return
  const target=document.getElementById('expert-reality-target')
  const names=document.getElementById('expert-reality-names')
  if(!target||!names)return
  e.target.dataset.applying='1'
  target.value=preset.name+':443'
  names.value=preset.name
  target.dispatchEvent(new Event('input',{bubbles:true}))
  names.dispatchEvent(new Event('input',{bubbles:true}))
  delete e.target.dataset.applying
})
document.addEventListener('input',e=>{
  if(e.target?.id!=='expert-reality-target'&&e.target?.id!=='expert-reality-names')return
  const template=document.getElementById('expert-reality-template')
  if(!template||template.dataset.applying)return
  const target=String(document.getElementById('expert-reality-target')?.value||'').trim()
  const names=String(document.getElementById('expert-reality-names')?.value||'').trim()
  template.value=realityPresetsV1.find(p=>target===p.name+':443'&&names===p.name)?.id||''
})
