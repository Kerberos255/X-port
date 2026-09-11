package xray

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

type Traffic struct {
	Up   int64 `json:"up"`
	Down int64 `json:"down"`
}

type StatsReader struct {
	BinaryPath string
	Server     string
	Timeout    time.Duration
}

func (r *StatsReader) ReadAndReset(ctx context.Context) (map[string]Traffic, error) {
	if strings.TrimSpace(r.BinaryPath) == "" { return nil, fmt.Errorf("Xray binary path is empty") }
	server := strings.TrimSpace(r.Server); if server == "" { server = "127.0.0.1:10085" }
	timeout := r.Timeout; if timeout <= 0 { timeout = 5*time.Second }
	ctx, cancel := context.WithTimeout(ctx, timeout); defer cancel()
	out, err := exec.CommandContext(ctx, r.BinaryPath, "api", "statsquery", "--server="+server, "--reset=true").CombinedOutput()
	if ctx.Err()!=nil { return nil, ctx.Err() }
	if err!=nil { return nil, fmt.Errorf("query Xray stats: %w: %s",err,strings.TrimSpace(string(out))) }
	return ParseInboundStats(out)
}

func ParseInboundStats(data []byte) (map[string]Traffic,error) {
	var reply struct{ Stat []struct{Name string `json:"name"`; Value json.RawMessage `json:"value"`} `json:"stat"` }
	if err:=json.Unmarshal(data,&reply);err!=nil{return nil,fmt.Errorf("decode Xray stats: %w",err)}
	out:=map[string]Traffic{}
	for _,item:=range reply.Stat{
		parts:=strings.Split(item.Name,">>>");if len(parts)!=4||parts[0]!="inbound"||parts[2]!="traffic"{continue}
		value,err:=parseStatValue(item.Value);if err!=nil{return nil,fmt.Errorf("decode %s: %w",item.Name,err)}
		t:=out[parts[1]];switch parts[3]{case "uplink":t.Up+=value;case "downlink":t.Down+=value;default:continue};out[parts[1]]=t
	}
	return out,nil
}
func parseStatValue(raw json.RawMessage)(int64,error){var s string;if err:=json.Unmarshal(raw,&s);err==nil{if s==""{return 0,nil};return strconv.ParseInt(s,10,64)};var n int64;if err:=json.Unmarshal(raw,&n);err!=nil{return 0,err};return n,nil}
