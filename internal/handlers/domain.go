package handlers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"clever-connect/internal/config"
	"clever-connect/internal/db/pebble"
	"clever-connect/internal/domainchecker"
	"clever-connect/internal/models"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type DomainHandler struct {
	cfg *config.Config
}

func NewDomainHandler(cfg *config.Config) *DomainHandler {
	return &DomainHandler{cfg: cfg}
}

func (h *DomainHandler) List(c *gin.Context) {
	sortBy := c.DefaultQuery("sortBy", "created_at")
	sortOrder := c.DefaultQuery("sortOrder", "desc")
	category := c.DefaultQuery("category", "ALL")
	search := c.DefaultQuery("search", "")
	status := c.DefaultQuery("status", "")
	tlsFilter := c.DefaultQuery("tlsFilter", "")
	
	httpStatus := 0
	if hs := c.Query("httpStatus"); hs != "" {
		if val, err := strconv.Atoi(hs); err == nil {
			httpStatus = val
		}
	}
	
	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil {
			limit = parsed
		}
	}
	
	offset := 0
	if o := c.Query("offset"); o != "" {
		if parsed, err := strconv.Atoi(o); err == nil {
			offset = parsed
		}
	}
	
	domains, total := pebble.ListDomains(category, search, status, tlsFilter, httpStatus, limit, offset, sortBy, sortOrder)
	
	c.JSON(http.StatusOK, gin.H{
		"domains": domains,
		"total":   total,
	})
}

func (h *DomainHandler) Categories(c *gin.Context) {
	cats := pebble.ListCategories()
	c.JSON(http.StatusOK, gin.H{
		"categories": cats,
	})
}

func (h *DomainHandler) Add(c *gin.Context) {
	var req struct {
		Domains  []string `json:"domains"`
		Category string   `json:"category"`
	}
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	catName := strings.TrimSpace(req.Category)
	if catName == "" {
		catName = "ALL"
	}

	added := 0
	for _, domainName := range req.Domains {
		domainName = strings.TrimSpace(domainName)
		if domainName == "" {
			continue
		}
		
		domainName = strings.ToLower(domainName)
		if strings.HasPrefix(domainName, "http://") {
			domainName = strings.TrimPrefix(domainName, "http://")
		}
		if strings.HasPrefix(domainName, "https://") {
			domainName = strings.TrimPrefix(domainName, "https://")
		}
		domainName = strings.Split(domainName, "/")[0]

		existing, err := pebble.GetDomainByNameAndCategory(domainName, catName)
		if err != nil || existing == nil {
			domain := models.Domain{
				ID:         uuid.New().String(),
				DomainName: domainName,
				Category:   catName,
				Status:     "pending",
				CreatedAt:  time.Now(),
				UpdatedAt:  time.Now(),
			}
			pebble.SaveDomain(&domain)
			added++
		}
	}
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Domains added successfully",
		"added": added,
	})
}

func (h *DomainHandler) CheckSingle(c *gin.Context) {
	id := c.Param("id")
	
	domain, err := pebble.GetDomain(id)
	if err != nil || domain == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Domain not found"})
		return
	}

	domain.Status = "checking"
	pebble.SaveDomain(domain)

	domainchecker.GetEngine().CheckSingle(domain.DomainName)
	
	c.JSON(http.StatusOK, gin.H{
		"message": "Check queued",
		"id": id,
	})
}

func (h *DomainHandler) CheckBulk(c *gin.Context) {
	var req struct {
		IDs      []string `json:"ids"`
		Category string   `json:"category"`
	}
	
	var domains []models.Domain
	if err := c.BindJSON(&req); err == nil {
		if len(req.IDs) > 0 {
			for _, id := range req.IDs {
				if d, err := pebble.GetDomain(id); err == nil && d != nil {
					domains = append(domains, *d)
				}
			}
		} else if req.Category != "" {
			domains, _ = pebble.ListDomains(req.Category, "", "", "", 0, 0, 0, "created_at", "desc")
		} else {
			// check all
			domains, _ = pebble.ListDomains("ALL", "", "", "", 0, 0, 0, "created_at", "desc")
		}
	} else {
		// check all
		domains, _ = pebble.ListDomains("ALL", "", "", "", 0, 0, 0, "created_at", "desc")
	}

	if len(domains) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No domains selected"})
		return
	}

	var targetDomains []string
	for _, d := range domains {
		d.Status = "checking"
		pebble.SaveDomain(&d)
		targetDomains = append(targetDomains, d.DomainName)
	}

	domainchecker.GetEngine().CheckBulk(targetDomains)

	c.JSON(http.StatusOK, gin.H{
		"message": "Bulk check queued",
		"count": len(targetDomains),
	})
}

func (h *DomainHandler) DeleteSingle(c *gin.Context) {
	id := c.Param("id")
	if err := pebble.DeleteDomain(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "Domain deleted successfully"})
}

func (h *DomainHandler) DeleteBulk(c *gin.Context) {
	var req struct {
		IDs      []string `json:"ids"`
		Category string   `json:"category"`
		All      bool     `json:"all"`
	}

	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request"})
		return
	}

	if len(req.IDs) > 0 {
		if err := pebble.DeleteDomainsBulk(req.IDs); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "Selected domains deleted successfully"})
		return
	}

	if req.All {
		if err := pebble.DeleteAllDomains(req.Category); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "All domains deleted successfully"})
		return
	}

	c.JSON(http.StatusBadRequest, gin.H{"error": "Nothing to delete"})
}
