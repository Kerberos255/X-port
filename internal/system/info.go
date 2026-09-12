package system

type Info struct {
	Hostname       string  `json:"hostname"`
	OS             string  `json:"os"`
	Distribution   string  `json:"distribution"`
	KernelVersion  string  `json:"kernelVersion"`
	SystemType     string  `json:"systemType"`
	HostAddress    string  `json:"hostAddress"`
	BootTime       int64   `json:"bootTime"`
	UptimeSeconds  int64   `json:"uptimeSeconds"`
	CPUPercent     float64 `json:"cpuPercent"`
	Load1          float64 `json:"load1"`
	MemoryUsed     uint64  `json:"memoryUsed"`
	MemoryTotal    uint64  `json:"memoryTotal"`
	DiskUsed       uint64  `json:"diskUsed"`
	DiskTotal      uint64  `json:"diskTotal"`
	NetworkRX      uint64  `json:"networkRx"`
	NetworkTX      uint64  `json:"networkTx"`
	XrayActive     bool    `json:"xrayActive"`
	XrayVersion    string  `json:"xrayVersion"`
}
