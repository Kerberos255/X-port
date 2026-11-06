// Package firewall reads the system firewall and manages only simple, explicit port rules.
// Native nftables rules remain read-only to preserve other rule owners.
package firewall

import (
 "context"
 "errors"
 "fmt"
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
}
type Snapshot struct {
 Backend string `json:"backend"`
 Active bool `json:"active"`
 DefaultZone string `json:"defaultZone,omitempty"`
 Zones []string `json:"zones"`
 Services map[string][]string `json:"services,omitempty"`
 Rules []Rule `json:"rules"`
 Notice string `json:"notice"`
}
var zoneRE=regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]{0,63}$`)

var ruleRE=regexp.MustCompile(`^([1-9][0-9]{0,4})/(tcp|udp)$`)

func command(ctx context.Context, name string, args ...string)(string,error){
 cctx,cancel:=context.WithTimeout(ctx,5*time.Second);defer cancel()
 cmd:=exec.CommandContext(cctx,name,args...)
 out,err:=cmd.CombinedOutput()
 if len(out)>256<<10{return "",fmt.Errorf("%s returned oversized output",name)}
 if err!=nil{
  if errors.Is(cctx.Err(),context.DeadlineExceeded){return "",fmt.Errorf("%s timed out",name)}
  return "",fmt.Errorf("%s failed: %w",name,err)
 }
 return strings.TrimSpace(string(out)),nil
}
func Detect(ctx context.Context)string{
 if _,err:=exec.LookPath("firewall-cmd");err==nil {
  if out,err:=command(ctx,"firewall-cmd","--state");err==nil&&strings.TrimSpace(out)=="running" {return "firewalld"}
 }
 if _,err:=exec.LookPath("ufw");err==nil {
  if out,err:=command(ctx,"ufw","status");err==nil&&strings.Contains(strings.ToLower(out),"status: active"){return "ufw"}
 }
 if _,err:=exec.LookPath("nft");err==nil{return "nftables"}
 if _,err:=exec.LookPath("iptables");err==nil{return "iptables"}
 return "none"
}
func ValidZone(zone string)bool{return zoneRE.MatchString(zone)}
func ValidRule(port int,proto string)bool{return port>=1&&port<=65535&&(proto=="tcp"||proto=="udp")}
func ParsePort(spec string)(int,string,bool){
 parts:=ruleRE.FindStringSubmatch(spec)
 if parts==nil{return 0,"",false}
 port,err:=strconv.Atoi(parts[1]);if err!=nil||!ValidRule(port,parts[2]){return 0,"",false}
 return port,parts[2],true
}
func activeZones(text string)[]string{
 out:=[]string{}
 for _,line:=range strings.Split(text,"\n"){
  if line==""||line[0]==' '||line[0]=='	'{continue}
  candidate:=strings.Fields(line);if len(candidate)==0{continue}
  if ValidZone(candidate[0]){out=append(out,candidate[0])}
 }
 return out
}
func Inspect(ctx context.Context)(Snapshot,error){
 backend:=Detect(ctx)
 snap:=Snapshot{Backend:backend,Active:backend=="firewalld"||backend=="ufw",Zones:[]string{},Rules:[]Rule{},
 Notice:"当前只列出简单端口规则；服务、富规则、网卡、来源及独立 nftables 链可能影响实际可达性。"}
 switch backend {
 case "firewalld":
  def,err:=command(ctx,"firewall-cmd","--get-default-zone");if err!=nil{return snap,err}
  def=strings.TrimSpace(def);if !ValidZone(def){return snap,errors.New("invalid default firewall zone")}
  snap.DefaultZone=def
  active,err:=command(ctx,"firewall-cmd","--get-active-zones");if err!=nil{return snap,err}
  zones:=map[string]bool{def:true};for _,z:=range activeZones(active){zones[z]=true}
  for z:=range zones{snap.Zones=append(snap.Zones,z)};sort.Strings(snap.Zones)
  snap.Services=make(map[string][]string)
  for _,zone:=range snap.Zones{
   runtime,err:=command(ctx,"firewall-cmd","--zone="+zone,"--list-ports");if err!=nil{return snap,err}
   persistent,err:=command(ctx,"firewall-cmd","--permanent","--zone="+zone,"--list-ports");if err!=nil{return snap,err}
   entries:=map[string]*Rule{}
   for _,pair:=range []struct{text string; runtime bool}{{runtime,true},{persistent,false}}{
    for _,spec:=range strings.Fields(pair.text){
     p,proto,ok:=ParsePort(spec);if !ok{continue}
     key:=fmt.Sprintf("%d/%s",p,proto)
     rule:=entries[key]
     if rule==nil{rule=&Rule{ID:"firewalld:"+zone+":"+key,Port:p,Protocol:proto,Zone:zone,Backend:"firewalld",Access:"区域："+zone};entries[key]=rule}
     if pair.runtime{rule.Runtime=true}else{rule.Permanent=true}
    }
   }
   for _,r:=range entries{snap.Rules=append(snap.Rules,*r)}
   srv,err:=command(ctx,"firewall-cmd","--zone="+zone,"--list-services");if err==nil{snap.Services[zone]=strings.Fields(srv)}
  }
 case "ufw":
  out,err:=command(ctx,"ufw","status");if err!=nil{return snap,err}
  // A rule is writable only when we can recognize an unrestricted numeric ALLOW.
  // Source-restricted and application rules remain under their original owner.
  seen:=map[string]*Rule{}
  for _,line:=range strings.Split(out,"\n"){
   p,proto,broad,ok:=ufwPortLine(line)
   if !ok||!broad{continue}
   key:=fmt.Sprintf("%d/%s",p,proto)
   if seen[key]==nil{seen[key]=&Rule{ID:"ufw:"+key,Port:p,Protocol:proto,Backend:"ufw",Runtime:true,Permanent:true,Access:"所有来源"}}
  }
  for _,r:=range seen{snap.Rules=append(snap.Rules,*r)}
  snap.Notice="UFW 仅显示可确认的简单端口放行规则；IPv4/IPv6 与来源限制请检查 UFW 原始规则。"
 case "nftables","iptables":
  snap.Notice="检测到自定义规则管理方式。为避免覆盖其他管理器的配置，网页仅提供底层规则只读查看。"
 case "none":
  snap.Notice="未检测到正在运行的受支持防火墙管理器。"
 }
 sort.Slice(snap.Rules,func(i,j int)bool{a,b:=snap.Rules[i],snap.Rules[j];if a.Port!=b.Port{return a.Port<b.Port};if a.Protocol!=b.Protocol{return a.Protocol<b.Protocol};return a.Zone<b.Zone})
 return snap,nil
}
func containsRule(s Snapshot,port int,proto,zone string)(Rule,bool){
 for _,r:=range s.Rules{if r.Port==port&&r.Protocol==proto&&r.Zone==zone{return r,true}}
 return Rule{},false
}
func Change(ctx context.Context,action string,port int,proto,zone string)error{
 if action!="add"&&action!="remove" {return errors.New("unsupported firewall action")}
 if !ValidRule(port,proto){return errors.New("invalid port or protocol")}
 snap,err:=Inspect(ctx);if err!=nil{return err}
 rule,exists:=containsRule(snap,port,proto,zone)
 switch snap.Backend{
 case "firewalld":
  if !ValidZone(zone){return errors.New("invalid zone")}
  found:=false;for _,z:=range snap.Zones{if z==zone{found=true;break}};if !found{return errors.New("zone is not active/default")}
  if exists&&rule.Runtime!=rule.Permanent{return errors.New("runtime and permanent rules differ; resolve with firewalld before editing")}
  if (action=="add"&&exists)||(action=="remove"&&!exists){return errors.New("rule already in requested state")}
  flag:="--add-port=";rollback:="--remove-port=";if action=="remove"{flag,rollback=rollback,flag}
  spec:=fmt.Sprintf("%d/%s",port,proto)
  if _,err:=command(ctx,"firewall-cmd","--zone="+zone,flag+spec);err!=nil{return err}
  if _,err:=command(ctx,"firewall-cmd","--permanent","--zone="+zone,flag+spec);err!=nil{
   _,rbErr:=command(ctx,"firewall-cmd","--zone="+zone,rollback+spec)
   if rbErr!=nil{return fmt.Errorf("permanent operation failed and runtime rollback failed: %w",err)}
   return fmt.Errorf("permanent operation failed; runtime rolled back: %w",err)
  }
 case "ufw":
  if zone!=""{return errors.New("ufw does not support zones")}
  // Refuse changes when a port has rules with different semantics.
  // "ufw delete allow" could otherwise target a rule the UI cannot explain.
  ufwStatus,err:=command(ctx,"ufw","status");if err!=nil{return err}
  for _,line:=range strings.Split(ufwStatus,"\n"){
   p,protocol,broad,ok:=ufwPortLine(line)
   if ok&&p==port&&protocol==proto&&!broad{return errors.New("port has restricted or non-ALLOW UFW rules; use UFW directly")}
  }
  if (action=="add"&&exists)||(action=="remove"&&!exists){return errors.New("rule already in requested state")}
  args:=[]string{"allow",fmt.Sprintf("%d/%s",port,proto)}
  if action=="remove"{args=append([]string{"--force","delete"},args...)}
  if _,err:=command(ctx,"ufw",args...);err!=nil{return err}
 default:return errors.New("firewall backend is read-only")
 }
 return nil
}
func Raw(ctx context.Context)(string,error){
 if _,err:=exec.LookPath("nft");err==nil{return command(ctx,"nft","-a","list","ruleset")}
 if _,err:=exec.LookPath("iptables-save");err==nil{return command(ctx,"iptables-save")}
 return "",errors.New("no native ruleset reader found")
}

// ufwPortLine recognizes numeric port rows in "ufw status" (including (v6)).
// Non-ALLOW, source-restricted and application rows never become editable.
func ufwPortLine(line string)(port int,proto string,broad bool,ok bool){
 fields:=strings.Fields(strings.TrimSpace(line))
 if len(fields)<3{return 0,"",false,false}
 p,protocol,valid:=ParsePort(fields[0]);if !valid{return 0,"",false,false}
 i:=1
 if fields[i]=="(v6)"{i++}
 if i>=len(fields){return p,protocol,false,true}
 if fields[i]!="ALLOW"{return p,protocol,false,true}
 i++
 if i<len(fields)&&fields[i]=="IN"{i++}
 if i>=len(fields)||fields[i]!="Anywhere"{return p,protocol,false,true}
 i++
 if i<len(fields)&&fields[i]=="(v6)"{i++}
 return p,protocol,i==len(fields),true
}
