package main

import (
	"math"
	"math/rand"
	"runtime"
	"time"
)

type TelemetrySnapshot struct {
	CPUPercent     float64 `json:"cpu_percent"`
	MemUsedMB      float64 `json:"mem_used_mb"`
	MemTotalMB     float64 `json:"mem_total_mb"`
	Goroutines     int     `json:"goroutines"`
	UptimeSeconds  float64 `json:"uptime_seconds"`
	ThroughputRPS  float64 `json:"throughput_rps"`
	GCCycles       uint32  `json:"gc_cycles"`
}

var cpuHistory [60]float64
var cpuIdx int

func init() {
	base := 15.0
	for i := range cpuHistory {
		cpuHistory[i] = base + rand.Float64()*10
	}
}

func simulateCPU() float64 {
	prev := cpuHistory[(cpuIdx+59)%60]
	delta := (rand.Float64() - 0.48) * 4.0
	next := math.Max(2.0, math.Min(94.0, prev+delta))
	if rand.Float64() < 0.05 {
		next = math.Min(94.0, next+rand.Float64()*20)
	}
	cpuHistory[cpuIdx] = next
	cpuIdx = (cpuIdx + 1) % 60
	return math.Round(next*10) / 10
}

func getTelemetrySnapshot() TelemetrySnapshot {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	usedMB := float64(ms.Alloc) / 1024 / 1024
	sysGB := float64(ms.Sys) / 1024 / 1024

	uptime := time.Since(startTime).Seconds()
	rps := 80.0 + rand.Float64()*140.0 + math.Sin(uptime/30)*20

	return TelemetrySnapshot{
		CPUPercent:    simulateCPU(),
		MemUsedMB:     math.Round(usedMB*100) / 100,
		MemTotalMB:    math.Round(sysGB*100) / 100,
		Goroutines:    runtime.NumGoroutine(),
		UptimeSeconds: math.Round(uptime*10) / 10,
		ThroughputRPS: math.Round(rps*10) / 10,
		GCCycles:      ms.NumGC,
	}
}
