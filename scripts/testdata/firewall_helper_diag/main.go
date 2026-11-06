package main

import (
 "bytes"
 "context"
 "fmt"
 "os"
 "os/exec"
 "strings"
 "time"
)
func main(){
 for _,mode:=range []string{"default","empty-stdin-pipe","inherit-stdin","inherit-stdout-stderr","no-pipe"}{
  fmt.Printf("[go-helper-matrix] case=%s\n",mode)
  ctx,cancel:=context.WithTimeout(context.Background(),16*time.Second)
  args:=[]string{"--quiet","--wait","--collect","--pipe","--unit=xport-go-diag-"+mode,"--property=Type=exec","--property=NoNewPrivileges=yes","/usr/local/bin/xport","firewall-inspect"}
  if mode=="no-pipe"{args=append(args[:3],args[4:]...)}
  cmd:=exec.CommandContext(ctx,"systemd-run",args...)
  switch mode{
  case "empty-stdin-pipe":
   cmd.Stdin=bytes.NewReader(nil)
  case "inherit-stdin":
   cmd.Stdin=os.Stdin
  case "inherit-stdout-stderr":
   cmd.Stdin=bytes.NewReader(nil)
   cmd.Stdout=os.Stdout
   cmd.Stderr=os.Stderr
  }
  if mode=="inherit-stdout-stderr"{
   err:=cmd.Run()
   fmt.Printf("[go-helper-matrix] %s exit=%v\n",mode,err)
  }else{
   out,err:=cmd.CombinedOutput()
   fmt.Printf("[go-helper-matrix] %s exit=%v output=%s\n",mode,err,strings.TrimSpace(string(out)))
  }
  cancel()
 }
}
