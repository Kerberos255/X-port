//go:build !linux

package system

import (
	"os"
	"runtime"
)

func Snapshot() Info { h, _ := os.Hostname(); return Info{Hostname: h, OS: runtime.GOOS} }
