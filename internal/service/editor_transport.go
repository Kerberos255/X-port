package service

import (
	"encoding/json"
	"strings"

	"github.com/Kerberos255/X-port/internal/accountcfg"
	"github.com/Kerberos255/X-port/internal/model"
)

// normalizeEditorInput keeps the HTTP API/editor on one canonical spelling for
// transports while still accepting Xray's common legacy/current aliases.
func normalizeEditorInput(in *accountcfg.Input) {
	in.Protocol = strings.ToLower(strings.TrimSpace(in.Protocol))
	in.Network = editorNetworkValue(in.Network)
	in.Security = strings.ToLower(strings.TrimSpace(in.Security))
	if in.Protocol == "socks" || in.Protocol == "http" {
		in.Network = ""
		in.Security = "none"
		in.Flow = ""
	}
	if in.Protocol != "vless" {
		in.Flow = ""
	}
}

func editorNetworkValue(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "tcp", "raw":
		return "tcp"
	case "ws", "websocket":
		return "ws"
	case "xhttp", "splithttp":
		return "xhttp"
	case "grpc":
		return "grpc"
	case "httpupgrade":
		return "httpupgrade"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}

func editorMethodValue(value string) string {
	switch editorNetworkValue(value) {
	case "tcp":
		return "raw"
	case "ws":
		return "websocket"
	default:
		return editorNetworkValue(value)
	}
}

// editorView adds the stream-method compatibility checks that the basic
// accountcfg view intentionally does not know about. This prevents imported
// transports which the built-in form cannot faithfully represent from being
// silently rewritten as RAW when opened in the editor.
func editorView(a model.Account) (accountcfg.View, error) {
	v, err := accountcfg.ToView(a)
	if err != nil {
		return v, err
	}
	protocol := strings.ToLower(strings.TrimSpace(a.Protocol))
	if protocol == "socks" || protocol == "http" {
		v.Network = ""
		v.Security = "none"
		return v, nil
	}

	stream := map[string]any{}
	if err := json.Unmarshal([]byte(defaultAdvancedJSON(a.StreamSettingsJSON)), &stream); err != nil {
		v.Editable = false
		return v, nil
	}
	method := expertMethod(stream, protocol)
	switch method {
	case "raw":
		v.Network = "tcp"
	case "websocket":
		v.Network = "ws"
	case "xhttp", "grpc", "httpupgrade":
		v.Network = method
	default:
		v.Editable = false
		return v, nil
	}
	security := strings.ToLower(strings.TrimSpace(expertString(stream["security"])))
	if security == "" {
		security = "none"
	}
	v.Security = security
	if !editorSecuritySupported(protocol, v.Network, security) {
		v.Editable = false
		return v, nil
	}
	if protocol == "vless" && v.Flow != "" && !editorVisionCompatible(v.Network, security) {
		v.Editable = false
		return v, nil
	}

	// accountcfg.ToView understands legacy network spellings. Re-read transport
	// fields for current method spellings as well so modern imported configs are
	// shown without losing Host/Path/serviceName.
	switch method {
	case "websocket":
		x := expertObject(stream, "wsSettings")
		v.Path = expertString(x["path"])
		headers := expertObject(x, "headers")
		v.Host = expertFirstString(x["host"], headers["Host"], headers["host"])
	case "grpc":
		x := expertObject(stream, "grpcSettings")
		v.ServiceName = expertString(x["serviceName"])
	case "httpupgrade":
		x := expertObject(stream, "httpupgradeSettings")
		v.Path, v.Host = expertString(x["path"]), expertString(x["host"])
	case "xhttp":
		x := expertObject(stream, expertXHTTPKey(stream))
		v.Path, v.Host = expertString(x["path"]), expertString(x["host"])
	}
	return v, nil
}

func editorPublicView(a model.Account) accountcfg.View {
	v, _ := editorView(a)
	v.Credential, v.Username, v.Password, v.ServerPassword, v.PrivateKey = "", "", "", "", ""
	return v
}

func editorSecuritySupported(protocol, network, security string) bool {
	if security != "none" && security != "tls" && security != "reality" {
		return false
	}
	if security == "reality" {
		if protocol != "vless" && protocol != "trojan" {
			return false
		}
		return network == "tcp" || network == "xhttp" || network == "grpc"
	}
	return true
}

func editorVisionCompatible(network, security string) bool {
	return network == "tcp" && (security == "tls" || security == "reality")
}

// sanitizeEditableBaseForExplicitClears makes empty values from visible editor
// fields mean "clear this field" instead of letting accountcfg.Update merge the
// old value back in. Unknown/inactive raw JSON remains preserved.
func sanitizeEditableBaseForExplicitClears(a model.Account, in accountcfg.Input) model.Account {
	settings, stream, err := expertMaps(a)
	if err != nil {
		return a
	}
	protocol := strings.ToLower(strings.TrimSpace(a.Protocol))
	if protocol == "vless" && strings.TrimSpace(in.Flow) == "" {
		if clients, ok := settings["clients"].([]any); ok && len(clients) == 1 {
			if client, ok := clients[0].(map[string]any); ok {
				delete(client, "flow")
			}
		}
	}

	security := strings.ToLower(strings.TrimSpace(in.Security))
	if security != "" {
		if security != "tls" {
			delete(stream, "tlsSettings")
		}
		if security != "reality" {
			delete(stream, "realitySettings")
		}
	}

	switch editorMethodValue(in.Network) {
	case "websocket":
		x := expertObject(stream, "wsSettings")
		if strings.TrimSpace(in.Path) == "" {
			delete(x, "path")
		}
		if strings.TrimSpace(in.Host) == "" {
			delete(x, "host")
			if headers, ok := x["headers"].(map[string]any); ok {
				delete(headers, "Host")
				delete(headers, "host")
				if len(headers) == 0 {
					delete(x, "headers")
				}
			}
		}
		if len(x) == 0 {
			delete(stream, "wsSettings")
		} else {
			stream["wsSettings"] = x
		}
	case "grpc":
		x := expertObject(stream, "grpcSettings")
		if strings.TrimSpace(in.ServiceName) == "" {
			delete(x, "serviceName")
		}
		if len(x) == 0 {
			delete(stream, "grpcSettings")
		} else {
			stream["grpcSettings"] = x
		}
	case "httpupgrade":
		x := expertObject(stream, "httpupgradeSettings")
		if strings.TrimSpace(in.Path) == "" {
			delete(x, "path")
		}
		if strings.TrimSpace(in.Host) == "" {
			delete(x, "host")
		}
		if len(x) == 0 {
			delete(stream, "httpupgradeSettings")
		} else {
			stream["httpupgradeSettings"] = x
		}
	case "xhttp":
		key := expertXHTTPKey(stream)
		x := expertObject(stream, key)
		if strings.TrimSpace(in.Path) == "" {
			delete(x, "path")
		}
		if strings.TrimSpace(in.Host) == "" {
			delete(x, "host")
		}
		if len(x) == 0 {
			delete(stream, key)
		} else {
			stream[key] = x
		}
	}

	if b, err := json.Marshal(settings); err == nil {
		a.SettingsJSON = string(b)
	}
	if b, err := json.Marshal(stream); err == nil {
		a.StreamSettingsJSON = string(b)
	}
	return a
}
