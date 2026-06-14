package handlers

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/geo"
	"clever-connect/internal/logger"
	"clever-connect/internal/models"

	"github.com/gin-gonic/gin"
)

type GeoHandler struct {
	cfg *config.Config
}

func NewGeoHandler(cfg *config.Config) *GeoHandler {
	return &GeoHandler{cfg: cfg}
}

// Resolve handles the offline database resolution query for back-compatibility
func (h *GeoHandler) Resolve(c *gin.Context) {
	var req struct {
		IP    string   `json:"ip"`
		IPs   []string `json:"ips"`
		Force bool     `json:"force"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request payload"})
		return
	}

	var ipsToResolve []string
	if req.IP != "" {
		ipsToResolve = append(ipsToResolve, strings.TrimSpace(req.IP))
	}
	for _, ip := range req.IPs {
		ipClean := strings.TrimSpace(ip)
		if ipClean != "" {
			ipsToResolve = append(ipsToResolve, ipClean)
		}
	}

	if len(ipsToResolve) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No IP addresses provided"})
		return
	}

	results := make([]*models.IPRegistry, 0, len(ipsToResolve))
	errors := make(map[string]string)

	for _, ip := range ipsToResolve {
		if net.ParseIP(ip) == nil {
			errors[ip] = "invalid IP address format"
			continue
		}

		res, err := geo.GetEngine().ResolveIP(ip, req.Force)
		if err != nil {
			errors[ip] = err.Error()
		} else {
			results = append(results, res)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
		"errors":  errors,
	})
}

// GetAPIKeys handles GET /api/settings/apikeys
func (h *GeoHandler) GetAPIKeys(c *gin.Context) {
	cfg, err := geo.GetIPLookupConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// SaveAPIKeys handles POST /api/settings/apikeys
func (h *GeoHandler) SaveAPIKeys(c *gin.Context) {
	var req models.IPLookupConfig
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid payload"})
		return
	}
	if err := geo.SaveIPLookupConfig(&req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "success", "message": "API configurations updated successfully"})
}

// PerformLookup handles POST /api/network/lookup
func (h *GeoHandler) PerformLookup(c *gin.Context) {
	var req struct {
		Target string `json:"target"`
		Type   string `json:"type"` // "ip", "domain", or "auto"
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	target := strings.TrimSpace(req.Target)
	if target == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Target cannot be empty"})
		return
	}

	targetType := req.Type
	if targetType == "auto" || targetType == "" {
		targetType = geo.ClassifyTarget(target)
	}

	if targetType == "invalid" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid IP address or domain format"})
		return
	}

	cfg, err := geo.GetIPLookupConfig()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get config"})
		return
	}

	response := geo.UnifiedLookupResponse{
		Target: target,
		Type:   targetType,
	}

	ctx := c.Request.Context()

	if targetType == "domain" {
		// 1. Check WHOIS cache first
		whoisCached, found := geo.QueryDomainWhoisCache(target)
		if found {
			response.Whois = whoisCached
			response.Source = "cache"
		} else {
			// Query Whois API
			whoisRes, err := geo.ResolveDomainWhois(ctx, target, cfg.IP2LocationKey)
			if err != nil {
				logger.Warn("IPLookup", "WHOIS lookup failed", "domain", target, "error", err)
			} else {
				geo.SaveDomainToCache(whoisRes)
				response.Whois = whoisRes
				response.Source = "api"
			}
		}

		// 2. DNS lookup to find the underlying IP address
		ips, err := net.LookupIP(target)
		if err == nil && len(ips) > 0 {
			// Prefer IPv4 for location lookup
			var resolvedIP string
			for _, ip := range ips {
				if ip.To4() != nil {
					resolvedIP = ip.String()
					break
				}
			}
			if resolvedIP == "" {
				resolvedIP = ips[0].String()
			}
			response.ResolvedIP = resolvedIP

			// Feed resolved IP back into geo aggregator
			ipCached, found := geo.QueryIPIntelligenceCache(resolvedIP)
			if found {
				response.Geo = ipCached
				if response.Source != "api" {
					response.Source = "cache"
				}
			} else {
				geoRes, err := geo.ConcurrentGeoResolver(ctx, resolvedIP, cfg)
				if err != nil {
					logger.Error("IPLookup", "Geo resolution failed for domain IP", "ip", resolvedIP, "error", err)
				} else {
					geo.SaveIPToCache(geoRes)
					response.Geo = geoRes
					response.Source = "api"
				}
			}
		} else {
			logger.Warn("IPLookup", "DNS resolution failed", "domain", target, "error", err)
		}

	} else {
		// Target is IP address
		ipCached, found := geo.QueryIPIntelligenceCache(target)
		if found {
			response.Geo = ipCached
			response.Source = "cache"
		} else {
			geoRes, err := geo.ConcurrentGeoResolver(ctx, target, cfg)
			if err != nil {
				logger.Error("IPLookup", "Geo resolution failed for IP", "ip", target, "error", err)
				response.ErrorMsg = err.Error()
				if strings.Contains(strings.ToLower(err.Error()), "quota") || strings.Contains(strings.ToLower(err.Error()), "limit") || strings.Contains(strings.ToLower(err.Error()), "credit") {
					response.QuotaError = true
				}
			} else {
				geo.SaveIPToCache(geoRes)
				response.Geo = geoRes
				response.Source = "api"
			}
		}
	}

	c.JSON(http.StatusOK, response)
}

// TestAPIKey handles POST /api/settings/test-key
func (h *GeoHandler) TestAPIKey(c *gin.Context) {
	var req struct {
		Service string `json:"service"`
		Key     string `json:"key"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	key := strings.TrimSpace(req.Key)
	if key == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "API Key is required"})
		return
	}

	var url string
	switch req.Service {
	case "ip2location":
		url = fmt.Sprintf("https://api.ip2location.io/?key=%s&ip=8.8.8.8", key)
	case "ipgeolocation":
		url = fmt.Sprintf("https://api.ipgeolocation.io/ipgeo?apiKey=%s&ip=8.8.8.8", key)
	case "ipwhois":
		url = fmt.Sprintf("https://ipwhois.pro/8.8.8.8?key=%s", key)
	case "findip":
		url = fmt.Sprintf("https://api.findip.net/8.8.8.8/?token=%s", key)
	default:
		c.JSON(http.StatusBadRequest, gin.H{"error": "Unknown service"})
		return
	}

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Get(url)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"valid": false, "error": fmt.Sprintf("Request failed: %v", err)})
		return
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"valid": false, "error": "Failed to read API response body"})
		return
	}

	if resp.StatusCode != http.StatusOK {
		c.JSON(http.StatusOK, gin.H{"valid": false, "error": fmt.Sprintf("API returned status %d: %s", resp.StatusCode, string(bodyBytes))})
		return
	}

	var data map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &data); err == nil {
		if errObj, ok := data["error"]; ok {
			c.JSON(http.StatusOK, gin.H{"valid": false, "error": fmt.Sprintf("API error response: %v", errObj)})
			return
		}
	}

	c.JSON(http.StatusOK, gin.H{"valid": true})
}
