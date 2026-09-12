import json
import os
from http.server import ThreadingHTTPServer, SimpleHTTPRequestHandler
from pathlib import Path

ROOT = Path(os.environ["GITHUB_WORKSPACE"]) / "web" / "dist"
ACCOUNT = {
    "id": 1, "name": "vless+XHTTP_test", "enabled": True, "disabledReason": "",
    "editable": True, "port": 40836, "protocol": "vless", "security": "none",
    "network": "xhttp", "upBytes": 1, "downBytes": 2, "allTimeBytes": 3,
    "quotaBytes": 0, "expiryTime": 0, "monthlyReset": False,
    "credential": "d84ff80b-e4d2-45c5-9778-f298e39f040d", "flow": "xtls-rprx-vision",
    "host": "", "path": "/vlessxhttptest", "tag": "xport-40836-test"
}
EXPERT = {
    "protocol": "vless", "method": "xhttp", "security": "none",
    "acceptProxyProtocol": False, "supportsFallbacks": False, "supportsHttpHeader": False,
    "supportsReality": False, "supportsTls": False, "supportsXhttp": True,
    "xhttpMode": "auto", "xhttpXPaddingBytes": "100-1000", "xhttpNoSseHeader": False,
    "xhttpScMaxBufferedPosts": 30, "xhttpScMaxEachPostBytes": "1000000",
    "xhttpScStreamUpServerSecs": "20-80", "xhttpUplinkHttpMethod": "POST"
}

class Handler(SimpleHTTPRequestHandler):
    def translate_path(self, path):
        clean = path.split("?", 1)[0]
        return str(ROOT / ("index.html" if clean == "/" else clean.lstrip("/")))

    def json(self, obj, status=200):
        body = json.dumps(obj).encode()
        self.send_response(status)
        self.send_header("Content-Type", "application/json")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        self.wfile.write(body)

    def do_GET(self):
        path = self.path.split("?", 1)[0]
        if path == "/api/overview":
            return self.json({"system":{"hostname":"QiLin","os":"Linux","uptimeSeconds":8730360,"cpuPercent":4.6,"load1":0.12,"memoryUsed":620000000,"memoryTotal":1070000000,"diskUsed":10700000000,"diskTotal":20000000000,"networkRx":1,"networkTx":1,"xrayActive":True,"xrayVersion":"26.3.27"},"accounts":{"total":1,"enabled":1},"xportVersion":"0.1.5"})
        if path == "/api/accounts": return self.json({"accounts":[ACCOUNT]})
        if path == "/api/accounts/1": return self.json(ACCOUNT)
        if path == "/api/accounts/online": return self.json({"peers":{"1":0},"connections":{"1":0}})
        if path == "/api/accounts/1/expert": return self.json(EXPERT)
        if path == "/api/accounts/port-suggestion": return self.json({"port":42317,"min":20000,"max":60000})
        if path == "/api/settings":
            return self.json({"panelListen":":50010","panelBasePath":"/YTjAodrPHeQ5TLI/","panelDomain":"","panelCertFile":"/root/cert/example/fullchain.pem","panelKeyFile":"/root/cert/example/privkey.pem","xrayApiPort":10085,"portMin":20000,"portMax":60000,"defaultRealitySni":"","defaultRealityDest":"","adminUsername":"admin-test"})
        if path == "/api/xray/update": return self.json({"current":"26.3.27","latest":"26.3.27","available":False})
        if path == "/api/xray/rollback": return self.json({"available":False})
        if path == "/api/health": return self.json({"ok":True,"database":True})
        if path == "/api/backups": return self.json({"backups":[]})
        if path == "/api/logs": return self.json({"logs":"mock"})
        return super().do_GET()

    def do_POST(self): return self.json({"id":2,"name":"new","port":42317}, 201)
    def do_PUT(self): return self.json(EXPERT)
    def do_PATCH(self): return self.json({"ok":True})

ThreadingHTTPServer(("127.0.0.1", 8878), Handler).serve_forever()
