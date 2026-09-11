package xray

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/Kerberos255/X-port/internal/model"
)

type Applier interface{ Apply([]model.Account) error }

type Manager struct {
	BinaryPath     string
	ConfigPath     string
	Service        string
	APIPort        int
	BaseConfigJSON string
}

func (m *Manager) Apply(accounts []model.Account) error {
	cfg, err := BuildConfigWithBase(accounts, m.APIPort, m.BaseConfigJSON)
	if err != nil { return err }
	if m.BinaryPath==""||m.ConfigPath==""{return errors.New("Xray runtime paths are not configured")}
	if err:=os.MkdirAll(filepath.Dir(m.ConfigPath),0750);err!=nil{return err}
	tmp,err:=os.CreateTemp(filepath.Dir(m.ConfigPath),".xport-config-*.json");if err!=nil{return err};tmpPath:=tmp.Name();defer os.Remove(tmpPath)
	if err:=tmp.Chmod(0640);err!=nil{tmp.Close();return err};if _,err:=tmp.Write(cfg);err!=nil{tmp.Close();return err};if err:=tmp.Close();err!=nil{return err}
	if out,err:=run(8*time.Second,m.BinaryPath,"run","-test","-config",tmpPath);err!=nil{return fmt.Errorf("Xray config validation failed: %v: %s",err,out)}
	old,oldErr:=os.ReadFile(m.ConfigPath);if err:=os.Rename(tmpPath,m.ConfigPath);err!=nil{return err};if m.Service==""{return nil}
	if err:=restartAndVerify(m.Service);err==nil{return nil}
	if oldErr==nil{_ = os.WriteFile(m.ConfigPath,old,0640);_ = restartAndVerify(m.Service)}
	return errors.New("Xray restart failed; previous configuration was restored")
}

func restartAndVerify(service string) error { if _,err:=run(10*time.Second,"systemctl","restart",service);err!=nil{return err};deadline:=time.Now().Add(6*time.Second);for time.Now().Before(deadline){if _,err:=run(2*time.Second,"systemctl","is-active","--quiet",service);err==nil{return nil};time.Sleep(250*time.Millisecond)};return errors.New("service did not become active") }
func run(timeout time.Duration,name string,args ...string)(string,error){ctx,cancel:=context.WithTimeout(context.Background(),timeout);defer cancel();b,err:=exec.CommandContext(ctx,name,args...).CombinedOutput();if ctx.Err()!=nil{return string(b),ctx.Err()};return string(b),err}
