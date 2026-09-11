package migrate

import (
	"encoding/json"
	"fmt"
	"os"
)

// ReadGlobalXrayConfig extracts global sections from an existing generated Xray
// config without retaining its inbounds or panel API/stats sections. Account
// credentials remain sourced from the migrated SQLite data only.
func ReadGlobalXrayConfig(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil { return "", err }
	if len(b) > 16<<20 { return "", fmt.Errorf("Xray config is unexpectedly large") }
	var cfg map[string]any
	if err := json.Unmarshal(b,&cfg);err!=nil{return "",fmt.Errorf("parse Xray config: %w",err)}
	delete(cfg,"inbounds")
	delete(cfg,"api")
	delete(cfg,"stats")
	out,err:=json.Marshal(cfg);if err!=nil{return "",err};return string(out),nil
}
