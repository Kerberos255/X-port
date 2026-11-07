#!/usr/bin/env bash
set -Eeuo pipefail
# This job runs on a disposable GitHub-hosted runner. Never use production Docker.
BIN="${1:?candidate xport executable required}"
test -x "$BIN"
test -S /var/run/docker.sock
command -v docker >/dev/null
DIR="$(mktemp -d -t xport-docker-discovery.XXXXXXXX)"
NAME="xport-docker-usage-ci-$$"
IMAGE="xport-docker-usage-ci:$$"
cleanup(){
 docker rm -f "$NAME" >/dev/null 2>&1 || true
 docker rmi "$IMAGE" >/dev/null 2>&1 || true
 rm -rf "$DIR"
}
trap cleanup EXIT
cat > "$DIR/main.go" <<'EOF'
package main
import ("net";"log")
func main() {
 tcp,err:=net.Listen("tcp",":8080")
 if err!=nil{log.Fatal(err)}
 defer tcp.Close()
 udp,err:=net.ListenPacket("udp",":8080")
 if err!=nil{log.Fatal(err)}
 defer udp.Close()
 go func(){for {conn,err:=tcp.Accept();if err!=nil{return};_ = conn.Close()}}()
 payload:=make([]byte,64)
 for {if _,_,err:=udp.ReadFrom(payload);err!=nil{log.Fatal(err)}}
}
EOF
CGO_ENABLED=0 GOOS=linux go build -trimpath -o "$DIR/probe" "$DIR/main.go"
printf 'FROM scratch\nCOPY probe /probe\nENTRYPOINT ["/probe"]\n' >"$DIR/Dockerfile"
docker build -q -t "$IMAGE" "$DIR" >/dev/null
docker run -d --name "$NAME" --network bridge \
 -p 127.0.0.1:53578:8080/tcp -p 127.0.0.1:53579:8080/udp "$IMAGE" >/dev/null
running=0
for i in $(seq 1 25); do
 if docker inspect -f '{{.State.Running}}' "$NAME" | grep -Fxq true; then running=1; break; fi
 sleep .2
done
if [[ "$running" != 1 ]]; then
 docker logs "$NAME" >&2 || true
 echo "Docker probe did not remain running" >&2
 exit 1
fi
sudo "$BIN" firewall-discovery >"$DIR/discovery.json"
python3 - "$DIR/discovery.json" "$NAME" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
assert d["dockerUncertain"] is False, d
mapped={(p["port"],p["protocol"],p["container"]) for p in (d.get("docker") or [])}
want={(53578,"tcp",sys.argv[2]),(53579,"udp",sys.argv[2])}
assert want.issubset(mapped), (want,mapped)
print("XPORT_DOCKER_PORT_DISCOVERY_OK")
PY
