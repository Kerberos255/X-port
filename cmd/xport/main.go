package main

import (
	"bufio"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Kerberos255/X-port/internal/buildinfo"
	"github.com/Kerberos255/X-port/internal/migrate"
	"github.com/Kerberos255/X-port/internal/server"
	"github.com/Kerberos255/X-port/internal/service"
	"github.com/Kerberos255/X-port/internal/store"
	"github.com/Kerberos255/X-port/internal/xray"
	webui "github.com/Kerberos255/X-port/web"
)

const version = buildinfo.Current

func main() {
	if len(os.Args)<2{usage();os.Exit(2)}
	var err error
	switch os.Args[1]{case "serve":err=serve(os.Args[2:]);case "init":err=initDB(os.Args[2:]);case "migrate":err=migrateDB(os.Args[2:]);case "render":err=render(os.Args[2:]);case "xray-check":err=xrayCheck(os.Args[2:]);case "xray-update":err=xrayUpdate(os.Args[2:]);case "version":fmt.Println(version);return;default:usage();os.Exit(2)}
	if err!=nil{log.Fatal(err)}
}
func usage(){fmt.Fprintln(os.Stderr,`X-port
  xport serve       --data /etc/x-port [--listen 127.0.0.1:8080]
  xport init        --data /etc/x-port --admin-user admin --listen 127.0.0.1:8080
  xport migrate     --data /etc/x-port --from /etc/x-ui/x-ui.db [--source-config /usr/local/x-ui/bin/config.json] [--apply]
  xport render      --data /etc/x-port --output /etc/x-port/xray/config.json
  xport xray-check  --binary /usr/local/x-port/bin/xray
  xport xray-update --binary /usr/local/x-port/bin/xray [--service xport-xray.service]`)}
func dbPath(data string)string{return filepath.Join(data,"xport.db")}
func openData(data string)(*store.Store,error){if err:=os.MkdirAll(data,0700);err!=nil{return nil,err};return store.Open(dbPath(data))}

func serve(args []string)error{
	fs:=flag.NewFlagSet("serve",flag.ContinueOnError);data:=fs.String("data","/etc/x-port","data directory");listen:=fs.String("listen","","listen address override; defaults to stored panel setting");xrayBin:=fs.String("xray-binary","/usr/local/x-port/bin/xray","Xray binary");xrayConfig:=fs.String("xray-config","/etc/x-port/xray/config.json","Xray config");xrayService:=fs.String("xray-service","xport-xray.service","systemd service");apiPortFlag:=fs.Int("xray-api-port",0,"local Xray API port override");if err:=fs.Parse(args);err!=nil{return err}
	st,err:=openData(*data);if err!=nil{return err};defer st.Close()
	listenAddr:=strings.TrimSpace(*listen);if listenAddr==""{listenAddr=stringSetting(st,"panel_listen","")};if listenAddr==""{listenAddr="127.0.0.1:8080"}
	apiPort:=*apiPortFlag;if apiPort==0{apiPort=intSetting(st,"xray_api_port",10085)};if apiPort<1||apiPort>65535{return fmt.Errorf("invalid Xray API port %d",apiPort)}
	baseConfig:=stringSetting(st,"xray_global_config","")
	certFile:=strings.TrimSpace(stringSetting(st,"panel_cert_file",""));keyFile:=strings.TrimSpace(stringSetting(st,"panel_key_file",""));if (certFile=="")!=(keyFile==""){return errors.New("panel_cert_file and panel_key_file must be configured together")}
	basePath:=normalizeBasePath(stringSetting(st,"panel_base_path","/"))
	static,err:=webui.FS();if err!=nil{return err};manager:=&xray.Manager{BinaryPath:*xrayBin,ConfigPath:*xrayConfig,Service:*xrayService,APIPort:apiPort,BaseConfigJSON:baseConfig};accounts:=service.NewAccounts(st,manager);updater:=&xray.Updater{BinaryPath:*xrayBin,ConfigPath:*xrayConfig,Service:*xrayService};panel:=server.New(st,accounts,updater,static).WithRuntime(server.RuntimeOptions{DataDir:*data,XrayService:*xrayService,PanelService:"xport.service",Manager:manager})
	ctx,cancel:=context.WithCancel(context.Background());defer cancel();stats:=&xray.StatsReader{BinaryPath:*xrayBin,Server:fmt.Sprintf("127.0.0.1:%d",apiPort),Timeout:4*time.Second};go runtimeLoop(ctx,accounts,stats)
	srv:=&http.Server{Addr:listenAddr,Handler:panel.Handler(),ReadHeaderTimeout:5*time.Second,ReadTimeout:20*time.Second,WriteTimeout:2*time.Minute,IdleTimeout:60*time.Second,MaxHeaderBytes:1<<20}
	scheme:="http";if certFile!=""{scheme="https"};log.Printf("X-port %s listening on %s://%s%s",version,scheme,listenAddr,basePath)
	if certFile!=""{err=srv.ListenAndServeTLS(certFile,keyFile)}else{err=srv.ListenAndServe()};if errors.Is(err,http.ErrServerClosed){return nil};return err
}
func runtimeLoop(ctx context.Context,accounts *service.Accounts,stats *xray.StatsReader){ticker:=time.NewTicker(5*time.Second);defer ticker.Stop();lastLogged:=time.Time{};tick:=func(){now:=time.Now();values,err:=stats.ReadAndReset(ctx);if err!=nil{_ = accounts.ApplyTraffic(nil,now);if time.Since(lastLogged)>=time.Minute{log.Printf("Xray stats unavailable: %v",err);lastLogged=time.Now()};return};if err:=accounts.ApplyTraffic(values,now);err!=nil&&time.Since(lastLogged)>=time.Minute{log.Printf("account runtime policy failed: %v",err);lastLogged=time.Now()}};tick();for{select{case<-ctx.Done():return;case<-ticker.C:tick()}}}

func initDB(args []string)error{fs:=flag.NewFlagSet("init",flag.ContinueOnError);data:=fs.String("data","/etc/x-port","data directory");user:=fs.String("admin-user","admin","admin username");listen:=fs.String("listen","127.0.0.1:8080","initial panel listen address");if err:=fs.Parse(args);err!=nil{return err};reader:=bufio.NewReader(os.Stdin);password,err:=reader.ReadString('\n');if err!=nil&&!errors.Is(err,io.EOF){return fmt.Errorf("read password from stdin: %w",err)};password=strings.TrimRight(password,"\r\n");if len(password)<12{return errors.New("admin password must be at least 12 characters")};st,err:=openData(*data);if err!=nil{return err};defer st.Close();hash,err:=bcrypt.GenerateFromPassword([]byte(password),bcrypt.DefaultCost);if err!=nil{return err};if err:=st.SetAdmin(*user,string(hash));err!=nil{return err};if err:=st.SetSetting("panel_listen",strings.TrimSpace(*listen));err!=nil{return err};_ = st.SetSetting("panel_base_path","/");_ = st.SetSetting("panel_cert_file","");_ = st.SetSetting("panel_key_file","");_ = st.SetSetting("panel_domain","");_ = st.SetSetting("xray_api_port","10085");_ = st.SetSetting("port_min","20000");_ = st.SetSetting("port_max","60000");return nil}

func migrateDB(args []string)error{
	fs:=flag.NewFlagSet("migrate",flag.ContinueOnError);data:=fs.String("data","/etc/x-port","data directory");from:=fs.String("from","/etc/x-ui/x-ui.db","source x-ui database");sourceConfig:=fs.String("source-config","","source generated Xray config to preserve global sections");apply:=fs.Bool("apply",false,"replace X-port accounts and compatible panel settings");if err:=fs.Parse(args);err!=nil{return err}
	result,err:=migrate.ReadXUI(*from);if err!=nil{return err};globalConfig:="";if strings.TrimSpace(*sourceConfig)!=""{globalConfig,err=migrate.ReadGlobalXrayConfig(*sourceConfig);if err!=nil{return err}}
	adminNames:=make([]string,0,len(result.Admins));for _,a:=range result.Admins{adminNames=append(adminNames,a.Username)};enc:=json.NewEncoder(os.Stdout);enc.SetIndent("","  ");_ = enc.Encode(map[string]any{"source":*from,"compatible":len(result.Accounts),"adminUsernames":adminNames,"panelListen":result.PanelListen,"panelBasePath":result.PanelBasePath,"panelDomain":result.PanelDomain,"panelTLS":result.PanelCertFile!=""&&result.PanelKeyFile!="","globalXrayConfig":globalConfig!="","warnings":result.Warnings,"skipped":result.Skipped,"apply":*apply})
	if !*apply{return nil};if len(result.Skipped)>0{return fmt.Errorf("refusing apply: %d source inbound(s) require attention",len(result.Skipped))};if result.PanelCertFile!=""&&result.PanelKeyFile!=""{if _,err:=tls.LoadX509KeyPair(result.PanelCertFile,result.PanelKeyFile);err!=nil{return fmt.Errorf("refusing apply: migrated panel TLS files cannot be loaded: %w",err)}}
	st,err:=openData(*data);if err!=nil{return err};defer st.Close();if err:=st.ReplaceAccounts(result.Accounts);err!=nil{return err};if len(result.Admins)>0{if err:=st.ReplaceAdmins(result.Admins);err!=nil{return err}}
	settings:=map[string]string{};if result.PanelListen!=""{settings["panel_listen"]=result.PanelListen};if result.PanelBasePath!=""{settings["panel_base_path"]=normalizeBasePath(result.PanelBasePath)};if result.PanelCertFile!=""&&result.PanelKeyFile!=""{settings["panel_cert_file"]=result.PanelCertFile;settings["panel_key_file"]=result.PanelKeyFile};if result.PanelDomain!=""{settings["panel_domain"]=result.PanelDomain};if globalConfig!=""{settings["xray_global_config"]=globalConfig};for k,v:=range settings{if err:=st.SetSetting(k,v);err!=nil{return err}};return nil
}

func render(args []string)error{fs:=flag.NewFlagSet("render",flag.ContinueOnError);data:=fs.String("data","/etc/x-port","data directory");output:=fs.String("output","","output config path");apiPortFlag:=fs.Int("api-port",0,"local Xray API port override");if err:=fs.Parse(args);err!=nil{return err};if *output==""{return errors.New("--output is required")};st,err:=openData(*data);if err!=nil{return err};defer st.Close();accounts,err:=st.Accounts();if err!=nil{return err};apiPort:=*apiPortFlag;if apiPort==0{apiPort=intSetting(st,"xray_api_port",10085)};cfg,err:=xray.BuildConfigWithBase(accounts,apiPort,stringSetting(st,"xray_global_config",""));if err!=nil{return err};if err:=os.MkdirAll(filepath.Dir(*output),0750);err!=nil{return err};tmp:=*output+".tmp";if err:=os.WriteFile(tmp,cfg,0640);err!=nil{return err};return os.Rename(tmp,*output)}
func intSetting(st *store.Store,key string,fallback int)int{v,ok,err:=st.Setting(key);if err!=nil||!ok{return fallback};n,err:=strconv.Atoi(strings.TrimSpace(v));if err!=nil{return fallback};return n}
func stringSetting(st *store.Store,key,fallback string)string{v,ok,err:=st.Setting(key);if err!=nil||!ok{return fallback};return v}
func normalizeBasePath(v string)string{v=strings.TrimSpace(v);if v==""||v=="/"{return "/"};return "/"+strings.Trim(v,"/")+"/"}
func xrayFlags(name string,args []string)(*xray.Updater,error){fs:=flag.NewFlagSet(name,flag.ContinueOnError);bin:=fs.String("binary","/usr/local/x-port/bin/xray","Xray binary");config:=fs.String("config","/etc/x-port/xray/config.json","Xray config");serviceName:=fs.String("service","","systemd service to restart; empty during initial install");if err:=fs.Parse(args);err!=nil{return nil,err};return &xray.Updater{BinaryPath:*bin,ConfigPath:*config,Service:*serviceName},nil}
func xrayCheck(args []string)error{u,err:=xrayFlags("xray-check",args);if err!=nil{return err};ctx,c:=context.WithTimeout(context.Background(),15*time.Second);defer c();info,_,err:=u.Check(ctx);if err!=nil{return err};b,_:=json.MarshalIndent(info,"","  ");fmt.Println(string(b));return nil}
func xrayUpdate(args []string)error{u,err:=xrayFlags("xray-update",args);if err!=nil{return err};ctx,c:=context.WithTimeout(context.Background(),3*time.Minute);defer c();info,err:=u.Update(ctx);if err!=nil{return err};b,_:=json.MarshalIndent(info,"","  ");fmt.Println(string(b));return nil}
