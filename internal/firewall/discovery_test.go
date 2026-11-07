package firewall

import (
 "context"
 "math/rand"
 "os"
 "path/filepath"
 "strings"
 "testing"
)

func TestProcessSocketAttribution(t *testing.T){
 lines:=[]string{
  `tcp LISTEN 0 128 0.0.0.0:22022 0.0.0.0:* users:(("sshd",pid=12345,fd=3))`,
  `tcp LISTEN 0 128 [::]:2053 [::]:* users:(("xray",pid=5010,fd=7),("xray",pid=5010,fd=8))`,
  `udp UNCONN 0 0 [::]:2053 [::]:* users:(("hbbc",pid=23456,fd=4))`,
  `tcp LISTEN 0 128 *:39000 *:*`,
 }
 got,err:=parseProcessSockets(strings.Join(lines,string(byte(10))))
 if err!=nil{t.Fatal(err)}
 if len(got)!=4{t.Fatalf("expected 4 unique PID/protocol/port entries: %+v",got)}
 for _,want:=range []ListeningProcess{
  {Port:22022,Protocol:"tcp",PID:12345,Name:"sshd"},
  {Port:2053,Protocol:"tcp",PID:5010,Name:"xray"},
  {Port:2053,Protocol:"udp",PID:23456,Name:"hbbc"},
  {Port:39000,Protocol:"tcp"},
 }{
  found:=false
  for _,item:=range got{if item==want{found=true;break}}
  if !found{t.Errorf("missing %+v from %+v",want,got)}
 }
 for _,bad:=range []string{
  `tcp LISTEN 0 128 INVALID 0.0.0.0:22`,
  `tcp LISTEN 0 128 127.0.0.1:NaN 0.0.0.0:22`,
  `raw LISTEN 0 128 127.0.0.1:22 0.0.0.0:*`,
  `tcp LISTEN 0`,
 }{
  if _,e:=parseProcessSockets(bad);e==nil{t.Errorf("bad socket accepted: %s",bad)}
 }
}
func TestConfiguredSSHProtection(t *testing.T){
 dir:=t.TempDir()
 script:=func(name string,lines []string){
  code:=strings.Join(append([]string{"#!/bin/sh"},lines...),string(byte(10)))
  if err:=os.WriteFile(filepath.Join(dir,name),[]byte(code),0755);err!=nil{t.Fatal(err)}
 }
 script("sshd",[]string{ `echo 'port 22022'`,`echo 'port 42222'` })
 script("systemctl",[]string{ `echo '53500 (Stream)'` })
 t.Setenv("PATH",dir)
 found,unknown:=SSHProtection(context.Background(),[]ListeningProcess{
  {Port:22222,Protocol:"tcp",PID:123,Name:"sshd"},
  {Port:22333,Protocol:"tcp",PID:234,Name:"xray"},
 })
 if unknown{t.Fatal("valid sshd -T must be known")}
 expected:=[]int{22,22022,22222,42222,53500}
 if len(found)!=len(expected){t.Fatalf("SSH ports=%v",found)}
 for i:=range expected{if found[i]!=expected[i]{t.Fatalf("SSH ports=%v",found)}}
 script("sshd",[]string{`exit 1`})
 _,unknown=SSHProtection(context.Background(),nil)
 if !unknown{t.Fatal("unreadable sshd config must be fail-closed")}
}
func TestFTPRangeValidation(t *testing.T){
 for _,value:=range []string{"39000 40000","39000-40000","39000,40000"}{
  got,ok:=parseFTPRange(value,"tcp","FTP")
  if !ok||got.Start!=39000||got.End!=40000{t.Fatalf("%q: %+v %v",value,got,ok)}
 }
 for _,value:=range []string{"0 40000","40000 39000","39000 70000","39000 39000 bad","25;rm -rf / 30"}{
  if _,ok:=parseFTPRange(value,"tcp","FTP");ok{t.Fatalf("invalid accepted %q",value)}
 }
}
func TestDiscoveryGuards(t *testing.T){
 d:=Discovery{SSHPorts:[]int{22,22022},Processes:[]ListeningProcess{{Port:53343,Protocol:"tcp",Name:"xray"}},Docker:[]PublishedPort{{Port:443,Protocol:"tcp",Container:"web"}}}
 for _,p:=range []int{22,22022,53343,443}{if !d.ProtectedPort(p){t.Errorf("%d unprotected",p)}}
 if d.ProtectedPort(55555){t.Fatal("unrelated port blocked")}
 d.SSHUncertain=true
 if !d.ProtectedPort(55555){t.Fatal("uncertain ssh must block")}
 d.SSHUncertain=false;d.DockerUncertain=true
 if !d.ProtectedPort(55555){t.Fatal("uncertain docker must block")}
}
func TestPortRangesRemainReadOnly(t *testing.T){
 for _,tc:=range []struct{raw string;start,end int;valid bool}{
  {"39000-40000/tcp",39000,40000,true},
  {"1-65535/udp",1,65535,true},
  {"39000-39000/tcp",0,0,false},
  {"39000-70000/tcp",0,0,false},
  {"39000-40000/icmp",0,0,false},
  {"39000-40000/tcp;foo",0,0,false},
 }{
  r,ok:=ParsePortSpec(tc.raw)
  if ok!=tc.valid{t.Fatalf("%s valid=%v",tc.raw,ok)}
  if ok&&(r.Port!=0||r.RangeStart!=tc.start||r.RangeEnd!=tc.end||!r.IsRange()||!r.ContainsPort(tc.start)||r.ContainsPort(tc.end+1)){
   t.Fatalf("%s incorrectly represented: %+v",tc.raw,r)
  }
 }
 a:=[]Rule{
  {Port:0,RangeStart:39000,RangeEnd:40000,Protocol:"tcp",Zone:"public",Backend:"firewalld"},
  {Port:39011,Protocol:"tcp",Zone:"public",Backend:"firewalld"},
  {Port:39011,Protocol:"udp",Zone:"public",Backend:"firewalld"},
 }
 markRuleConflicts(a)
 if !a[0].Conflict||!a[1].Conflict||a[2].Conflict{t.Fatalf("range overlap classification: %+v",a)}
}

func TestDockerPublishedMappings(t *testing.T){
 jsonLine:=`/web {"80/tcp":[{"HostIp":"0.0.0.0","HostPort":"18080"},{"HostIp":"::","HostPort":"18080"}],"53/udp":[{"HostIp":"0.0.0.0","HostPort":"15353"}],"9090/tcp":null}`
 rows,err:=parseDockerInspect([]byte(jsonLine))
 if err!=nil{t.Fatal(err)}
 if len(rows)!=2{t.Fatalf("expected deduplicated TCP/UDP host mappings: %+v",rows)}
 hasTCP,hasUDP:=false,false
 for _,p:=range rows{
  if p.Port==18080&&p.Protocol=="tcp"&&p.Container=="web"{hasTCP=true}
  if p.Port==15353&&p.Protocol=="udp"&&p.Container=="web"{hasUDP=true}
 }
 if !hasTCP||!hasUDP{t.Fatalf("wrong host mappings: %+v",rows)}
 for _,raw:=range []string{
  `/web not-json`,
  `missing-space`,
 }{
  if _,err:=parseDockerInspect([]byte(raw));err==nil{t.Errorf("malformed Docker inspect accepted: %s",raw)}
 }
}

func TestConflictSweepMatchesPairwiseReference(t *testing.T){
 random:=rand.New(rand.NewSource(20261107))
 zones:=[]string{"public","trusted"}
 backends:=[]string{"firewalld","ufw"}
 protocols:=[]string{"tcp","udp"}
 for sample:=0;sample<90;sample++{
  n:=random.Intn(180)+20
  rules:=make([]Rule,n)
  expected:=make([]bool,n)
  for i:=range rules{
   start:=random.Intn(500)+20000
   proto:=protocols[random.Intn(len(protocols))]
   zone:=zones[random.Intn(len(zones))]
   backend:=backends[random.Intn(len(backends))]
   if random.Intn(3)==0{
    rules[i]=Rule{RangeStart:start,RangeEnd:start+random.Intn(200)+1,Protocol:proto,Zone:zone,Backend:backend}
   }else{
    rules[i]=Rule{Port:start,Protocol:proto,Zone:zone,Backend:backend}
   }
  }
  for i:=range rules{
   for j:=i+1;j<len(rules);j++{
    a,b:=rules[i],rules[j]
    if a.Backend==b.Backend&&a.Zone==b.Zone&&a.Protocol==b.Protocol&&
     (a.ContainsPort(b.FirstPort())||b.ContainsPort(a.FirstPort())){
     expected[i],expected[j]=true,true
    }
   }
  }
  markRuleConflicts(rules)
  for i,r:=range rules{
   if r.Conflict!=expected[i]{
    t.Fatalf("sample %d rule %d: expected conflict=%t got=%t %+v",sample,i,expected[i],r.Conflict,r)
   }
  }
 }
}
