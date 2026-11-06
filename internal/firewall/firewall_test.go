package firewall

import (
 "context"
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
