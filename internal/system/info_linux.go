//go:build linux

package system

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

type cpuTimes struct{ total, idle uint64 }

func Snapshot() Info {
	host, _ := os.Hostname()
	a := readCPU()
	time.Sleep(120 * time.Millisecond)
	b := readCPU()
	mu, mt := readMemory()
	du, dt := readDisk()
	rx, tx := readNetwork()
	return Info{
		Hostname:      host,
		OS:            "Linux",
		UptimeSeconds: readUptime(),
		CPUPercent:    cpuPercent(a, b),
		Load1:         readLoad1(),
		MemoryUsed:    mu,
		MemoryTotal:   mt,
		DiskUsed:      du,
		DiskTotal:     dt,
		NetworkRX:     rx,
		NetworkTX:     tx,
		XrayActive:    serviceActive("xport-xray.service"),
		XrayVersion:   xrayVersion(),
	}
}

func readCPU() cpuTimes {
	b, err := os.ReadFile("/proc/stat")
	if err != nil {
		return cpuTimes{}
	}
	f := strings.Fields(strings.SplitN(string(b), "\n", 2)[0])
	if len(f) < 5 {
		return cpuTimes{}
	}
	v := make([]uint64, 0, len(f)-1)
	for _, x := range f[1:] {
		n, _ := strconv.ParseUint(x, 10, 64)
		v = append(v, n)
	}
	var total uint64
	for _, n := range v {
		total += n
	}
	idle := v[3]
	if len(v) > 4 {
		idle += v[4]
	}
	return cpuTimes{total: total, idle: idle}
}

func cpuPercent(a, b cpuTimes) float64 {
	if b.total <= a.total {
		return 0
	}
	dt := b.total - a.total
	di := b.idle - a.idle
	if di > dt {
		return 0
	}
	return float64(dt-di) * 100 / float64(dt)
}

func readUptime() int64 {
	b, err := os.ReadFile("/proc/uptime")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return int64(v)
}

func readLoad1() float64 {
	b, err := os.ReadFile("/proc/loadavg")
	if err != nil {
		return 0
	}
	f := strings.Fields(string(b))
	if len(f) == 0 {
		return 0
	}
	v, _ := strconv.ParseFloat(f[0], 64)
	return v
}

func readMemory() (uint64, uint64) {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	var total, avail uint64
	s := bufio.NewScanner(f)
	for s.Scan() {
		p := strings.Fields(s.Text())
		if len(p) < 2 {
			continue
		}
		v, _ := strconv.ParseUint(p[1], 10, 64)
		switch p[0] {
		case "MemTotal:":
			total = v * 1024
		case "MemAvailable:":
			avail = v * 1024
		}
	}
	if avail > total {
		avail = total
	}
	return total - avail, total
}

func readDisk() (uint64, uint64) {
	var st unix.Statfs_t
	if unix.Statfs("/", &st) != nil {
		return 0, 0
	}
	if st.Bsize <= 0 {
		return 0, 0
	}
	blockSize := uint64(st.Bsize)
	total := st.Blocks * blockSize
	free := st.Bavail * blockSize
	if free > total {
		free = total
	}
	return total - free, total
}

func readNetwork() (uint64, uint64) {
	f, err := os.Open("/proc/net/dev")
	if err != nil {
		return 0, 0
	}
	defer f.Close()
	var rx, tx uint64
	s := bufio.NewScanner(f)
	for s.Scan() {
		p := strings.SplitN(strings.TrimSpace(s.Text()), ":", 2)
		if len(p) != 2 || strings.TrimSpace(p[0]) == "lo" {
			continue
		}
		v := strings.Fields(p[1])
		if len(v) < 9 {
			continue
		}
		a, _ := strconv.ParseUint(v[0], 10, 64)
		b, _ := strconv.ParseUint(v[8], 10, 64)
		rx += a
		tx += b
	}
	return rx, tx
}

func serviceActive(name string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	return exec.CommandContext(ctx, "systemctl", "is-active", "--quiet", name).Run() == nil
}

func xrayVersion() string {
	ctx, cancel := context.WithTimeout(context.Background(), 800*time.Millisecond)
	defer cancel()
	out, err := exec.CommandContext(ctx, "/usr/local/x-port/bin/xray", "version").Output()
	if err != nil {
		return ""
	}
	line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
	line = strings.TrimPrefix(line, "Xray ")
	f := strings.Fields(line)
	if len(f) == 0 {
		return ""
	}
	return f[0]
}
