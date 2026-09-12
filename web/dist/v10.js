/* X-port v0.1.11: surface XHTTP runtime defaults without persisting them. */
function applyXHTTPDefaultHintsV11(html){
 const template=document.createElement('template')
 template.innerHTML=html
 const hints=[
  ['#expert-xhttp-padding','100-1000（默认）'],
  ['#expert-xhttp-postbytes','1000000（默认）'],
  ['#expert-xhttp-buffered','30（默认）'],
  ['#expert-xhttp-streamsecs','20-80（默认）'],
  ['#expert-xhttp-method','POST（默认）'],
 ]
 for(const [selector,placeholder] of hints){
  const input=template.content.querySelector(selector)
  if(!input)continue
  const current=String(input.getAttribute('value')||'').trim()
  if(current===''||(selector==='#expert-xhttp-buffered'&&current==='0')){
   input.setAttribute('value','')
   input.setAttribute('placeholder',placeholder)
   input.classList.add('xhttp-default-hint')
  }
 }
 return template.innerHTML
}

const renderExpertV3BeforeV11=renderExpertV3
renderExpertV3=(...args)=>applyXHTTPDefaultHintsV11(renderExpertV3BeforeV11(...args))
