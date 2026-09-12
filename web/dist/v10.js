/* X-port v0.1.9 preview: compact transport/security layout and effective advanced defaults. */
function protocolFieldsV10(p,a={}){
 p=String(p||'vless').toLowerCase()
 const transport=!['socks','http'].includes(p)
 const network=editorNetworkV9(a.network||'tcp')
 const requestedSecurity=String(a.security||(p==='vless'||p==='trojan'?'reality':'none')).toLowerCase()
 if(!transport){
  let direct=''
  if(p==='socks'||p==='http')direct=editorLabelV9('用户名',`<input name="username" value="${esc(a.username||'')}">`)+editorLabelV9('密码',`<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成">`)
  return `<div class="protocol-auth-only-grid">${direct}</div>`
 }
 const secChoices=editorSecurityChoicesV9(p,network)
 const safeSecurity=secChoices.includes(requestedSecurity)?requestedSecurity:secChoices[0]
 const securityControl=editorLabelV9('传输安全',`<select name="security" id="security-select">${secChoices.map(x=>option(x,x.toUpperCase(),safeSecurity)).join('')}</select>`)
 let auth=''
 if(p==='vless'){
  const canVision=editorVisionCompatibleV9(p,network,safeSecurity)
  const flow=canVision?(a.flow||(a.id?'':'xtls-rprx-vision')):String(a.flow||'')
  auth=editorLabelV9('UUID',`<input name="credential" value="${esc(a.credential||'')}" placeholder="留空自动生成">`)+securityControl+editorLabelV9('Flow',`<select name="flow"><option value="" ${flow===''?'selected':''}>无</option><option value="xtls-rprx-vision" ${flow==='xtls-rprx-vision'?'selected':''}>xtls-rprx-vision</option></select>`,'vless-flow')
 }
 if(p==='vmess')auth=editorLabelV9('UUID',`<input name="credential" value="${esc(a.credential||'')}" placeholder="留空自动生成">`)+securityControl+editorLabelV9('VMess Security',`<select name="clientSecurity">${['auto','aes-128-gcm','chacha20-poly1305','none','zero'].map(x=>option(x,x,a.clientSecurity||'auto')).join('')}</select>`)
 if(p==='trojan')auth=editorLabelV9('Trojan 密码',`<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成">`)+securityControl
 if(p==='shadowsocks')auth=editorLabelV9('加密方式',`<select name="method">${['chacha20-ietf-poly1305','aes-256-gcm','2022-blake3-aes-256-gcm','2022-blake3-chacha20-poly1305'].map(x=>option(x,x,a.method||'chacha20-ietf-poly1305')).join('')}</select>`)+securityControl+editorLabelV9('服务器密码',`<input name="serverPassword" value="${esc(a.serverPassword||'')}" placeholder="留空自动生成">`)+editorLabelV9('客户端密码',`<input name="password" value="${esc(a.password||'')}" placeholder="留空自动生成">`)
 const transportControls=editorLabelV9('传输',`<select name="network" id="network-select">${[['tcp','RAW'],['xhttp','XHTTP'],['grpc','gRPC'],['ws','WebSocket'],['httpupgrade','HTTPUpgrade']].map(([v,l])=>option(v,l,network)).join('')}</select>`)+editorLabelV9('Host',`<input name="host" value="${esc(a.host||'')}">`,'transport-host')+editorLabelV9('Path',`<input name="path" value="${esc(a.path||'')}" placeholder="/">`,'transport-path')+editorLabelV9('gRPC Service',`<input name="serviceName" value="${esc(a.serviceName||'')}">`,'transport-service')
 return `<div class="protocol-config-grid"><div class="protocol-column protocol-auth-column">${auth}</div><div class="protocol-column protocol-transport-column">${transportControls}</div></div>`
}
protocolFieldsV9=protocolFieldsV10
protocolFieldsV6=protocolFieldsV10
protocolFieldsV3=protocolFieldsV10

/* Show Xray's effective defaults in Advanced for fields whose defaults are
   stable and documented. Existing/imported explicit values always win. Merely
   opening Advanced does not rewrite the stored raw config; values become
   explicit only if the user saves the account with Advanced loaded. */
function expertDisplayDefaultsV10(d={}){
 const out={...d}
 if(out.supportsTls){
  if(!Array.isArray(out.tlsAlpn)||out.tlsAlpn.length===0)out.tlsAlpn=['h2','http/1.1']
  if(!out.tlsCertificatesJson)out.tlsCertificatesJson='[]'
 }
 if(out.supportsXhttp){
  if(!out.xhttpMode)out.xhttpMode='auto'
  if(!out.xhttpXPaddingBytes)out.xhttpXPaddingBytes='100-1000'
  if(!out.xhttpScMaxEachPostBytes)out.xhttpScMaxEachPostBytes='1000000'
  if(!(Number(out.xhttpScMaxBufferedPosts)>0))out.xhttpScMaxBufferedPosts=30
  if(!out.xhttpScStreamUpServerSecs)out.xhttpScStreamUpServerSecs='20-80'
  if(!out.xhttpUplinkHttpMethod)out.xhttpUplinkHttpMethod='POST'
 }
 return out
}
const renderExpertV10Base=renderExpertV3
renderExpertV3=function(d){
 let html=renderExpertV10Base(expertDisplayDefaultsV10(d))
 html=html.replace('id="expert-reality-private"','id="expert-reality-private" placeholder="留空，创建时自动生成"')
 html=html.replace('id="expert-reality-shortids"','id="expert-reality-shortids" placeholder="留空，创建时自动生成"')
 return html
}
