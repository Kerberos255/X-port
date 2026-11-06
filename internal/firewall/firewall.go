// Package firewall inspects host firewall managers and only changes narrow port allowances.
// Native netfilter rules and ambiguous ownership are intentionally read-only.
package firewall

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"net"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type Rule struct {
	ID string `json:"id"`
	Port int `json:"port"`
	Protocol string `json:"protocol"`
	Zone string `json:"zone,omitempty"`
	Backend string `json:"backend"`
	Runtime bool `json:"runtime"`
	Permanent bool `json:"permanent"`
	Access string `json:"access"`
	IPv4 bool `json:"ipv4"`
	IPv6 bool `json:"ipv6"`
}
type Snapshot struct {
	Backend string `json:"backend"`
	Managers []string `json:"managers"`
	Active bool `json:"active"`
	Writable bool `json:"writable"`
	UnsafeReason string `json:"unsafeReason,omitempty"`
	DefaultZone string `json:"defaultZone,omitempty"`
	Zones []string `json:"zones"`
	Services map[string][]string `json:"services,omitempty"`
	Rules []Rule `json:"rules"`
	Notice string `json:"notice"`
}
var zoneRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)
var portRE = regexp.MustCompile(`^([1-9][0-9]{0,4})/(tcp|udp)$`)

func command(ctx context.Context, name string, args ...string) (string,error) {
	cctx,cancel:=context.WithTimeout(ctx,5*time.Second);defer cancel()
	cmd:=exec.CommandContext(cctx,name,args...)
 cmd.Env=append(os.Environ(),"LC_ALL=C","LANG=C")
	out,err:=cmd.CombinedOutput()
	if len(out)>256<<10{return "",fmt.Errorf("%s output too large",name)}
	if err!=nil {
		if errors.Is(cctx.Err(),context.DeadlineExceeded){return "",fmt.Errorf("%s timed out",name)}
		return "",fmt.Errorf("%s failed: %w",name,err)
	}
	return strings.TrimSpace(string(out)),nil
}
func hasTool(name string)bool{_,err:=exec.LookPath(name);return err==nil}
func ValidZone(z string)bool{return zoneRE.MatchString(z)}
func ValidRule(p int,proto string)bool{return p>0&&p<=65535&&(proto=="tcp"||proto=="udp")}
func ParsePort(s string)(int,string,bool){
	m:=portRE.FindStringSubmatch(s);if m==nil{return 0,"",false}
	n,err:=strconv.Atoi(m[1]);return n,m[2],err==nil&&ValidRule(n,m[2])
}
func activeZones(raw string)[]string{
	var zones []string
	for _,line:=range strings.Split(raw,"\n"){
		if line==""||line[0]==' '||line[0]=='	'{continue}
		parts:=strings.Fields(line)
		if len(parts)>0&&ValidZone(parts[0]){zones=append(zones,parts[0])}
	}
	return zones
}
func runningManagers(ctx context.Context)(firewalld,ufw bool){
	if hasTool("firewall-cmd"){
		out,err:=command(ctx,"firewall-cmd","--state")
		firewalld=err==nil&&strings.EqualFold(strings.TrimSpace(out),"running")
	}
	if hasTool("ufw"){
		out,err:=command(ctx,"ufw","status")
		ufw=err==nil&&strings.HasPrefix(strings.ToLower(strings.TrimSpace(out)),"status: active")
	}
	return
}
func Detect(ctx context.Context)string{
	f,u:=runningManagers(ctx)
	if f&&u{return "mixed"}
	if f{return "firewalld"}
	if u{return "ufw"}
	if hasTool("nft"){return "nftables"}
	if hasTool("iptables"){return "iptables"}
	return "none"
}

// independentInputChains distinguishes native firewall base chains from the
// tables managed by firewalld itself. A foreign input hook makes safe editing
// impossible without understanding all participating policies.
// nftables userspace also shows iptables-nft chains created by UFW. A
// recognized UFW filter/INPUT chain must not be mistaken for an independent
// firewall. Accept only the standard UFW top-level jumps; any foreign rule
// or another base chain is still a conflict and makes the WebUI read-only.
type nftChain struct {
 Family string `json:"family"`
 Table string `json:"table"`
 Name string `json:"name"`
 Type string `json:"type"`
 Hook string `json:"hook"`
}
type nftRule struct {
 Family string `json:"family"`
 Table string `json:"table"`
 Chain string `json:"chain"`
 Expr []map[string]json.RawMessage `json:"expr"`
}
func ufwManagedInput(c nftChain, objects []map[string]json.RawMessage) bool {
 if c.Name!="INPUT"||c.Table!="filter"||(c.Family!="ip"&&c.Family!="ip6"){return false}
 prefix:="ufw-";if c.Family=="ip6"{prefix="ufw6-"}
 required:=map[string]bool{prefix+"before-input":false,prefix+"after-input":false,prefix+"reject-input":false,prefix+"track-input":false}
 rules:=0
 for _,obj:=range objects{
  raw:=obj["rule"];if len(raw)==0{continue}
  var r nftRule
  if json.Unmarshal(raw,&r)!=nil{return false}
  if r.Family!=c.Family||r.Table!=c.Table||r.Chain!=c.Name{continue}
  rules++
  if len(r.Expr)==0{return false}
  sawJump:=false
  for _,item:=range r.Expr{
   if len(item)!=1{return false}
   for kind,data:=range item{
    if kind=="counter"{continue}
    if kind!="jump"{return false}
    var target struct{Target string `json:"target"`}
    if json.Unmarshal(data,&target)!=nil||!strings.HasPrefix(target.Target,prefix){return false}
    sawJump=true
    if _,ok:=required[target.Target];ok{required[target.Target]=true}
   }
  }
  if !sawJump{return false}
 }
 if rules==0{return false}
 for _,present:=range required{if !present{return false}}
 return true
}
func independentInputChainBlocker(data string, ufwActive bool)(string,error){
 var tree struct{Nftables []map[string]json.RawMessage `json:"nftables"`}
 if err:=json.Unmarshal([]byte(data),&tree);err!=nil{return "",err}
 if len(tree.Nftables)==0{return "",errors.New("nftables ruleset response is empty")}
 for _,obj:=range tree.Nftables{
  raw:=obj["chain"];if len(raw)==0{continue}
  var c nftChain
  if json.Unmarshal(raw,&c)!=nil{return "unparseable chain",nil}
  switch c.Hook {
  case "input","prerouting","ingress":
  default:continue
  }
  // Only the inet table is owned by firewalld. An unrelated ip/ip6 table
  // with the same name is still an independent firewall chain.
  if c.Table=="firewalld"&&c.Family=="inet"{continue}
  // An ordinary NAT PREROUTING chain alters addresses/ports, but does not
  // represent an independent input filter. Keep unknown or non-NAT chains
  // fail-closed; NAT can still affect effective reachability.
  if c.Hook=="prerouting"&&c.Type=="nat"{continue}
  if ufwActive&&c.Hook=="input"&&ufwManagedInput(c,tree.Nftables){continue}
  return fmt.Sprintf("%s/%s/%s (type=%s, hook=%s)",c.Family,c.Table,c.Name,c.Type,c.Hook),nil
 }
 return "",nil
}
func independentInputChainsWithUFW(data string, ufwActive bool)(bool,error){
 blocker,err:=independentInputChainBlocker(data,ufwActive)
 return blocker!="",err
}
func independentInputChains(data string)(bool,error){
 return independentInputChainsWithUFW(data,false)
}
func nativeInputBlocker(ctx context.Context, ufwActive bool)(string,error){
 if !hasTool("nft"){return "",errors.New("nftables inspector is unavailable")}
 out,err:=command(ctx,"nft","-j","list","ruleset")
 if err!=nil{return "",err}
 return independentInputChainBlocker(out,ufwActive)
}
func Inspect(ctx context.Context)(Snapshot,error){
	f,u:=runningManagers(ctx)
	snap:=Snapshot{Zones:[]string{},Rules:[]Rule{},Managers:[]string{},
		Notice:"只显示简单端口规则；服务、富规则、来源限制、NAT 与其他链可能影响实际连通性。"}
	if f{snap.Managers=append(snap.Managers,"firewalld")}
	if u{snap.Managers=append(snap.Managers,"ufw")}
	switch {
 case f&&u:snap.Backend="mixed"
 case f:snap.Backend="firewalld"
 case u:snap.Backend="ufw"
 case hasTool("nft"):snap.Backend="nftables"
 case hasTool("iptables"):snap.Backend="iptables"
 default:snap.Backend="none"
 }
	snap.Active=f||u
	if f&&u{
		snap.Backend="mixed";snap.UnsafeReason="firewalld 与 UFW 同时活动，暂停网页写操作"
		snap.Notice="发现多个活动防火墙管理器，请先检查规则所有权；网页只读。"
		return snap,nil
	}
	snap.Writable=f||u
	if f||u{
		if blocker,err:=nativeInputBlocker(ctx,u);err!=nil{
			snap.Writable=false;snap.UnsafeReason="无法确认底层 nftables 规则链："+err.Error()
		}else if blocker!=""{
			snap.Writable=false;snap.UnsafeReason="检测到独立 nftables 流量处理链 "+blocker+"，无法确认规则顺序"
		}
	}
	switch snap.Backend{
	case "firewalld":
		def,err:=command(ctx,"firewall-cmd","--get-default-zone");if err!=nil{return snap,err}
		if !ValidZone(def){return snap,errors.New("invalid firewalld default zone")}
		snap.DefaultZone=def
		act,err:=command(ctx,"firewall-cmd","--get-active-zones");if err!=nil{return snap,err}
		zones:=map[string]bool{def:true};for _,z:=range activeZones(act){zones[z]=true}
		for z:=range zones{snap.Zones=append(snap.Zones,z)};sort.Strings(snap.Zones)
		snap.Services=map[string][]string{}
		for _,zone:=range snap.Zones{
			rt,err:=command(ctx,"firewall-cmd","--zone="+zone,"--list-ports");if err!=nil{return snap,err}
			pm,err:=command(ctx,"firewall-cmd","--permanent","--zone="+zone,"--list-ports");if err!=nil{return snap,err}
			entries:=map[string]*Rule{}
			for _,batch:=range []struct{text string;runtime bool}{{rt,true},{pm,false}}{
				for _,spec:=range strings.Fields(batch.text){
					port,proto,ok:=ParsePort(spec);if !ok{continue}
					r:=entries[spec]
					if r==nil{r=&Rule{ID:"firewalld:"+zone+":"+spec,Port:port,Protocol:proto,Zone:zone,Backend:"firewalld",Access:"区域："+zone};entries[spec]=r}
					if batch.runtime{r.Runtime=true}else{r.Permanent=true}
				}
			}
			for _,r:=range entries{snap.Rules=append(snap.Rules,*r)}
			if srv,err:=command(ctx,"firewall-cmd","--zone="+zone,"--list-services");err==nil{snap.Services[zone]=strings.Fields(srv)}
		}
	case "ufw":
		out,err:=command(ctx,"ufw","status");if err!=nil{return snap,err}
		seen:=map[string]*Rule{}
		for _,line:=range strings.Split(out,"\n"){
			p,proto,broad,v6,ok:=ufwPortLine(line)
			if !ok||!broad{continue}
			key:=fmt.Sprintf("%d/%s",p,proto)
			r:=seen[key];if r==nil{r=&Rule{ID:"ufw:"+key,Port:p,Protocol:proto,Backend:"ufw",Runtime:true,Permanent:true,Access:"所有来源"};seen[key]=r}
			if v6{r.IPv6=true}else{r.IPv4=true}
		}
		for _,r:=range seen{snap.Rules=append(snap.Rules,*r)}
		snap.Notice="UFW 仅列出无来源限制的明确放行规则；其他规则须经原管理器检查。"
	case "nftables","iptables":
		snap.Notice="原生规则只读；检测到对应命令不代表系统防火墙已启用。"
	case "none":
		snap.Notice="未发现活动的受支持防火墙管理器。"
	}
	sort.Slice(snap.Rules,func(i,j int)bool{a,b:=snap.Rules[i],snap.Rules[j];if a.Port!=b.Port{return a.Port<b.Port};if a.Protocol!=b.Protocol{return a.Protocol<b.Protocol};return a.Zone<b.Zone})
	return snap,nil
}
func containsRule(s Snapshot,p int,proto,zone string)(Rule,bool){
	for _,r:=range s.Rules{if r.Port==p&&r.Protocol==proto&&r.Zone==zone{return r,true}}
	return Rule{},false
}
func RuleID(backend,zone string,p int,proto string)string{
	spec:=fmt.Sprintf("%d/%s",p,proto)
	if backend=="firewalld"{return "firewalld:"+zone+":"+spec}
	return "ufw:"+spec
}
func Change(ctx context.Context,action string,p int,proto,zone string)error{
	if action!="add"&&action!="remove"{return errors.New("unsupported action")}
	if !ValidRule(p,proto){return errors.New("invalid port/protocol")}
	snap,err:=Inspect(ctx);if err!=nil{return err}
	if !snap.Writable{return fmt.Errorf("firewall is read-only: %s",snap.UnsafeReason)}
	r,exists:=containsRule(snap,p,proto,zone)
	switch snap.Backend {
	case "firewalld":
		if !ValidZone(zone){return errors.New("invalid zone")}
		ok:=false;for _,z:=range snap.Zones{if z==zone{ok=true;break}};if !ok{return errors.New("unknown zone")}
		if exists&&r.Runtime!=r.Permanent{return errors.New("runtime and permanent differ; resolve externally")}
		if (action=="add"&&exists)||(action=="remove"&&!exists){return errors.New("rule already in requested state")}
		flag,undo:="--add-port=","--remove-port=";if action=="remove"{flag,undo=undo,flag}
		spec:=fmt.Sprintf("%d/%s",p,proto)
		if _,err:=command(ctx,"firewall-cmd","--zone="+zone,flag+spec);err!=nil{return err}
		if _,err:=command(ctx,"firewall-cmd","--permanent","--zone="+zone,flag+spec);err!=nil{
			_,rollback:=command(ctx,"firewall-cmd","--zone="+zone,undo+spec)
			if rollback!=nil{return fmt.Errorf("permanent update failed and runtime rollback failed: %w",err)}
			return fmt.Errorf("permanent update failed; runtime rolled back: %w",err)
		}
	case "ufw":
		if zone!=""{return errors.New("ufw does not use zones")}
		status,err:=command(ctx,"ufw","status");if err!=nil{return err}
		for _,line:=range strings.Split(status,"\n"){
			port,protocol,broad,_,ok:=ufwPortLine(line)
			if ok&&port==p&&protocol==proto&&!broad{return errors.New("port overlaps another UFW rule; use UFW directly")}
		}
		if (action=="add"&&exists)||(action=="remove"&&!exists){return errors.New("rule already in requested state")}
		args:=[]string{"allow",fmt.Sprintf("%d/%s",p,proto)}
		if action=="remove"{args=append([]string{"--force","delete"},args...)}
		_,err=command(ctx,"ufw",args...);if err!=nil{return err}
	default:return errors.New("firewall backend is read-only")
	}
	return nil
}
func Raw(ctx context.Context)(string,error){
	if hasTool("nft"){return command(ctx,"nft","-a","list","ruleset")}
	if hasTool("iptables-save"){return command(ctx,"iptables-save")}
	return "",errors.New("no native ruleset reader")
}
func ufwPortLine(line string)(port int,proto string,broad,v6,ok bool){
	f:=strings.Fields(strings.TrimSpace(line))
	if len(f)<3{return 0,"",false,false,false}
	p,pr,valid:=ParsePort(f[0]);if !valid{return 0,"",false,false,false}
	i:=1
	if f[i]=="(v6)"{v6=true;i++}
	if i>=len(f)||f[i]!="ALLOW"{return p,pr,false,v6,true}
	i++
	if i<len(f)&&f[i]=="IN"{i++}
	if i>=len(f)||f[i]!="Anywhere"{return p,pr,false,v6,true}
	i++
	if i<len(f)&&f[i]=="(v6)"{v6=true;i++}
	return p,pr,i==len(f),v6,true
}

// ListeningPorts lists all host TCP/UDP listening endpoints. If the
// host cannot report them, removal fails closed to protect remote access.
func ListeningPorts(ctx context.Context)(map[int]bool,error){
	if !hasTool("ss"){return nil,errors.New("ss command unavailable")}
	out,err:=command(ctx,"ss","-H","-lnut")
	if err!=nil{return nil,err}
	ports:=map[int]bool{}
	for _,line:=range strings.Split(out,"\n"){
		fields:=strings.Fields(line)
		if len(fields)<4{continue}
		// ss output may start with "tcp"/"udp" (Netid) or directly with
		// LISTEN. Find the first endpoint containing a numeric local port.
		for _,local:=range fields[3:]{
			if !strings.Contains(local,":"){continue}
			_,port,err:=net.SplitHostPort(local)
			if err!=nil{
				i:=strings.LastIndex(local,":")
				if i<0{continue}
				port=local[i+1:]
			}
			if n,err:=strconv.Atoi(port);err==nil&&n>0&&n<=65535{
				ports[n]=true
				break
			}
		}
	}
	return ports,nil
}

 // Restore is a narrowly-scoped emergency re-open operation. It is called
 // by a pre-scheduled systemd timer if the WebUI fails to confirm connectivity.
 // Unlike normal edits, an independent chain cannot cancel an attempted recovery.
func Restore(ctx context.Context,backend string,port int,proto,zone string)error{
 if !ValidRule(port,proto){return errors.New("invalid recovery port")}
 spec:=fmt.Sprintf("%d/%s",port,proto)
 switch backend{
 case "firewalld":
  if !ValidZone(zone){return errors.New("invalid recovery zone")}
  if _,err:=command(ctx,"firewall-cmd","--zone="+zone,"--add-port="+spec);err!=nil{return err}
  _,err:=command(ctx,"firewall-cmd","--permanent","--zone="+zone,"--add-port="+spec)
  return err
 case "ufw":
  if zone!=""{return errors.New("unexpected UFW zone")}
  _,err:=command(ctx,"ufw","allow",spec);return err
 default:return errors.New("unsupported recovery backend")
 }
}
