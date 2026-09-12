/* X-port v0.1.7: small account-editor alignment polish. */
const renderExpertV3BaseV8=renderExpertV3
renderExpertV3=function(d){
 let html=renderExpertV3BaseV8(d)
 html=html.replace(
  /<label class="expert-check full"><input id="expert-proxy" type="checkbox"([^>]*)>接收 PROXY Protocol<small class="field-hint">([^<]*)<\/small><\/label>/,
  '<label class="expert-check full proxy-protocol-row"><span class="proxy-protocol-copy"><b>接收 PROXY Protocol</b><small class="field-hint">$2</small></span><input id="expert-proxy" type="checkbox"$1 aria-label="接收 PROXY Protocol"></label>'
 )
 return html
}
