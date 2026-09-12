from pathlib import Path
import re


def replace_once(path, old, new):
    p = Path(path)
    text = p.read_text()
    if old not in text:
        raise SystemExit(f"missing replacement target in {path}: {old[:120]!r}")
    p.write_text(text.replace(old, new, 1))


replace_once(
    "internal/server/ux_routes.go",
    '\tmux.Handle("GET /api/accounts/online", s.require(http.HandlerFunc(s.accountOnlineConnections)))',
    '\tmux.Handle("GET /api/accounts/online", s.require(http.HandlerFunc(s.accountOnlineConnections)))\n\tmux.Handle("GET /api/accounts/port-suggestion", s.require(http.HandlerFunc(s.suggestAccountPort)))',
)

replace_once(
    "internal/accountcfg/account.go",
    '\tcase "vless":\n\t\tif in.Credential == "" {\n\t\t\tin.Credential = newUUID()\n\t\t}\n\t\tif in.Flow == "" {\n\t\t\tin.Flow = "xtls-rprx-vision"\n\t\t}\n\t\tif in.Security == "" {\n\t\t\tin.Security = "reality"\n\t\t}',
    '\tcase "vless":\n\t\tif in.Credential == "" {\n\t\t\tin.Credential = newUUID()\n\t\t}\n\t\tif in.Security == "" {\n\t\t\tin.Security = "reality"\n\t\t}\n\t\tif in.Flow == "" && (in.Network == "tcp" || in.Network == "raw") && (in.Security == "tls" || in.Security == "reality") {\n\t\t\tin.Flow = "xtls-rprx-vision"\n\t\t}',
)

replace_once(
    "internal/accountcfg/account.go",
    '\tif in.Protocol == "vmess" && in.ClientSecurity != "" {',
    '\tif in.Security == "reality" {\n\t\tswitch in.Network {\n\t\tcase "tcp", "raw", "xhttp", "grpc":\n\t\tdefault:\n\t\t\treturn fmt.Errorf("REALITY is not supported with %s transport", in.Network)\n\t\t}\n\t}\n\tif in.Protocol == "vless" && in.Flow != "" {\n\t\tif in.Flow != "xtls-rprx-vision" {\n\t\t\treturn fmt.Errorf("unsupported VLESS flow %q", in.Flow)\n\t\t}\n\t\tif (in.Network != "tcp" && in.Network != "raw") || (in.Security != "tls" && in.Security != "reality") {\n\t\t\treturn errors.New("xtls-rprx-vision requires RAW/TCP with TLS or REALITY in the built-in editor")\n\t\t}\n\t}\n\tif in.Protocol == "vmess" && in.ClientSecurity != "" {',
)

replace_once(
    "internal/accountcfg/account.go",
    '\tcase "vless", "vmess", "trojan":\n\t\tclient, err := oneObject(settings, "clients")',
    '\tcase "vless", "vmess", "trojan":\n\t\tif v.Protocol == "vless" {\n\t\t\tdecryption := strings.TrimSpace(stringValue(settings["decryption"]))\n\t\t\tif decryption != "" && decryption != "none" {\n\t\t\t\treturn false\n\t\t\t}\n\t\t}\n\t\tclient, err := oneObject(settings, "clients")',
)

p = Path("internal/service/expert.go")
text = p.read_text()
pattern = r'\tif method == "xhttp" \{\n\t\tkey := expertXHTTPKey\(stream\).*?\n\t\tstream\[key\] = x\n\t\}\n\n\tsettingsJSON'
replacement = '\tif method == "xhttp" {\n\t\tkey := expertXHTTPKey(stream)\n\t\tx := expertObject(stream, key)\n\t\ttarget := x\n\t\tusesExtra := false\n\t\tif extra, ok := x["extra"].(map[string]any); ok && extra != nil {\n\t\t\ttarget, usesExtra = extra, true\n\t\t}\n\t\tmode := strings.TrimSpace(in.XHTTPMode)\n\t\tswitch mode {\n\t\tcase "", "auto", "packet-up", "stream-up", "stream-one":\n\t\tdefault:\n\t\t\treturn ExpertConfig{}, fmt.Errorf("unsupported XHTTP mode %q", mode)\n\t\t}\n\t\tsetExpertString(x, "mode", mode)\n\t\tsetExpertString(target, "xPaddingBytes", in.XHTTPXPaddingBytes)\n\t\tif in.XHTTPNoSSEHeader {\n\t\t\ttarget["noSSEHeader"] = true\n\t\t} else {\n\t\t\tdelete(target, "noSSEHeader")\n\t\t}\n\t\tif in.XHTTPScMaxBufferedPosts > 0 {\n\t\t\ttarget["scMaxBufferedPosts"] = in.XHTTPScMaxBufferedPosts\n\t\t} else {\n\t\t\tdelete(target, "scMaxBufferedPosts")\n\t\t}\n\t\tsetExpertString(target, "scMaxEachPostBytes", in.XHTTPScMaxEachPostBytes)\n\t\tsetExpertString(target, "scStreamUpServerSecs", in.XHTTPScStreamUpServerSecs)\n\t\tsetExpertString(target, "uplinkHTTPMethod", strings.ToUpper(strings.TrimSpace(in.XHTTPUplinkHTTPMethod)))\n\t\tif usesExtra {\n\t\t\tx["extra"] = target\n\t\t}\n\t\tstream[key] = x\n\t}\n\n\tsettingsJSON'
text2, count = re.subn(pattern, replacement, text, count=1, flags=re.S)
if count != 1:
    raise SystemExit(f"expert update xhttp block matches={count}")
text = text2
pattern = r'\tif out\.SupportsXHTTP \{\n\t\tx := expertObject\(stream, expertXHTTPKey\(stream\)\).*?\n\t\}\n\treturn out, nil'
replacement = '\tif out.SupportsXHTTP {\n\t\tx := expertObject(stream, expertXHTTPKey(stream))\n\t\ttarget := x\n\t\tif extra, ok := x["extra"].(map[string]any); ok && extra != nil {\n\t\t\ttarget = extra\n\t\t}\n\t\tout.XHTTPMode = expertString(x["mode"])\n\t\tout.XHTTPXPaddingBytes = expertScalarString(target["xPaddingBytes"])\n\t\tout.XHTTPNoSSEHeader = expertBool(target["noSSEHeader"])\n\t\tout.XHTTPScMaxBufferedPosts = expertInt64(target["scMaxBufferedPosts"])\n\t\tout.XHTTPScMaxEachPostBytes = expertScalarString(target["scMaxEachPostBytes"])\n\t\tout.XHTTPScStreamUpServerSecs = expertScalarString(target["scStreamUpServerSecs"])\n\t\tout.XHTTPUplinkHTTPMethod = expertString(target["uplinkHTTPMethod"])\n\t}\n\treturn out, nil'
text2, count = re.subn(pattern, replacement, text, count=1, flags=re.S)
if count != 1:
    raise SystemExit(f"expert view xhttp block matches={count}")
p.write_text(text2)

p = Path("web/dist/index.html")
text = p.read_text()
if 'v6.css?v=0.1.5' not in text:
    text = text.replace('<link rel="stylesheet" href="/v5.css?v=0.1.4">', '<link rel="stylesheet" href="/v5.css?v=0.1.4"><link rel="stylesheet" href="/v6.css?v=0.1.5">')
text = text.replace('<label>面板监听<input name="panelListen" placeholder="127.0.0.1:8080"></label>', '<label>面板端口<input name="panelListen" inputmode="numeric" pattern="[0-9]*" placeholder="例如 50010"></label>')
text = text.replace('<label class="full">当前密码（修改管理员时需要）<input name="currentPassword" type="password"></label>', '')
text = text.replace('面板监听、Base Path 或 HTTPS 变更后需要重启 X-port；防火墙仍由你手动放行。', '面板端口、Base Path 或 HTTPS 变更后需要重启 X-port；防火墙仍由你手动放行。')
if 'v6.js?v=0.1.5' not in text:
    text = text.replace('<script src="/v5-runtime.js?v=0.1.4" defer></script>', '<script src="/v5-runtime.js?v=0.1.4" defer></script><script src="/v6.js?v=0.1.5" defer></script>')
p.write_text(text)

Path("internal/buildinfo/version.go").write_text('package buildinfo\n\nconst Current = "0.1.5"\n')
