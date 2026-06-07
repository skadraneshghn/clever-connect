package handlers

import (
	"image"
	"image/color"
	_ "image/gif"
	_ "image/jpeg"
	"image/png"
	"net/http"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	_ "golang.org/x/image/webp"
)

type SystemStats struct {
	CPUPercent        float64   `json:"cpu_percent"`
	CPUCoresPercent   []float64 `json:"cpu_cores_percent"`
	CPUMhz            float64   `json:"cpu_mhz"`
	MemTotalGB        float64   `json:"mem_total_gb"`
	MemUsedGB         float64   `json:"mem_used_gb"`
	MemPercent        float64   `json:"mem_percent"`
	MemFreeGB         float64   `json:"mem_free_gb"`
	SwapTotalGB       float64   `json:"swap_total_gb"`
	SwapUsedGB        float64   `json:"swap_used_gb"`
	SwapPercent       float64   `json:"swap_percent"`
	DiskTotalGB       float64   `json:"disk_total_gb"`
	DiskUsedGB        float64   `json:"disk_used_gb"`
	DiskPercent       float64   `json:"disk_percent"`
	DiskFreeGB        float64   `json:"disk_free_gb"`
	DiskReadBytesSec  float64   `json:"disk_read_bytes_sec"`
	DiskWriteBytesSec float64   `json:"disk_write_bytes_sec"`
	NetRecvBytesSec   float64   `json:"net_recv_bytes_sec"`
	NetSentBytesSec   float64   `json:"net_sent_bytes_sec"`
	CPUTemp           float64   `json:"cpu_temp"`
	UptimeSeconds     int64     `json:"uptime_seconds"`
	BootTime          uint64    `json:"boot_time"`
	OSPlatform        string    `json:"os_platform"`
	OSKernel          string    `json:"os_kernel"`
	AppMemMB          float64   `json:"app_mem_mb"`
}

var (
	statsCached SystemStats
	statsMu     sync.RWMutex
	startTime   = time.Now()

	// I/O delta tracking
	prevDiskTime  time.Time
	prevReadBytes  uint64
	prevWriteBytes uint64

	prevNetTime  time.Time
	prevRecvBytes uint64
	prevSentBytes uint64
)

func init() {
	// Initialize CPU and other resources
	collectStats()

	// Start lightweight background collector every 3 seconds
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		for range ticker.C {
			collectStats()
		}
	}()
}

func collectStats() {
	statsMu.Lock()
	defer statsMu.Unlock()

	// 1. CPU Percent & Core usage
	cpuPercs, err := cpu.Percent(0, false)
	if err == nil && len(cpuPercs) > 0 {
		statsCached.CPUPercent = cpuPercs[0]
	}
	corePercs, err := cpu.Percent(0, true)
	if err == nil {
		statsCached.CPUCoresPercent = corePercs
	}
	cpuInfos, err := cpu.Info()
	if err == nil && len(cpuInfos) > 0 {
		statsCached.CPUMhz = cpuInfos[0].Mhz
	}

	// 2. Memory & Swap
	vmem, err := mem.VirtualMemory()
	if err == nil {
		statsCached.MemTotalGB = float64(vmem.Total) / 1024 / 1024 / 1024
		statsCached.MemUsedGB = float64(vmem.Used) / 1024 / 1024 / 1024
		statsCached.MemPercent = vmem.UsedPercent
		statsCached.MemFreeGB = float64(vmem.Free) / 1024 / 1024 / 1024
	}
	sw, err := mem.SwapMemory()
	if err == nil {
		statsCached.SwapTotalGB = float64(sw.Total) / 1024 / 1024 / 1024
		statsCached.SwapUsedGB = float64(sw.Used) / 1024 / 1024 / 1024
		statsCached.SwapPercent = sw.UsedPercent
	}

	// 3. Disk Space & IO
	dUsage, err := disk.Usage("/")
	if err == nil {
		statsCached.DiskTotalGB = float64(dUsage.Total) / 1024 / 1024 / 1024
		statsCached.DiskUsedGB = float64(dUsage.Used) / 1024 / 1024 / 1024
		statsCached.DiskPercent = dUsage.UsedPercent
		statsCached.DiskFreeGB = float64(dUsage.Free) / 1024 / 1024 / 1024
	}

	dIOCounters, err := disk.IOCounters()
	if err == nil {
		var totalReadBytes, totalWriteBytes uint64
		for _, io := range dIOCounters {
			totalReadBytes += io.ReadBytes
			totalWriteBytes += io.WriteBytes
		}

		now := time.Now()
		if !prevDiskTime.IsZero() {
			duration := now.Sub(prevDiskTime).Seconds()
			if duration > 0 {
				statsCached.DiskReadBytesSec = float64(totalReadBytes-prevReadBytes) / duration
				statsCached.DiskWriteBytesSec = float64(totalWriteBytes-prevWriteBytes) / duration
			}
		}
		prevDiskTime = now
		prevReadBytes = totalReadBytes
		prevWriteBytes = totalWriteBytes
	}

	// 4. Network delta I/O
	nIOCounters, err := net.IOCounters(false)
	if err == nil && len(nIOCounters) > 0 {
		totalRecvBytes := nIOCounters[0].BytesRecv
		totalSentBytes := nIOCounters[0].BytesSent

		now := time.Now()
		if !prevNetTime.IsZero() {
			duration := now.Sub(prevNetTime).Seconds()
			if duration > 0 {
				statsCached.NetRecvBytesSec = float64(totalRecvBytes-prevRecvBytes) / duration
				statsCached.NetSentBytesSec = float64(totalSentBytes-prevSentBytes) / duration
			}
		}
		prevNetTime = now
		prevRecvBytes = totalRecvBytes
		prevSentBytes = totalSentBytes
	}

	// 5. Temperatures
	temps, err := host.SensorsTemperatures()
	if err == nil && len(temps) > 0 {
		var totalTemp float64
		var count int
		for _, t := range temps {
			if strings.Contains(strings.ToLower(t.SensorKey), "cpu") || strings.Contains(strings.ToLower(t.SensorKey), "core") || strings.Contains(strings.ToLower(t.SensorKey), "temp") {
				totalTemp += t.Temperature
				count++
			}
		}
		if count > 0 {
			statsCached.CPUTemp = totalTemp / float64(count)
		} else {
			statsCached.CPUTemp = temps[0].Temperature
		}
	} else {
		// Fallback: check /sys/class/thermal/thermal_zone0/temp
		data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp")
		if err == nil {
			tStr := strings.TrimSpace(string(data))
			tVal, err := strconv.ParseFloat(tStr, 64)
			if err == nil {
				statsCached.CPUTemp = tVal / 1000.0
			}
		}
	}

	// 6. Uptime & Host info
	hInfo, err := host.Info()
	if err == nil {
		statsCached.UptimeSeconds = int64(hInfo.Uptime)
		statsCached.BootTime = hInfo.BootTime
		statsCached.OSPlatform = hInfo.Platform
		statsCached.OSKernel = hInfo.KernelVersion
	} else {
		statsCached.UptimeSeconds = int64(time.Since(startTime).Seconds())
	}

	// 7. Go App Memory
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	statsCached.AppMemMB = float64(m.Alloc) / 1024 / 1024
}

// GetSystemStats handles GET /api/system/stats and returns cached stats instantly
func GetSystemStats(c *gin.Context) {
	statsMu.RLock()
	defer statsMu.RUnlock()
	c.JSON(http.StatusOK, statsCached)
}

// GetSystemStatsData returns the current cached system stats struct
func GetSystemStatsData() SystemStats {
	statsMu.RLock()
	defer statsMu.RUnlock()
	return statsCached
}

// UploadFavicon handles POST /api/settings/favicon
func UploadFavicon(c *gin.Context) {
	file, err := c.FormFile("favicon")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No file uploaded"})
		return
	}

	srcFile, err := file.Open()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to open uploaded file"})
		return
	}
	defer srcFile.Close()

	img, _, err := image.Decode(srcFile)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unsupported image format. Please upload PNG, JPEG, GIF, or WEBP."})
		return
	}

	// Resize to 32x32 for standard favicon
	resized := resizeImage(img, 32, 32)

	// Ensure data directory exists
	if err := os.MkdirAll("data", 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create data directory"})
		return
	}

	// Save as data/favicon.png
	outFile, err := os.Create("data/favicon.png")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to save favicon on server"})
		return
	}
	defer outFile.Close()

	if err := png.Encode(outFile, resized); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to encode favicon to PNG"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "Favicon updated successfully"})
}

// ServeFavicon serves the custom favicon if it exists, else serves a default icon
func ServeFavicon(c *gin.Context) {
	c.Header("Cache-Control", "no-cache, must-revalidate")
	if _, err := os.Stat("data/favicon.png"); err == nil {
		c.File("data/favicon.png")
		return
	}

	// Generate default favicon: 32x32 orange circle
	img := image.NewRGBA(image.Rect(0, 0, 32, 32))
	brandColor := color.RGBA{255, 107, 44, 255}
	for y := 0; y < 32; y++ {
		for x := 0; x < 32; x++ {
			dx := x - 16
			dy := y - 16
			if dx*dx+dy*dy <= 16*16 {
				img.Set(x, y, brandColor)
			} else {
				img.Set(x, y, color.Transparent)
			}
		}
	}

	c.Header("Content-Type", "image/png")
	_ = png.Encode(c.Writer, img)
}

func resizeImage(src image.Image, width, height int) image.Image {
	dst := image.NewRGBA(image.Rect(0, 0, width, height))
	srcBounds := src.Bounds()
	srcWidth := srcBounds.Dx()
	srcHeight := srcBounds.Dy()

	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			srcX := int(float64(x) / float64(width) * float64(srcWidth)) + srcBounds.Min.X
			srcY := int(float64(y) / float64(height) * float64(srcHeight)) + srcBounds.Min.Y
			dst.Set(x, y, src.At(srcX, srcY))
		}
	}
	return dst
}
