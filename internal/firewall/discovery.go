package firewall

import (
 "context"
 "encoding/json"
 "errors"
 "fmt"
 "os"
 "os/exec"
 "regexp"
 "sort"
 "strconv"
 "strings"
 "time"
)

// Discovery is read-only host context. It is not proof that a firewall
// allowance is reachable through NAT, a cloud security group or another chain.
type Discovery struct {
 Processes []ListeningProcess `json:"processes"`
 SSHPorts []int `json:"sshPorts"`
 SSHUncertain bool `json:"sshUncertain"`
 FTPRanges []ConfiguredRange `json:"ftpRanges"`
 Docker []PublishedPort `json:"docker"`
 DockerUncertain bool `json:"dockerUncertain"`
}
type ListeningProcess struct {
 Port int `json:"port"`
 Protocol string `json:"protocol"`
 PID int `json:"pid,omitempty"`
 Name string `json:"name,omitempty"`
}
type ConfiguredRange struct {
 Start int `json:"start"`
 End int `json:"end"`
 Protocol string `json:"protocol"`
 Service string `json:"service"`
}
type PublishedPort struct {
 Port int `json:"port"`
 Protocol string `json:"protocol"`
 Container string `json:"container"`
}

var ownerRE=regexp.MustCompile(`"([^"\n]{1,64})",pid=([0-9]{1,9})`)
var sshPortRE=regexp.MustCompile(`(?m)^port[ 	]+([0-9]{1,5})[ 	]*$`)
var socketPortRE=regexp.MustCompile(`(?m)([0-9]{1,5})[ \t]+[(](Stream|Datagram)[)]`)
var numberRangeRE=regexp.MustCompile(`^([0-9]{1,5})[[:space:],-]+([0-9]{1,5})$`)

func parseProcessSockets(raw string)([]ListeningProcess,error){
 entries:=make([]ListeningProcess,0,32)
 seen:=make(map[string]bool)
 for _,line:=range strings.Split(raw,"\n"){
  fields:=strings.Fields(line)
  if len(fields)==0{continue}
  proto:=strings.TrimSuffix(strings.ToLower(fields[0]),"6")
  if proto!="tcp"&&proto!="udp"{return nil,errors.New("unrecognized listening transport")}
  if len(fields)<5{return nil,errors.New("truncated listening socket")}
  _,s,err:=parseLocalSocket(fields[4])
  if err!=nil{return nil,err}
  p,err:=strconv.Atoi(s)
  if err!=nil||!ValidRule(p,proto){return nil,errors.New("invalid listening socket port")}
  owners:=ownerRE.FindAllStringSubmatch(line,-1)
  if len(owners)==0{owners=[][]string{{"","", "0"}}}
  for _,owner:=range owners{
   pid,_:=strconv.Atoi(owner[2])
   name:=strings.TrimSpace(owner[1])
   if len(name)>64{name=name[:64]}
   key:=fmt.Sprintf("%s/%d/%d/%s",proto,p,pid,name)
   if seen[key]{continue}
   seen[key]=true
   entries=append(entries,ListeningProcess{Port:p,Protocol:proto,PID:pid,Name:name})
   if len(entries)>2048{return nil,errors.New("too many listening processes")}
  }
 }
 sort.Slice(entries,func(i,j int)bool{
  a,b:=entries[i],entries[j]
  if a.Port!=b.Port{return a.Port<b.Port}
  if a.Protocol!=b.Protocol{return a.Protocol<b.Protocol}
  return a.PID<b.PID
 })
 return entries,nil
}
func parseLocalSocket(local string)(string,string,error){
 host,port,err:=splitHostPortLoose(local)
 if err!=nil{return "","",err}
 return host,port,nil
}
func splitHostPortLoose(local string)(string,string,error){
 if strings.HasPrefix(local,"["){
  host,port,err:=splitBracketedLocal(local)
  if err==nil{return host,port,nil}
 }
 i:=strings.LastIndexByte(local,':')
 if i<0{return "","",errors.New("invalid local socket address")}
 return local[:i],local[i+1:],nil
}
func splitBracketedLocal(local string)(string,string,error){
 i:=strings.LastIndex(local,"]:")
 if i<0{return "","",errors.New("invalid bracketed local socket")}
 return local[1:i],local[i+2:],nil
}
func readSSProcesses(ctx context.Context)([]ListeningProcess,error){
 out,err:=command(ctx,"ss","-H","-lntup")
 if err!=nil{return nil,err}
 return parseProcessSockets(out)
}
func parseSSHSettings(out string)[]int{
 ports:=make(map[int]bool)
 for _,m:=range sshPortRE.FindAllStringSubmatch(strings.ToLower(out),-1){
  p,_:=strconv.Atoi(m[1]);if ValidRule(p,"tcp"){ports[p]=true}
 }
 return sortedPortKeys(ports)
}
func sortedPortKeys(m map[int]bool)[]int{
 v:=make([]int,0,len(m));for p:=range m{v=append(v,p)};sort.Ints(v);return v
}
func SSHProtection(ctx context.Context,processes []ListeningProcess)([]int,bool){
 ports:=map[int]bool{}
 uncertain:=false
 // A listening sshd process remains protected even when the config has drifted.
 for _,p:=range processes{
  if p.Protocol=="tcp"&&(p.Name=="sshd"||p.Name=="sshd-session"||p.Name=="dropbear"){ports[p.Port]=true}
 }
 if _,err:=exec.LookPath("sshd");err==nil{
  out,e:=command(ctx,"sshd","-T")
  if e!=nil{uncertain=true}else{
   parsed:=parseSSHSettings(out)
   if len(parsed)==0{uncertain=true}
   for _,p:=range parsed{ports[p]=true}
  }
 }
 // systemd socket activation can override sshd_config's Port directive.
 for _,unit:=range []string{"sshd.socket","ssh.socket"}{
  cctx,cancel:=context.WithTimeout(ctx,2*time.Second)
  out,e:=exec.CommandContext(cctx,"systemctl","show",unit,"--property=Listen","--value").Output()
  cancel()
  if e!=nil{continue}
  for _,m:=range socketPortRE.FindAllStringSubmatch(string(out),-1){
   p,_:=strconv.Atoi(m[1]);if ValidRule(p,"tcp"){ports[p]=true}
  }
 }
 // Default SSH remains protected even if no SSH daemon is installed.
 ports[22]=true
 return sortedPortKeys(ports),uncertain
}
func parseFTPRange(raw,protocol,service string)(ConfiguredRange,bool){
 m:=numberRangeRE.FindStringSubmatch(strings.TrimSpace(raw))
 if m==nil{return ConfiguredRange{},false}
 start,_:=strconv.Atoi(m[1]);end,_:=strconv.Atoi(m[2])
 if !ValidRule(start,protocol)||!ValidRule(end,protocol)||start>end{return ConfiguredRange{},false}
 return ConfiguredRange{Start:start,End:end,Protocol:protocol,Service:service},true
}
func readFTPRanges()[]ConfiguredRange{
 paths:=[]struct{path,service string}{
  {"/etc/pure-ftpd/conf/PassivePortRange","Pure-FTPd"},
  {"/etc/pure-ftpd/pure-ftpd.conf","Pure-FTPd"},
  {"/etc/pure-ftpd.conf","Pure-FTPd"},
  {"/etc/vsftpd/vsftpd.conf","vsftpd"},
  {"/etc/proftpd/proftpd.conf","ProFTPD"},
 }
 seen:=map[string]bool{};var ranges []ConfiguredRange
 for _,item:=range paths{
  info,err:=os.Lstat(item.path)
  if err!=nil||!info.Mode().IsRegular()||info.Size()>128<<10{continue}
  raw,err:=os.ReadFile(item.path);if err!=nil{continue}
  min,max:="",""
  for _,rawLine:=range strings.Split(string(raw),"\n"){
   line:=strings.TrimSpace(strings.SplitN(rawLine,"#",2)[0])
   if line==""{continue}
   lower:=strings.ToLower(line)
   var value string
   switch{
   case strings.HasPrefix(lower,"passiveportrange"):
    value=strings.TrimSpace(line[len("passiveportrange"):])
   case strings.HasPrefix(lower,"passiveports"):
    value=strings.TrimSpace(line[len("passiveports"):])
   case strings.HasPrefix(lower,"pasv_min_port="):
    min=strings.TrimSpace(line[len("pasv_min_port="):]);continue
   case strings.HasPrefix(lower,"pasv_max_port="):
    max=strings.TrimSpace(line[len("pasv_max_port="):]);continue
   default:
    if item.path=="/etc/pure-ftpd/conf/PassivePortRange"{value=line}else{continue}
   }
   if r,ok:=parseFTPRange(value,"tcp",item.service);ok{
    key:=fmt.Sprintf("%d:%d:%s",r.Start,r.End,r.Service)
    if !seen[key]{seen[key]=true;ranges=append(ranges,r)}
   }
  }
  if min!=""&&max!=""{
   if r,ok:=parseFTPRange(min+" "+max,"tcp",item.service);ok{
    key:=fmt.Sprintf("%d:%d:%s",r.Start,r.End,r.Service)
    if !seen[key]{seen[key]=true;ranges=append(ranges,r)}
   }
  }
 }
 return ranges
}
var dockerIDRE=regexp.MustCompile(`^[a-f0-9]{12,64}$`)
func readDocker(ctx context.Context)([]PublishedPort,bool){
 if _,err:=os.Stat("/var/run/docker.sock");err!=nil{return nil,false}
 executable,err:=exec.LookPath("docker")
 if err!=nil{return nil,true}
 cctx,cancel:=context.WithTimeout(ctx,5*time.Second)
 defer cancel()
 ps:=exec.CommandContext(cctx,executable,"--host","unix:///var/run/docker.sock","ps","-q","--no-trunc")
 b,err:=ps.Output()
 if err!=nil||len(b)>16<<10{return nil,true}
 ids:=strings.Fields(string(b))
 if len(ids)>64{return nil,true}
 if len(ids)==0{return nil,false}
 for _,id:=range ids{if !dockerIDRE.MatchString(id){return nil,true}}
 args:=[]string{"--host","unix:///var/run/docker.sock","inspect","--format",`{{.Name}} {{json .NetworkSettings.Ports}}`}
 args=append(args,ids...)
 cmd:=exec.CommandContext(cctx,executable,args...)
 buf,err:=cmd.Output()
 if err!=nil||len(buf)>256<<10{return nil,true}
 ports,err:=parseDockerInspect(buf)
 if err!=nil{return nil,true}
 return ports,false

}
func parseDockerInspect(buf []byte)([]PublishedPort,error){
 var ports []PublishedPort
 seen:=map[string]bool{}
 for _,line:=range strings.Split(strings.TrimSpace(string(buf)),string(byte(10))){
  if line==""{continue}
  parts:=strings.SplitN(line," ",2);if len(parts)!=2{return nil,errors.New("invalid docker inspect row")}
  name:=strings.TrimPrefix(parts[0],"/")
  if len(name)>80{name=name[:80]}
  var bindings map[string][]struct{HostPort string `json:"HostPort"`}
  if err:=json.Unmarshal([]byte(parts[1]),&bindings);err!=nil{return nil,err}
  for protoPort,items:=range bindings{
   split:=strings.Split(protoPort,"/")
   if len(split)!=2||!ValidRule(1,split[1]){continue}
   for _,item:=range items{
    p,e:=strconv.Atoi(item.HostPort);if e!=nil||!ValidRule(p,split[1]){continue}
    key:=fmt.Sprintf("%s/%d/%s",split[1],p,name)
    if seen[key]{continue};seen[key]=true
    ports=append(ports,PublishedPort{Port:p,Protocol:split[1],Container:name})
    if len(ports)>2048{return nil,errors.New("too many docker port mappings")}
   }
  }
 }
 sort.Slice(ports,func(i,j int)bool{if ports[i].Port!=ports[j].Port{return ports[i].Port<ports[j].Port};return ports[i].Container<ports[j].Container})
 return ports,nil
}
func Discover(ctx context.Context)(Discovery,error){
 processes,err:=readSSProcesses(ctx)
 if err!=nil{return Discovery{},err}
 sshPorts,sshUnknown:=SSHProtection(ctx,processes)
 docker,dockerUnknown:=readDocker(ctx)
 return Discovery{Processes:processes,SSHPorts:sshPorts,SSHUncertain:sshUnknown,FTPRanges:readFTPRanges(),Docker:docker,DockerUncertain:dockerUnknown},nil
}
// ProtectedPort must be checked in the privileged helper as well as the Web
// read model: otherwise a stale browser snapshot could bypass a new SSH port.
func (d Discovery) ProtectedPort(p int)bool{
 if d.SSHUncertain||d.DockerUncertain{return true}
 for _,v:=range d.SSHPorts{if v==p{return true}}
 for _,v:=range d.Processes{if v.Port==p{return true}}
 for _,v:=range d.Docker{if v.Port==p{return true}}
 return false
}
