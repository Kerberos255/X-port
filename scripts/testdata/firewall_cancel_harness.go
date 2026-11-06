package main

import (
 "context"
 "fmt"
 "os"
 "time"

 "github.com/Kerberos255/X-port/internal/ops"
)

func main() {
 if len(os.Args)!=2 {fmt.Fprintln(os.Stderr,"usage: cancel-harness UNIT");os.Exit(2)}
 ctx,cancel:=context.WithTimeout(context.Background(),10*time.Second)
 defer cancel()
 if err:=ops.CancelFirewallRestore(ctx,os.Args[1]);err!=nil{fmt.Fprintln(os.Stderr,err);os.Exit(1)}
 fmt.Println("X-port timer cancellation API stopped a real systemd timer")
}
