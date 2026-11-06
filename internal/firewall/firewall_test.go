package firewall

import (
 "context"
 "encoding/json"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestPortAndZoneValidation(t *testing.T) {
 cases:=[]struct{value string;valid bool}{
  {"1/tcp",true},{"65535/udp",true},{"0/tcp",false},{"65536/tcp",false},
  {"22/icmp",false},{"22/tcp;rm",false},{"-1/tcp",false},
 }
 for _,tc:=range cases{p,proto,ok:=ParsePort(tc.value);if ok!=tc.valid{t.Errorf("%s: got %v",tc.value,ok)};if ok&&!ValidRule(p,proto){t.Errorf("%s invalid after parse",tc.value)}}
 for _,tc:=range []struct{value string;valid bool}{{"public",true},{"zone-test_01",true},{"bad zone",false},{"--evil",false},{"a/b",false},{"",false}}{
  if ValidZone(tc.value)!=tc.valid{t.Errorf("%q validity",tc.value)}
 }
}
func TestActiveZones(t *testing.T){
 got:=strings.Join(activeZones("public\n  interfaces: eth0\ntrusted\n  sources: 192.0.2.0/24\n"),",")
 if got!="public,trusted"{t.Fatalf("zones=%q",got)}
}
func TestFirewalldSnapshotAndGuards(t *testing.T){
 dir:=t.TempDir()
 script:="#!/bin/sh\ncase \"$1\" in\n  --state) echo running;;\n  --get-default-zone) echo public;;\n  --get-active-zones) printf 'public\\n  interfaces: eth0\\n';;\n  --permanent) echo '22/tcp 2053/tcp';;\n  --zone=public) case \"$2\" in --list-ports) echo '22/tcp 2053/tcp';; --list-services) echo ssh;; *) exit 0;; esac;;\n  *) exit 1;;\nesac\n"
 if err:=os.WriteFile(filepath.Join(dir,"firewall-cmd"),[]byte(script),0755);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",dir)
 snap,err:=Inspect(context.Background());if err!=nil{t.Fatal(err)}
 if snap.Backend!="firewalld"||snap.DefaultZone!="public"||len(snap.Rules)!=2||len(snap.Services["public"])!=1{t.Fatalf("snapshot=%+v",snap)}
 if err:=Change(context.Background(),"remove",2053,"tcp","unknown");err==nil{t.Fatal("unknown zone should fail")}
 if err:=Change(context.Background(),"remove",22,"icmp","public");err==nil{t.Fatal("invalid protocol should fail")}
 if err:=Change(context.Background(),"add",2053,"tcp","public");err==nil{t.Fatal("duplicate should fail")}
}
func TestReadOnlyNftables(t *testing.T){
 dir:=t.TempDir()
 nft:="#!/bin/sh\nif [ \"$1\" = '-a' ]; then echo 'table inet example {}'; else exit 2; fi\n"
 if err:=os.WriteFile(filepath.Join(dir,"nft"),[]byte(nft),0755);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",dir)
 snap,err:=Inspect(context.Background());if err!=nil{t.Fatal(err)}
 if snap.Backend!="nftables"||snap.Active{t.Fatalf("snapshot=%+v",snap)}
 if err:=Change(context.Background(),"add",2053,"tcp","");err==nil{t.Fatal("nft mutating should be rejected")}
 raw,err:=Raw(context.Background());if err!=nil||!strings.Contains(raw,"table inet example"){t.Fatalf("raw=%q err=%v",raw,err)}
}

func TestUFWPortLine(t *testing.T){
 for _,tc:=range []struct{line string;valid,broad bool}{
  {"22/tcp ALLOW Anywhere",true,true},
  {"22/tcp (v6) ALLOW Anywhere (v6)",true,true},
  {"8443/udp ALLOW IN Anywhere",true,true},
  {"443/tcp ALLOW 192.0.2.1",true,false},
  {"443/tcp DENY Anywhere",true,false},
  {"OpenSSH ALLOW Anywhere",false,false},
 }{
  _,_,broad,_,valid:=ufwPortLine(tc.line)
  if valid!=tc.valid||broad!=tc.broad{t.Errorf("line %q: valid=%v broad=%v",tc.line,valid,broad)}
 }
}

func TestIndependentInputChains(t *testing.T){
 for _,tt:=range []struct{rules string;conflict bool}{
  {`{"nftables":[{"table":{"family":"inet","name":"firewalld"}},{"chain":{"family":"inet","table":"firewalld","name":"filter_INPUT","type":"filter","hook":"input"}}]}`,false},
  {`{"nftables":[{"chain":{"family":"inet","table":"custom","name":"input","type":"filter","hook":"input"}}]}`,true},
  {`{"nftables":[{"chain":{"family":"ip","table":"docker","name":"forward","type":"filter","hook":"forward"}}]}`,false},
 }{
  conflict,err:=independentInputChains(tt.rules)
  if err!=nil||conflict!=tt.conflict{t.Errorf("rules=%s, conflict=%v err=%v",tt.rules,conflict,err)}
 }
}
// A common host uses ip/ip6 NAT PREROUTING alongside firewalld's
// filter_INPUT chain. A NAT hook alone must not disable port management.
func TestNativePreroutingChainClassification(t *testing.T) {
 cases:=[]struct{name,rule string;blocked bool}{
  {"ipv4 NAT",`{"nftables":[{"chain":{"family":"ip","table":"nat","name":"PREROUTING","type":"nat","hook":"prerouting","prio":-100}}]}`,false},
  {"ipv6 NAT",`{"nftables":[{"chain":{"family":"ip6","table":"nat","name":"PREROUTING","type":"nat","hook":"prerouting","prio":-100}}]}`,false},
  {"external filter prerouting",`{"nftables":[{"chain":{"family":"inet","table":"custom","name":"early","type":"filter","hook":"prerouting","prio":-150}}]}`,true},
  {"external mangle prerouting",`{"nftables":[{"chain":{"family":"ip","table":"mangle","name":"PRE","type":"filter","hook":"prerouting"}}]}`,true},
  {"unknown prerouting type",`{"nftables":[{"chain":{"family":"ip","table":"custom","name":"PRE","hook":"prerouting"}}]}`,true},
  {"external input filter",`{"nftables":[{"chain":{"family":"inet","table":"custom","name":"input","type":"filter","hook":"input"}}]}`,true},
  {"external ingress",`{"nftables":[{"chain":{"family":"netdev","table":"custom","name":"ingress","type":"filter","hook":"ingress"}}]}`,true},
  {"foreign ipv4 firewalld-named table",`{"nftables":[{"chain":{"family":"ip","table":"firewalld","name":"filter_INPUT","type":"filter","hook":"input"}}]}`,true},
  {"foreign ipv6 firewalld-named table",`{"nftables":[{"chain":{"family":"ip6","table":"firewalld","name":"filter_INPUT","type":"filter","hook":"input"}}]}`,true},
  {"external nat INPUT",`{"nftables":[{"chain":{"family":"ip","table":"nat","name":"INPUT","type":"nat","hook":"input"}}]}`,true},
  {"external filter with same prerouting hook",`{"nftables":[{"chain":{"family":"ip","table":"nat","name":"PRE","type":"filter","hook":"prerouting"}}]}`,true},
  {"firewalld internals only",`{"nftables":[{"chain":{"family":"inet","table":"firewalld","name":"filter_INPUT","type":"filter","hook":"input"}}]}`,false},

 }
 for _,tc:=range cases{t.Run(tc.name,func(t *testing.T){
  blocker,err:=independentInputChainBlocker(tc.rule,false)
  if err!=nil{t.Fatal(err)}
  if (blocker!="")!=tc.blocked{t.Fatalf("blocker=%q expected blocked=%v",blocker,tc.blocked)}
 })}
 // A NAT chain by itself does not authorize writes when an unrelated input
 // filter also participates. This must remain fail-closed.
 mixed:=`{"nftables":[
 {"chain":{"family":"ip","table":"nat","name":"PREROUTING","type":"nat","hook":"prerouting"}},
 {"chain":{"family":"ip6","table":"nat","name":"PREROUTING","type":"nat","hook":"prerouting"}},
 {"chain":{"family":"inet","table":"other","name":"INPUT","type":"filter","hook":"input"}}
 ]}`
 blocker,err:=independentInputChainBlocker(mixed,false)
 if err!=nil||!strings.Contains(blocker,"inet/other/INPUT"){
  t.Fatalf("independent INPUT hidden by NAT coexistence: blocker=%q err=%v",blocker,err)
 }
 // Fixture matches observed firewalld + IPv4/IPv6 NAT base-chain layout.
 combined:=`{"nftables":[
 {"chain":{"family":"ip","table":"nat","name":"PREROUTING","type":"nat","hook":"prerouting","prio":-100}},
 {"chain":{"family":"ip6","table":"nat","name":"PREROUTING","type":"nat","hook":"prerouting","prio":-100}},
 {"chain":{"family":"inet","table":"firewalld","name":"mangle_PREROUTING","type":"filter","hook":"prerouting","prio":-140}},
 {"chain":{"family":"inet","table":"firewalld","name":"nat_PREROUTING","type":"nat","hook":"prerouting","prio":-90}},
 {"chain":{"family":"inet","table":"firewalld","name":"filter_PREROUTING","type":"filter","hook":"prerouting","prio":10}},
 {"chain":{"family":"inet","table":"firewalld","name":"filter_INPUT","type":"filter","hook":"input","prio":10}}
 ]}`
 blocked,err:=independentInputChainsWithUFW(combined,false)
 if err!=nil||blocked{t.Fatalf("valid NAT coexistence blocked: %v %v",blocked,err)}
}

func TestFirewalldNATCoexistenceEnablesWrites(t *testing.T){
 dir:=t.TempDir()
 cmd:=`#!/bin/sh
case "$1" in
 --state) echo running;;
 --get-default-zone) echo public;;
 --get-active-zones) echo public;;
 --permanent) echo '443/tcp';;
 --zone=public) echo '443/tcp';;
 *) exit 1;;
esac
`
 nft:=`#!/bin/sh
echo '{"nftables":[
{"chain":{"family":"ip","table":"nat","name":"PREROUTING","type":"nat","hook":"prerouting"}},
{"chain":{"family":"ip6","table":"nat","name":"PREROUTING","type":"nat","hook":"prerouting"}},
{"chain":{"family":"inet","table":"firewalld","name":"filter_INPUT","type":"filter","hook":"input"}}]}'
`
 for n,s:=range map[string]string{"firewall-cmd":cmd,"nft":nft}{
  if err:=os.WriteFile(filepath.Join(dir,n),[]byte(s),0755);err!=nil{t.Fatal(err)}
 }
 t.Setenv("PATH",dir)
 snap,err:=Inspect(context.Background())
 if err!=nil||!snap.Writable||snap.Backend!="firewalld"{t.Fatalf("NAT coexistence: %+v %v",snap,err)}
}

func TestMixedManagersDisableWrites(t *testing.T){
 dir:=t.TempDir()
 a:=`#!/bin/sh
if [ "$1" = "--state" ]; then echo running; exit 0; fi
exit 1
`
 b:=`#!/bin/sh
echo "Status: active"
`
 for name,code:=range map[string]string{"firewall-cmd":a,"ufw":b}{
  if err:=os.WriteFile(filepath.Join(dir,name),[]byte(code),0755);err!=nil{t.Fatal(err)}
 }
 t.Setenv("PATH",dir)
 snap,err:=Inspect(context.Background())
 if err!=nil||snap.Backend!="mixed"||snap.Writable||len(snap.Managers)!=2{t.Fatalf("snapshot=%+v err=%v",snap,err)}
 if err:=Change(context.Background(),"add",23333,"tcp","public");err==nil{t.Fatal("mixed manager should not write")}
}
func TestUFWIPv4IPv6Status(t *testing.T){
 p,proto,broad,v6,ok:=ufwPortLine("2053/tcp (v6) ALLOW Anywhere (v6)")
 if !ok||!broad||!v6||p!=2053||proto!="tcp"{t.Fatalf("IPv6 parsing: %d %s %v %v %v",p,proto,broad,v6,ok)}
}

func TestListeningPortsAndUnknownFailure(t *testing.T){
 dir:=t.TempDir()
 script:=`#!/bin/sh
printf 'tcp LISTEN 0 128 127.0.0.1:8080 0.0.0.0:*\n'
printf 'udp UNCONN 0 0 [::]:2053 [::]:*\n'
`
 if err:=os.WriteFile(filepath.Join(dir,"ss"),[]byte(script),0755);err!=nil{t.Fatal(err)}
 t.Setenv("PATH",dir)
 ports,err:=ListeningPorts(context.Background());if err!=nil{t.Fatal(err)}
 if !ports[8080]||!ports[2053]{t.Fatalf("ports=%v",ports)}
 sockets,err:=ListeningSockets(context.Background());if err!=nil{t.Fatal(err)}
 if !sockets["tcp/8080"]||!sockets["udp/2053"]{t.Fatalf("sockets=%v",sockets)}
 if sockets["udp/8080"]||sockets["tcp/2053"]{t.Fatalf("transport confusion: %v",sockets)}
}

func TestListeningSocketsMustReadLocalEndpointNotPeer(t *testing.T){
 cases:=[]struct{name,raw string;key string;wantErr bool}{
  {"IPv4 TCP","tcp LISTEN 0 128 127.0.0.1:8443 127.0.0.1:2053","tcp/8443",false},
  {"IPv6 TCP","tcp LISTEN 0 128 [::]:443 [::]:*","tcp/443",false},
  {"IPv4 UDP","udp UNCONN 0 0 0.0.0.0:443 0.0.0.0:*","udp/443",false},
  {"IPv6 UDP","udp UNCONN 0 0 [::]:2053 [::]:*","udp/2053",false},
  {"no local socket","tcp LISTEN 0 128 BAD 127.0.0.1:80","",true},
  {"malformed local port","udp UNCONN 0 0 127.0.0.1:NaN 127.0.0.1:5000","",true},
  {"unexpected protocol","raw UNCONN 0 0 127.0.0.1:80 127.0.0.1:*","",true},
  {"truncated row","tcp LISTEN 0 128","",true},
  {"no sockets","","",false},
 }
 for _,tt:=range cases{t.Run(tt.name,func(t *testing.T){
  result,err:=parseListeningSockets(tt.raw)
  if (err!=nil)!=tt.wantErr{t.Fatalf("error=%v wantErr=%t",err,tt.wantErr)}
  if err==nil&&tt.key!=""&&!result[tt.key]{t.Fatalf("missing %s: %v",tt.key,result)}
 })}
 mixed,err:=parseListeningSockets("tcp LISTEN 0 128 *:7443 *:*\nudp UNCONN 0 0 [::]:7443 [::]:*")
 if err!=nil{t.Fatal(err)}
 if !mixed["tcp/7443"]||!mixed["udp/7443"]||len(mixed)!=2{t.Fatalf("shared port must preserve both transports: %v",mixed)}
}
func TestNativeInputChainBlocksWrites(t *testing.T){
 dir:=t.TempDir()
 firewalld:=`#!/bin/sh
case "$1" in
 --state) echo running;;
 --get-default-zone) echo public;;
 --get-active-zones) echo public;;
 --permanent) echo '443/tcp';;
 --zone=public) echo '443/tcp';;
esac
`
 nft:=`#!/bin/sh
echo '{"nftables":[{"chain":{"family":"inet","table":"independent","hook":"input","type":"filter"}}]}'
`
 for name,script:=range map[string]string{"firewall-cmd":firewalld,"nft":nft}{
  if err:=os.WriteFile(filepath.Join(dir,name),[]byte(script),0755);err!=nil{t.Fatal(err)}
 }
 t.Setenv("PATH",dir)
 snap,err:=Inspect(context.Background())
 if err!=nil{t.Fatal(err)}
 if snap.Writable||snap.UnsafeReason==""{t.Fatalf("independent input hook should block writes: %+v",snap)}
}

func TestUFWTopLevelOwnershipDetection(t *testing.T){
 baseline:=`{"nftables":[{"chain":{"family":"ip","table":"filter","name":"INPUT","hook":"input"}},{"rule":{"family":"ip","table":"filter","chain":"INPUT","expr":[{"counter":{"packets":0,"bytes":0}},{"jump":{"target":"ufw-before-input"}}]}},{"rule":{"family":"ip","table":"filter","chain":"INPUT","expr":[{"counter":{"packets":0,"bytes":0}},{"jump":{"target":"ufw-after-input"}}]}},{"rule":{"family":"ip","table":"filter","chain":"INPUT","expr":[{"counter":{"packets":0,"bytes":0}},{"jump":{"target":"ufw-reject-input"}}]}},{"rule":{"family":"ip","table":"filter","chain":"INPUT","expr":[{"counter":{"packets":0,"bytes":0}},{"jump":{"target":"ufw-track-input"}}]}}]}`
 conflict,err:=independentInputChainsWithUFW(baseline,true)
 if err!=nil||conflict{t.Fatalf("UFW managed input incorrectly flagged: %v %v",conflict,err)}
 ip6:=strings.ReplaceAll(strings.ReplaceAll(baseline,`"family":"ip"`,`"family":"ip6"`),"ufw-","ufw6-")
 conflict,err=independentInputChainsWithUFW(ip6,true)
 if err!=nil||conflict{t.Fatalf("UFW IPv6 managed input incorrectly flagged: %v %v",conflict,err)}
 conflict,err=independentInputChainsWithUFW(baseline,false)
 if err!=nil||!conflict{t.Fatalf("same rules without UFW must be flagged: %v %v",conflict,err)}
 var parsed struct{Nftables []map[string]json.RawMessage `json:"nftables"`}
 if err:=json.Unmarshal([]byte(baseline),&parsed);err!=nil{t.Fatal(err)}
 // A direct DROP inserted into UFW's top-level INPUT chain must not
 // be treated as a manager-owned UFW jump.
 foreign:=map[string]any{"rule":map[string]any{"family":"ip","table":"filter","chain":"INPUT","expr":[]any{map[string]any{"drop":nil}}}}
 raw,err:=json.Marshal(foreign);if err!=nil{t.Fatal(err)}
 var entry map[string]json.RawMessage
 if err=json.Unmarshal(raw,&entry);err!=nil{t.Fatal(err)}
 parsed.Nftables=append(parsed.Nftables,entry)
 modified,err:=json.Marshal(parsed);if err!=nil{t.Fatal(err)}
 conflict,err=independentInputChainsWithUFW(string(modified),true)
 if err!=nil||!conflict{t.Fatalf("foreign DROP must disable writing: %v %v",conflict,err)}
}
