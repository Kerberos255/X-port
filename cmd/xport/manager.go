package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Kerberos255/X-port/internal/backup"
	"github.com/Kerberos255/X-port/internal/buildinfo"
	"github.com/Kerberos255/X-port/internal/ops"
	"github.com/Kerberos255/X-port/internal/selfupdate"
	"github.com/Kerberos255/X-port/internal/store"
	"github.com/Kerberos255/X-port/internal/xray"
)

const (
	managerDataDir    = "/etc/x-port"
	managerXrayBinary = "/usr/local/x-port/bin/xray"
	managerXrayConfig = "/etc/x-port/xray/config.json"
	managerPanelUnit  = "xport.service"
	managerXrayUnit   = "xport-xray.service"
	managerMigration  = "/usr/local/lib/xport/migrate-xpanel.sh"
	managerBinary     = "/usr/local/bin/xport"
)

// Keep the low-level CLI in main.go stable for scripts while exposing the
// human-facing manager from the same binary. go test always passes -test.*
// arguments, so this pre-main dispatcher is inactive during tests.
func init() {
	if len(os.Args) == 1 {
		if err := runManagerMenu(); err != nil { fmt.Fprintln(os.Stderr, "xport:", err); os.Exit(1) }
		os.Exit(0)
	}
	if len(os.Args) < 2 || !isManagerCommand(os.Args[1]) { return }
	if err := runManagerCommand(os.Args[1], os.Args[2:]); err != nil { fmt.Fprintln(os.Stderr, "xport:", err); os.Exit(1) }
	os.Exit(0)
}

func isManagerCommand(cmd string) bool {
	switch cmd {
	case "menu", "status", "start", "stop", "restart", "restart-xray", "logs", "autostart", "settings", "admin", "update", "update-xray", "update-geodata", "backup", "firewall", "repair", "uninstall":
		return true
	default:
		return false
	}
}

func runManagerCommand(cmd string, args []string) error {
	switch cmd {
	case "menu": return runManagerMenu()
	case "status": return printManagerStatus(os.Stdout)
	case "start": return serviceAction("start", managerXrayUnit, managerPanelUnit)
	case "stop": return serviceAction("stop", managerPanelUnit, managerXrayUnit)
	case "restart": return serviceAction("restart", managerXrayUnit, managerPanelUnit)
	case "restart-xray": return serviceAction("restart", managerXrayUnit)
	case "logs": return directLogs(args)
	case "autostart": return directAutostart(args)
	case "settings": return settingsMenu()
	case "admin": return adminMenu()
	case "update": return updateXportCLI()
	case "update-xray": return updateXrayCLI()
	case "update-geodata": return updateGeodataCLI()
	case "backup": return backupMenu()
	case "firewall": return firewallMenu()
	case "repair": return repairInstall()
	case "uninstall": return uninstallXport()
	default: return fmt.Errorf("unknown manager command %q", cmd)
	}
}

func runManagerMenu() error {
	reader := bufio.NewReader(os.Stdin)
	for {
		fmt.Println()
		fmt.Println("╔══════════════════════════════════════╗")
		fmt.Println("║                X-port                ║")
		fmt.Println("╚══════════════════════════════════════╝")
		fmt.Printf(" Panel: %-10s  Xray: %-10s  Version: %s\n", unitState(managerPanelUnit), unitState(managerXrayUnit), buildinfo.Current)
		fmt.Println()
		fmt.Println("  1. 查看状态 / 面板地址")
		fmt.Println("  2. 面板设置")
		fmt.Println("  3. 修改管理员账号 / 密码")
		fmt.Println()
		fmt.Println("  4. 启动 X-port")
		fmt.Println("  5. 停止 X-port")
		fmt.Println("  6. 重启 X-port")
		fmt.Println("  7. 重启 Xray")
		fmt.Println("  8. 查看日志")
		fmt.Println()
		fmt.Println("  9. 更新 X-port")
		fmt.Println(" 10. 更新 Xray")
		fmt.Println(" 11. 更新 GeoData")
		fmt.Println()
		fmt.Println(" 12. 备份与恢复")
		fmt.Println(" 13. X-Panel / 3x-ui 迁移")
		fmt.Println(" 14. 开机自启设置")
		fmt.Println(" 15. 防火墙管理（仅终端）")
		fmt.Println(" 16. 修复 systemd 安装")
		fmt.Println(" 17. 卸载 X-port")
		fmt.Println()
		fmt.Println("  0. 退出")
		choice := promptReader(reader, "请选择", "")
		var err error
		switch choice {
		case "1": err = printManagerStatus(os.Stdout)
		case "2": err = settingsMenu()
		case "3": err = adminMenu()
		case "4": err = serviceAction("start", managerXrayUnit, managerPanelUnit)
		case "5": err = serviceAction("stop", managerPanelUnit, managerXrayUnit)
		case "6": err = serviceAction("restart", managerXrayUnit, managerPanelUnit)
		case "7": err = serviceAction("restart", managerXrayUnit)
		case "8": err = logsMenu()
		case "9": err = updateXportCLI()
		case "10": err = updateXrayCLI()
		case "11": err = updateGeodataCLI()
		case "12": err = backupMenu()
		case "13": err = guardedMigration()
		case "14": err = autostartMenu()
		case "15": err = firewallMenu()
		case "16": err = repairInstall()
		case "17": err = uninstallXport()
		case "0", "q", "quit", "exit": return nil
		default: fmt.Println("无效选项"); continue
		}
		if err != nil { fmt.Println("操作失败:", err) } else { fmt.Println("完成。") }
		pauseReader(reader)
	}
}

func promptReader(r *bufio.Reader, label, def string) string {
	if def != "" { fmt.Printf("%s [%s]: ", label, def) } else { fmt.Printf("%s: ", label) }
	v, _ := r.ReadString('\n'); v = strings.TrimSpace(v)
	if v == "" { return def }
	return v
}
func pauseReader(r *bufio.Reader) { fmt.Print("按 Enter 返回菜单..."); _, _ = r.ReadString('\n') }
func confirmReader(r *bufio.Reader, label string) bool { v := strings.ToLower(promptReader(r, label+" [y/N]", "")); return v == "y" || v == "yes" }
func readSecret(r *bufio.Reader, label string) string {
	fmt.Print(label)
	_ = exec.Command("stty", "-echo").Run()
	v, _ := r.ReadString('\n')
	_ = exec.Command("stty", "echo").Run()
	fmt.Println()
	return strings.TrimSpace(v)
}

func requireRoot() error { if os.Geteuid() != 0 { return errors.New("此操作需要 root，请使用 sudo xport") }; return nil }
func runCommand(name string, args ...string) error { out, err := exec.Command(name, args...).CombinedOutput(); if err != nil { return fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(out))) }; if len(out) > 0 { fmt.Print(string(out)) }; return nil }
func outputCommand(name string, args ...string) string { out, err := exec.Command(name, args...).CombinedOutput(); if err != nil { return "" }; return strings.TrimSpace(string(out)) }
func unitState(unit string) string { s := outputCommand("systemctl", "is-active", unit); if s == "" { return "unknown" }; return s }

func serviceAction(action string, units ...string) error {
	if err := requireRoot(); err != nil { return err }
	args := append([]string{action}, units...)
	return runCommand("systemctl", args...)
}

func printManagerStatus(w *os.File) error {
	fmt.Fprintf(w, "X-port      %s\n", unitState(managerPanelUnit))
	fmt.Fprintf(w, "Xray        %s\n", unitState(managerXrayUnit))
	fmt.Fprintf(w, "X-port ver  %s\n", buildinfo.Current)
	xrayVersion := outputCommand(managerXrayBinary, "version")
	if xrayVersion != "" { if i := strings.IndexByte(xrayVersion, '\n'); i >= 0 { xrayVersion = xrayVersion[:i] }; fmt.Fprintf(w, "Xray ver    %s\n", xrayVersion) }
	if _, err := os.Stat(dbPath(managerDataDir)); err != nil { fmt.Fprintln(w, "Config      未初始化"); return nil }
	st, err := store.Open(dbPath(managerDataDir)); if err != nil { return err }; defer st.Close()
	listen := stringSetting(st, "panel_listen", "127.0.0.1:8080")
	base := normalizeBasePath(stringSetting(st, "panel_base_path", "/"))
	domain := strings.TrimSpace(stringSetting(st, "panel_domain", ""))
	cert := strings.TrimSpace(stringSetting(st, "panel_cert_file", ""))
	scheme := "http"; if cert != "" { scheme = "https" }
	host := domain; if host == "" { host = listen }
	fmt.Fprintf(w, "Panel       %s://%s%s\n", scheme, host, base)
	fmt.Fprintf(w, "Listen      %s\n", listen)
	fmt.Fprintf(w, "Base Path   %s\n", base)
	fmt.Fprintf(w, "TLS         %t\n", cert != "")
	fmt.Fprintf(w, "Xray API    127.0.0.1:%d\n", intSetting(st, "xray_api_port", 10085))
	accounts, err := st.Accounts(); if err != nil { return err }
	enabled := 0; for _, a := range accounts { if a.Enabled { enabled++ } }
	fmt.Fprintf(w, "Accounts    %d (%d enabled)\n", len(accounts), enabled)
	return nil
}

func directLogs(args []string) error {
	serviceName := "xport"; follow := false
	for _, a := range args { if a == "xray" || a == "xport" { serviceName = a }; if a == "-f" || a == "--follow" { follow = true } }
	unit := managerPanelUnit; if serviceName == "xray" { unit = managerXrayUnit }
	if follow { cmd := exec.Command("journalctl", "-u", unit, "-f", "-n", "100", "-o", "short-iso"); cmd.Stdin=os.Stdin;cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;return cmd.Run() }
	text, err := ops.Journal(unit, 200); if err != nil { return err }; fmt.Print(text); return nil
}
func logsMenu() error {
	r := bufio.NewReader(os.Stdin); fmt.Println("1. X-port 日志\n2. Xray 日志\n3. 实时跟踪 X-port\n4. 实时跟踪 Xray\n0. 返回")
	switch promptReader(r,"请选择","") { case "1": return directLogs([]string{"xport"}); case "2": return directLogs([]string{"xray"}); case "3": return directLogs([]string{"xport","--follow"}); case "4": return directLogs([]string{"xray","--follow"}); default: return nil }
}

func directAutostart(args []string) error {
	if len(args)==0 || args[0]=="status" { fmt.Printf("X-port: %s\nXray: %s\n", outputCommand("systemctl","is-enabled",managerPanelUnit), outputCommand("systemctl","is-enabled",managerXrayUnit)); return nil }
	if err:=requireRoot();err!=nil{return err}
	switch args[0] { case "on": return runCommand("systemctl","enable",managerPanelUnit,managerXrayUnit); case "off": return runCommand("systemctl","disable",managerPanelUnit,managerXrayUnit); default: return errors.New("用法: xport autostart [status|on|off]") }
}
func autostartMenu() error { r:=bufio.NewReader(os.Stdin); _=directAutostart([]string{"status"}); fmt.Println("1. 启用开机自启\n2. 关闭开机自启\n0. 返回"); switch promptReader(r,"请选择",""){case "1":return directAutostart([]string{"on"});case "2":return directAutostart([]string{"off"});default:return nil} }

func validManagerBasePath(v string) bool {
	if v == "/" { return true }
	if !strings.HasPrefix(v,"/") || !strings.HasSuffix(v,"/") || strings.ContainsAny(v,"?#\\\t\r\n ") { return false }
	for _,part:=range strings.Split(strings.Trim(v,"/"),"/"){if part==""||part=="."||part==".."{return false}}
	return true
}
func settingsMenu() error {
	if err:=requireRoot();err!=nil{return err}
	st,err:=store.Open(dbPath(managerDataDir));if err!=nil{return err};defer st.Close();r:=bufio.NewReader(os.Stdin)
	oldListen:=stringSetting(st,"panel_listen","127.0.0.1:8080");oldBase:=normalizeBasePath(stringSetting(st,"panel_base_path","/"));oldDomain:=stringSetting(st,"panel_domain","");oldCert:=stringSetting(st,"panel_cert_file","");oldKey:=stringSetting(st,"panel_key_file","")
	fmt.Println("直接回车保留当前值；TLS cert/key 同时留空表示关闭 TLS。")
	listen:=promptReader(r,"Listen",oldListen);base:=normalizeBasePath(promptReader(r,"Base Path",oldBase));domain:=promptReader(r,"Domain",oldDomain);cert:=promptReader(r,"TLS cert path",oldCert);key:=promptReader(r,"TLS key path",oldKey)
	if _,_,err:=net.SplitHostPort(listen);err!=nil{return fmt.Errorf("Listen 必须是 host:port: %w",err)};if !validManagerBasePath(base){return errors.New("Base Path 无效")}
	if (cert=="")!=(key==""){return errors.New("TLS cert/key 必须同时设置或同时清空")};if cert!=""{if !filepath.IsAbs(cert)||!filepath.IsAbs(key){return errors.New("TLS 路径必须是绝对路径")};if _,err:=tls.LoadX509KeyPair(cert,key);err!=nil{return fmt.Errorf("TLS 证书/私钥无法加载: %w",err)}}
	for k,v:=range map[string]string{"panel_listen":listen,"panel_base_path":base,"panel_domain":strings.TrimSpace(domain),"panel_cert_file":strings.TrimSpace(cert),"panel_key_file":strings.TrimSpace(key)}{if err:=st.SetSetting(k,v);err!=nil{return err}}
	fmt.Println("设置已保存。")
	if confirmReader(r,"现在重启 X-port 使设置生效？"){return serviceAction("restart",managerPanelUnit)}
	return nil
}

func adminMenu() error {
	if err:=requireRoot();err!=nil{return err};st,err:=store.Open(dbPath(managerDataDir));if err!=nil{return err};defer st.Close();admins,err:=st.Admins();if err!=nil{return err};if len(admins)==0{return errors.New("未找到管理员")};r:=bufio.NewReader(os.Stdin);old:=admins[0].Username
	user:=promptReader(r,"管理员用户名",old);p1:=readSecret(r,"新密码（至少 12 位；直接回车表示只改用户名）: ");hash:=admins[0].PasswordHash
	if p1!=""{if len(p1)<12{return errors.New("密码至少 12 位")};p2:=readSecret(r,"重复新密码: ");if p1!=p2{return errors.New("两次密码不一致")};h,err:=bcrypt.GenerateFromPassword([]byte(p1),bcrypt.DefaultCost);if err!=nil{return err};hash=string(h)}
	if user==old&&p1==""{fmt.Println("没有修改。");return nil};return st.ApplySettings(nil,old,user,hash)
}

func updateXrayCLI() error { if err:=requireRoot();err!=nil{return err};return xrayUpdate([]string{"--binary", managerXrayBinary, "--config", managerXrayConfig, "--service", managerXrayUnit}) }
func updateGeodataCLI() error {
	if err:=requireRoot();err!=nil{return err};u:=&xray.Updater{BinaryPath:managerXrayBinary,ConfigPath:managerXrayConfig,Service:managerXrayUnit};ctx,c:=context.WithTimeout(context.Background(),3*time.Minute);defer c();info,err:=u.UpdateGeodata(ctx);if err!=nil{return err};fmt.Printf("GeoData: %+v\n",info);return nil
}
func privateReleaseToken() string {
	if v:=strings.TrimSpace(os.Getenv("XPORT_GITHUB_TOKEN"));v!=""{return v}
	b,err:=os.ReadFile(filepath.Join(managerDataDir,"xport.env"));if err!=nil{return ""}
	for _,line:=range strings.Split(string(b),"\n"){line=strings.TrimSpace(line);if strings.HasPrefix(line,"XPORT_GITHUB_TOKEN="){return strings.Trim(strings.TrimSpace(strings.TrimPrefix(line,"XPORT_GITHUB_TOKEN=")),"\"'")}}
	return ""
}
func updateXportCLI() error {
	if err:=requireRoot();err!=nil{return err};if _,err:=os.Stat(managerBinary);err!=nil{return errors.New("未找到已安装的 /usr/local/bin/xport")};u:=&selfupdate.Updater{CurrentVersion:buildinfo.Current,BinaryPath:managerBinary,Repo:"Kerberos255/X-port",Token:privateReleaseToken()};ctx,c:=context.WithTimeout(context.Background(),2*time.Minute);defer c();info,previous,err:=u.Stage(ctx);if err!=nil{return err};if previous==""{fmt.Printf("X-port 已是最新版本：%s\n",info.Current);return nil};if unitState(managerPanelUnit)=="active"{if err:=ops.ScheduleVerifiedBinaryRestart(managerPanelUnit,strings.TrimSuffix(previous,".previous"),previous,time.Second);err!=nil{_ = u.Restore(previous);return err};fmt.Printf("X-port 已更新到 %s，已安排服务验证重启。\n",info.Current);return nil};_ = os.Remove(previous);fmt.Printf("X-port 已更新到 %s。\n",info.Current);return nil
}

func backupDirCLI()string{return filepath.Join(managerDataDir,"backups","manual")}
func backupMenu() error {
	if err:=requireRoot();err!=nil{return err};r:=bufio.NewReader(os.Stdin);fmt.Println("1. 创建备份\n2. 列出备份\n3. 恢复备份\n0. 返回");choice:=promptReader(r,"请选择","")
	st,err:=store.Open(dbPath(managerDataDir));if err!=nil{return err};defer st.Close();dir:=backupDirCLI()
	switch choice{
	case "1":snap,err:=st.Snapshot();if err!=nil{return err};info,err:=backup.Create(dir,snap);if err==nil{fmt.Println("已创建:",info.Name)};return err
	case "2":items,err:=backup.List(dir);if err!=nil{return err};for i,v:=range items{fmt.Printf("%2d. %-40s %8d bytes\n",i+1,v.Name,v.Size)};return nil
	case "3":items,err:=backup.List(dir);if err!=nil{return err};if len(items)==0{return errors.New("没有可恢复的备份")};for i,v:=range items{fmt.Printf("%2d. %s\n",i+1,v.Name)};n,err:=strconv.Atoi(promptReader(r,"备份编号",""));if err!=nil||n<1||n>len(items){return errors.New("无效编号")};if !confirmReader(r,"恢复会覆盖当前账号和设置，继续？"){return nil};return restoreSnapshotCLI(st,dir,items[n-1].Name)
	default:return nil}
}
func restoreSnapshotCLI(st *store.Store,dir,name string)error{
	old,err:=st.Snapshot();if err!=nil{return err};if _,err:=backup.Create(dir,old);err!=nil{return fmt.Errorf("恢复前备份失败: %w",err)};target,err:=backup.Load(dir,name);if err!=nil{return err}
	managerFor:=func(settings map[string]string)*xray.Manager{api:=10085;if v:=settings["xray_api_port"];v!=""{if n,e:=strconv.Atoi(v);e==nil{api=n}};return &xray.Manager{BinaryPath:managerXrayBinary,ConfigPath:managerXrayConfig,Service:managerXrayUnit,APIPort:api,BaseConfigJSON:settings["xray_global_config"]}}
	targetMgr:=managerFor(target.Settings);if err:=targetMgr.Apply(target.Accounts);err!=nil{return fmt.Errorf("备份 Xray 配置验证失败: %w",err)}
	if err:=st.ReplaceSnapshot(target);err!=nil{_ = managerFor(old.Settings).Apply(old.Accounts);return fmt.Errorf("恢复数据库失败，Xray 已回滚: %w",err)}
	fmt.Println("备份已恢复。面板设置可能已变化。")
	return serviceAction("restart",managerPanelUnit)
}

func guardedMigration() error {
	if err:=requireRoot();err!=nil{return err};candidates:=[]string{managerMigration,"./scripts/migrate-xpanel.sh"};for _,p:=range candidates{if st,err:=os.Stat(p);err==nil&&!st.IsDir(){cmd:=exec.Command("bash",p);cmd.Stdin=os.Stdin;cmd.Stdout=os.Stdout;cmd.Stderr=os.Stderr;return cmd.Run()}};return fmt.Errorf("未找到迁移脚本；请确认 %s 已安装",managerMigration)
}

func firewallKind()string{
	if _,err:=exec.LookPath("ufw");err==nil&&strings.Contains(strings.ToLower(outputCommand("ufw","status")),"status: active"){return "ufw"}
	if _,err:=exec.LookPath("firewall-cmd");err==nil&&outputCommand("systemctl","is-active","firewalld.service")=="active"{return "firewalld"}
	if _,err:=exec.LookPath("ufw");err==nil{return "ufw"}
	if _,err:=exec.LookPath("nft");err==nil{return "nftables-readonly"}
	if _,err:=exec.LookPath("iptables");err==nil{return "iptables-readonly"}
	return "none"
}
func firewallMenu()error{
	if err:=requireRoot();err!=nil{return err};r:=bufio.NewReader(os.Stdin);kind:=firewallKind();fmt.Println("检测到防火墙:",kind);fmt.Println("1. 查看规则\n2. 放行端口\n3. 撤销端口\n0. 返回");choice:=promptReader(r,"请选择","");if choice=="0"{return nil};if choice=="1"{return firewallList(kind)};if choice!="2"&&choice!="3"{return errors.New("无效选项")};if kind!="ufw"&&kind!="firewalld"{return errors.New("当前规则体系只提供只读查看；X-port 不猜测自定义 nftables/iptables 规则")};port,err:=strconv.Atoi(promptReader(r,"端口",""));if err!=nil||port<1||port>65535{return errors.New("无效端口")};proto:=strings.ToLower(promptReader(r,"协议 tcp / udp / both","both"));if proto!="tcp"&&proto!="udp"&&proto!="both"{return errors.New("协议只能是 tcp、udp 或 both")};remove:=choice=="3";verb:="放行";if remove{verb="撤销"};if !confirmReader(r,fmt.Sprintf("确认%s %d/%s？",verb,port,proto)){return nil};return firewallChange(kind,remove,port,proto)
}
func firewallList(kind string)error{switch kind{case "ufw":return runCommand("ufw","status","numbered");case "firewalld":return runCommand("firewall-cmd","--list-all");case "nftables-readonly":return runCommand("nft","list","ruleset");case "iptables-readonly":return runCommand("iptables","-S");default:return errors.New("未检测到防火墙工具")}}
func firewallChange(kind string,remove bool,port int,proto string)error{protos:=[]string{proto};if proto=="both"{protos=[]string{"tcp","udp"}};for _,p:=range protos{spec:=fmt.Sprintf("%d/%s",port,p);switch kind{case "ufw":args:=[]string{"allow",spec};if remove{args=[]string{"delete","allow",spec}};if err:=runCommand("ufw",args...);err!=nil{return err};case "firewalld":flag:="--add-port="+spec;if remove{flag="--remove-port="+spec};if err:=runCommand("firewall-cmd",flag);err!=nil{return err};if err:=runCommand("firewall-cmd","--permanent",flag);err!=nil{return err}}};return nil}

func repairInstall()error{
	if err:=requireRoot();err!=nil{return err};if _,err:=os.Stat(managerBinary);err!=nil{return errors.New("/usr/local/bin/xport 不存在；首次安装请在源码目录运行 scripts/install.sh")};if _,err:=os.Stat(managerXrayBinary);err!=nil{return errors.New("Xray binary 不存在；首次安装请在源码目录运行 scripts/install.sh")}
	panel:=`[Unit]
Description=X-port Control Panel
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
EnvironmentFile=-/etc/x-port/xport.env
ExecStart=/usr/local/bin/xport serve --data /etc/x-port --xray-binary /usr/local/x-port/bin/xray --xray-config /etc/x-port/xray/config.json --xray-service xport-xray.service
Restart=on-failure
RestartSec=3
UMask=0027
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=read-only
[Install]
WantedBy=multi-user.target
`
	xr:=`[Unit]
Description=X-port Xray Core
After=network-online.target
Wants=network-online.target
[Service]
Type=simple
Environment=XRAY_LOCATION_ASSET=/usr/local/x-port/bin
ExecStart=/usr/local/x-port/bin/xray run -config /etc/x-port/xray/config.json
Restart=on-failure
RestartSec=3
LimitNOFILE=1048576
NoNewPrivileges=true
PrivateTmp=true
ProtectHome=read-only
[Install]
WantedBy=multi-user.target
`
	if err:=os.WriteFile("/etc/systemd/system/xport.service",[]byte(panel),0644);err!=nil{return err};if err:=os.WriteFile("/etc/systemd/system/xport-xray.service",[]byte(xr),0644);err!=nil{return err};if err:=runCommand("systemctl","daemon-reload");err!=nil{return err};return runCommand("systemctl","enable",managerPanelUnit,managerXrayUnit)
}

func uninstallXport()error{
	if err:=requireRoot();err!=nil{return err};r:=bufio.NewReader(os.Stdin);fmt.Println("将停止并移除 X-port/Xray systemd unit、/usr/local/bin/xport 和 /usr/local/x-port。默认保留 /etc/x-port 数据和备份。")
	if promptReader(r,"请输入 UNINSTALL 确认","")!="UNINSTALL"{fmt.Println("已取消。");return nil};_ = runCommand("systemctl","disable","--now",managerPanelUnit,managerXrayUnit);_ = os.Remove("/etc/systemd/system/xport.service");_ = os.Remove("/etc/systemd/system/xport-xray.service");_ = os.RemoveAll("/usr/local/x-port");_ = os.Remove(managerBinary);_ = os.RemoveAll("/usr/local/lib/xport");_ = runCommand("systemctl","daemon-reload");if confirmReader(r,"同时删除 /etc/x-port 的数据库、配置和备份？"){return os.RemoveAll(managerDataDir)};fmt.Println("/etc/x-port 已保留。");return nil
}
