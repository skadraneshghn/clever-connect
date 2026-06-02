package handlers

import (
	"bufio"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/gin-gonic/gin"
)

type SystemStats struct {
	CPUPercent    float64 `json:"cpu_percent"`
	MemTotalGB    float64 `json:"mem_total_gb"`
	MemUsedGB     float64 `json:"mem_used_gb"`
	MemPercent    float64 `json:"mem_percent"`
	DiskTotalGB   float64 `json:"disk_total_gb"`
	DiskUsedGB    float64 `json:"disk_used_gb"`
	DiskPercent   float64 `json:"disk_percent"`
	AppMemMB      float64 `json:"app_mem_mb"`
	UptimeSeconds int64   `json:"uptime_seconds"`
}

var (
	statsCached SystemStats
	statsMu     sync.RWMutex
	startTime   = time.Now()
	prevIdle    uint64
	prevTotal   uint64
)

func init() {
	// Initialize CPU values
	prevIdle, prevTotal = getCPUSample()
	
	// Start lightweight background collector every 3 seconds
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		for range ticker.C {
			collectStats()
		}
	}()
}

func getCPUSample() (idle, total uint64) {
	file, err := os.Open("/proc/stat")
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	if scanner.Scan() {
		line := scanner.Text()
		fields := strings.Fields(line)
		if len(fields) > 4 && fields[0] == "cpu" {
			var sum uint64
			for i := 1; i < len(fields); i++ {
				val, _ := strconv.ParseUint(fields[i], 10, 64)
				sum += val
			}
			idleVal, _ := strconv.ParseUint(fields[4], 10, 64)
			return idleVal, sum
		}
	}
	return
}

func collectStats() {
	statsMu.Lock()
	defer statsMu.Unlock()

	// 1. Calculate CPU Percent
	idle, total := getCPUSample()
	if total > prevTotal {
		diffIdle := idle - prevIdle
		diffTotal := total - prevTotal
		statsCached.CPUPercent = float64(diffTotal-diffIdle) / float64(diffTotal) * 100.0
	}
	prevIdle = idle
	prevTotal = total

	// 2. Parse Memory from /proc/meminfo
	file, err := os.Open("/proc/meminfo")
	if err == nil {
		var totalMem, availMem float64
		scanner := bufio.NewScanner(file)
		for scanner.Scan() {
			line := scanner.Text()
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				key := parts[0]
				val, _ := strconv.ParseFloat(parts[1], 64)
				if key == "MemTotal:" {
					totalMem = val / 1024 / 1024 // GB
				} else if key == "MemAvailable:" {
					availMem = val / 1024 / 1024 // GB
				}
			}
		}
		file.Close()

		if totalMem > 0 {
			statsCached.MemTotalGB = totalMem
			statsCached.MemUsedGB = totalMem - availMem
			statsCached.MemPercent = (statsCached.MemUsedGB / totalMem) * 100.0
		}
	}

	// 3. Disk Usage using syscall.Statfs
	var stat syscall.Statfs_t
	if err := syscall.Statfs("/", &stat); err == nil {
		totalDisk := float64(stat.Blocks*uint64(stat.Bsize)) / 1024 / 1024 / 1024 // GB
		freeDisk := float64(stat.Bfree*uint64(stat.Bsize)) / 1024 / 1024 / 1024   // GB
		statsCached.DiskTotalGB = totalDisk
		statsCached.DiskUsedGB = totalDisk - freeDisk
		statsCached.DiskPercent = (statsCached.DiskUsedGB / totalDisk) * 100.0
	}

	// 4. Go App Memory usage
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	statsCached.AppMemMB = float64(m.Alloc) / 1024 / 1024 // MB

	// 5. Uptime
	statsCached.UptimeSeconds = int64(time.Since(startTime).Seconds())
}

// GetSystemStats handles GET /api/system/stats and returns cached stats instantly
func GetSystemStats(c *gin.Context) {
	statsMu.RLock()
	defer statsMu.RUnlock()
	c.JSON(http.StatusOK, statsCached)
}
