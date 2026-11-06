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
  _,_,broad,valid:=ufwPortLine(tc.line)
  if valid!=tc.valid||broad!=tc.broad{t.Errorf("line %q: valid=%v broad=%v",tc.line,valid,broad)}
 }
}
