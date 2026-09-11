package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Kerberos255/X-port/internal/migrate"
	"github.com/Kerberos255/X-port/internal/server"
	"github.com/Kerberos255/X-port/internal/service"
	"github.com/Kerberos255/X-port/internal/store"
	"github.com/Kerberos255/X-port/internal/xray"
	webui "github.com/Kerberos255/X-port/web"
)

const version = "0.1.0-alpha"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "serve":
		err = serve(os.Args[2:])
	case "init":
		err = initDB(os.Args[2:])
	case "migrate":
		err = migrateDB(os.Args[2:])
	case "render":
		err = render(os.Args[2:])
	case "xray-check":
		err = xrayCheck(os.Args[2:])
	case "xray-update":
		err = xrayUpdate(os.Args[2:])
	case "version":
		fmt.Println(version)
		return
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		log.Fatal(err)
	}
}
func usage() {
	fmt.Fprintln(os.Stderr, `X-port
  xport serve       --data /etc/x-port [--listen 127.0.0.1:8080]
  xport init        --data /etc/x-port --admin-user admin --listen 127.0.0.1:8080
  xport migrate     --data /etc/x-port --from /etc/x-ui/x-ui.db [--apply]
  xport render      --data /etc/x-port --output /etc/x-port/xray/config.json
  xport xray-check  --binary /usr/local/x-port/bin/xray
  xport xray-update --binary /usr/local/x-port/bin/xray [--service xport-xray.service]`)
}
func dbPath(data string) string { return filepath.Join(data, "xport.db") }
func openData(data string) (*store.Store, error) {
	if err := os.MkdirAll(data, 0700); err != nil {
		return nil, err
	}
	return store.Open(dbPath(data))
}

func serve(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	data := fs.String("data", "/etc/x-port", "data directory")
	listen := fs.String("listen", "", "listen address override; defaults to stored panel setting")
	xrayBin := fs.String("xray-binary", "/usr/local/x-port/bin/xray", "Xray binary")
	xrayConfig := fs.String("xray-config", "/etc/x-port/xray/config.json", "Xray config")
	xrayService := fs.String("xray-service", "xport-xray.service", "systemd service")
	apiPort := fs.Int("xray-api-port", 10085, "local Xray API port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	st, err := openData(*data)
	if err != nil {
		return err
	}
	defer st.Close()
	listenAddr := strings.TrimSpace(*listen)
	if listenAddr == "" {
		if stored, ok, err := st.Setting("panel_listen"); err != nil {
			return err
		} else if ok && strings.TrimSpace(stored) != "" {
			listenAddr = strings.TrimSpace(stored)
		}
	}
	if listenAddr == "" {
		listenAddr = "127.0.0.1:8080"
	}
	static, err := webui.FS()
	if err != nil {
		return err
	}
	manager := &xray.Manager{BinaryPath: *xrayBin, ConfigPath: *xrayConfig, Service: *xrayService, APIPort: *apiPort}
	accounts := service.NewAccounts(st, manager)
	updater := &xray.Updater{BinaryPath: *xrayBin, ConfigPath: *xrayConfig, Service: *xrayService}
	srv := &http.Server{Addr: listenAddr, Handler: server.New(st, accounts, updater, static).Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 2 * time.Minute, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20}
	log.Printf("X-port %s listening on %s", version, listenAddr)
	err = srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
func initDB(args []string) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	data := fs.String("data", "/etc/x-port", "data directory")
	user := fs.String("admin-user", "admin", "admin username")
	listen := fs.String("listen", "127.0.0.1:8080", "initial panel listen address")
	if err := fs.Parse(args); err != nil {
		return err
	}
	reader := bufio.NewReader(os.Stdin)
	password, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return fmt.Errorf("read password from stdin: %w", err)
	}
	password = strings.TrimRight(password, "\r\n")
	if len(password) < 12 {
		return errors.New("admin password must be at least 12 characters")
	}
	st, err := openData(*data)
	if err != nil {
		return err
	}
	defer st.Close()
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return err
	}
	if err := st.SetAdmin(*user, string(hash)); err != nil {
		return err
	}
	return st.SetSetting("panel_listen", strings.TrimSpace(*listen))
}
func migrateDB(args []string) error {
	fs := flag.NewFlagSet("migrate", flag.ContinueOnError)
	data := fs.String("data", "/etc/x-port", "data directory")
	from := fs.String("from", "/etc/x-ui/x-ui.db", "source x-ui database")
	apply := fs.Bool("apply", false, "replace X-port accounts and compatible panel settings")
	if err := fs.Parse(args); err != nil {
		return err
	}
	result, err := migrate.ReadXUI(*from)
	if err != nil {
		return err
	}
	adminNames := make([]string, 0, len(result.Admins))
	for _, a := range result.Admins {
		adminNames = append(adminNames, a.Username)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{"source": *from, "compatible": len(result.Accounts), "adminUsernames": adminNames, "panelListen": result.PanelListen, "warnings": result.Warnings, "skipped": result.Skipped, "apply": *apply})
	if !*apply {
		return nil
	}
	if len(result.Skipped) > 0 {
		return fmt.Errorf("refusing apply: %d source inbound(s) require attention", len(result.Skipped))
	}
	st, err := openData(*data)
	if err != nil {
		return err
	}
	defer st.Close()
	if err := st.ReplaceAccounts(result.Accounts); err != nil {
		return err
	}
	if len(result.Admins) > 0 {
		if err := st.ReplaceAdmins(result.Admins); err != nil {
			return err
		}
	}
	if result.PanelListen != "" {
		if err := st.SetSetting("panel_listen", result.PanelListen); err != nil {
			return err
		}
	}
	return nil
}
func render(args []string) error {
	fs := flag.NewFlagSet("render", flag.ContinueOnError)
	data := fs.String("data", "/etc/x-port", "data directory")
	output := fs.String("output", "", "output config path")
	apiPort := fs.Int("api-port", 10085, "local Xray API port")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *output == "" {
		return errors.New("--output is required")
	}
	st, err := openData(*data)
	if err != nil {
		return err
	}
	defer st.Close()
	accounts, err := st.Accounts()
	if err != nil {
		return err
	}
	cfg, err := xray.BuildConfig(accounts, *apiPort)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(*output), 0750); err != nil {
		return err
	}
	tmp := *output + ".tmp"
	if err := os.WriteFile(tmp, cfg, 0640); err != nil {
		return err
	}
	return os.Rename(tmp, *output)
}
func xrayFlags(name string, args []string) (*xray.Updater, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	bin := fs.String("binary", "/usr/local/x-port/bin/xray", "Xray binary")
	config := fs.String("config", "/etc/x-port/xray/config.json", "Xray config")
	serviceName := fs.String("service", "", "systemd service to restart; empty during initial install")
	if err := fs.Parse(args); err != nil {
		return nil, err
	}
	return &xray.Updater{BinaryPath: *bin, ConfigPath: *config, Service: *serviceName}, nil
}
func xrayCheck(args []string) error {
	u, err := xrayFlags("xray-check", args)
	if err != nil {
		return err
	}
	ctx, c := context.WithTimeout(context.Background(), 15*time.Second)
	defer c()
	info, _, err := u.Check(ctx)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(info, "", "  ")
	fmt.Println(string(b))
	return nil
}
func xrayUpdate(args []string) error {
	u, err := xrayFlags("xray-update", args)
	if err != nil {
		return err
	}
	ctx, c := context.WithTimeout(context.Background(), 3*time.Minute)
	defer c()
	info, err := u.Update(ctx)
	if err != nil {
		return err
	}
	b, _ := json.MarshalIndent(info, "", "  ")
	fmt.Println(string(b))
	return nil
}
