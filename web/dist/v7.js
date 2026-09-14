/* X-port v0.1.7: retained expert-row compatibility only.
   The former account-form override is superseded by the later v9/v10 editor implementation. */

/* Folded from the former v8 layer; kept here in the same execution position. */
const renderExpertV3BaseV8=renderExpertV3
renderExpertV3=function(d){
 let html=renderExpertV3BaseV8(d)
 html=html.replace(
  /<label class="expert-check full"><input id="expert-proxy" type="checkbox"([^>]*)>接收 PROXY Protocol<small class="field-hint">([^<]*)<\/small><\/label>/,
  '<label class="expert-check full proxy-protocol-row"><span class="proxy-protocol-copy"><b>接收 PROXY Protocol</b><small class="field-hint">$2</small></span><input id="expert-proxy" type="checkbox"$1 aria-label="接收 PROXY Protocol"></label>'
 )
 return html
}
