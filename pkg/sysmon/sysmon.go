// Copyright (C) 2025 Josh Simonot
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <https://www.gnu.org/licenses/>.

package sysmon

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"

	"burlo/v2/pkg/logger"

	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/process"
)

type Service struct {
	dir string
	log *logger.Logger
}

func New() *Service {
	dir, err := os.Getwd()
	if err != nil {
		log.Fatalf("Fatal: Error getting working directory: %v\n", err)
	}
	return &Service{
		log: logger.New("System Monitor"),
		dir: dir,
	}
}

func (s *Service) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// System-wide CPU and memory
	cpuPercentList, _ := cpu.Percent(0, false)
	cpuPercent := 0.0
	if len(cpuPercentList) > 0 {
		cpuPercent = cpuPercentList[0]
	}

	vmem, _ := mem.VirtualMemory()
	totalDisk, freeDisk, usedDisk, _ := DiskUsage("/")

	// Current process stats
	p, err := process.NewProcess(int32(os.Getpid()))
	var procMem uint64
	var procCPU float64
	if err == nil {
		if memInfo, err := p.MemoryInfo(); err == nil {
			procMem = memInfo.RSS // resident memory
		}
		if cpuPercent, err := p.CPUPercent(); err == nil {
			procCPU = cpuPercent
		}
	}

	metrics := map[string]any{
		"go_version": runtime.Version(),
		"cpu": map[string]any{
			"system_percent":  cpuPercent,
			"process_percent": procCPU,
		},
		"memory": map[string]any{
			"system_total": vmem.Total,
			"system_used":  vmem.Used,
			"system_free":  vmem.Available,
			"process_rss":  procMem,
		},
		"disk": map[string]any{
			"total": totalDisk,
			"used":  usedDisk,
			"free":  freeDisk,
		},
	}

	// JSON API
	if r.Header.Get("Accept") == "application/json" {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metrics)
		return
	}

	// HTML dashboard
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprintf(w, `
<!DOCTYPE html>
<html>
<head>
	<title>System Monitor</title>
	<style>
		body { font-family: sans-serif; margin: 2em; background: #f9f9f9; }
		h1 { color: #333; }
		table { border-collapse: collapse; width: 60%%; margin-top: 1em; }
		th, td { border: 1px solid #ccc; padding: 0.6em 1em; text-align: left; }
		th { background: #eee; }
	</style>
</head>
<body>
	<h1>System Monitor</h1>
	<h2>Go</h2>
	<p>Version: %s</p>
	<h2>CPU</h2>
	<table>
		<tr><th>System %%</th><th>Process %%</th></tr>
		<tr><td>%.2f%%</td><td>%.2f%%</td></tr>
	</table>
	<h2>Memory</h2>
	<table>
		<tr><th>System Total</th><th>System Used</th><th>System Free</th><th>Process RSS</th></tr>
		<tr>
			<td>%.2f GB</td>
			<td>%.2f GB</td>
			<td>%.2f GB</td>
			<td>%.2f MB</td>
		</tr>
	</table>
	<h2>Disk (/)</h2>
	<table>
		<tr><th>Total</th><th>Used</th><th>Free</th></tr>
		<tr>
			<td>%.2f GB</td>
			<td>%.2f GB</td>
			<td>%.2f GB</td>
		</tr>
	</table>
</body>
</html>
`,
		metrics["go_version"],
		cpuPercent, procCPU,
		float64(vmem.Total)/(1024*1024*1024),
		float64(vmem.Used)/(1024*1024*1024),
		float64(vmem.Available)/(1024*1024*1024),
		float64(procMem)/(1024*1024),
		float64(totalDisk)/(1024*1024*1024),
		float64(usedDisk)/(1024*1024*1024),
		float64(freeDisk)/(1024*1024*1024),
	)

}
