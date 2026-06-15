package accesslog

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Handler exposes access log API endpoints.
type Handler struct {
	store *Store
}

// NewHandler creates a new access log handler.
func NewHandler(store *Store) *Handler {
	return &Handler{store: store}
}

// RegisterRoutes registers access log routes under the given group.
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup, sysadminMiddleware ...gin.HandlerFunc) {
	al := rg.Group("/access-log")
	for _, mw := range sysadminMiddleware {
		al.Use(mw)
	}
	{
		al.GET("/entries", h.GetEntries)
		al.GET("/ips", h.GetUniqueIPs)
		al.GET("/blocked", h.GetBlockedIPs)
		al.POST("/block", h.BlockIP)
		al.DELETE("/block/:ip", h.UnblockIP)
	}
}

// GetEntries returns recent access log entries.
func (h *Handler) GetEntries(c *gin.Context) {
	limit := 500
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	ipFilter := c.Query("ip")
	publicOnly := c.Query("public") == "true"

	var entries []Entry
	if publicOnly {
		// Filter to public IPs only.
		all := h.store.GetEntries(limit*3, ipFilter)
		entries = make([]Entry, 0, limit)
		for _, e := range all {
			if isPublicIP(e.IP) {
				entries = append(entries, e)
				if len(entries) >= limit {
					break
				}
			}
		}
	} else {
		entries = h.store.GetEntries(limit, ipFilter)
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": entries,
		"count":   len(entries),
	})
}

// GetUniqueIPs returns unique IPs with request counts.
func (h *Handler) GetUniqueIPs(c *gin.Context) {
	publicOnly := c.Query("public") == "true"

	var ips []IPStats
	if publicOnly {
		ips = h.store.GetPublicIPs()
	} else {
		ips = h.store.GetUniqueIPs()
	}

	c.JSON(http.StatusOK, gin.H{
		"ips":   ips,
		"count": len(ips),
	})
}

// GetBlockedIPs returns all blocked IPs.
func (h *Handler) GetBlockedIPs(c *gin.Context) {
	blocked := h.store.GetBlockedIPs()
	c.JSON(http.StatusOK, gin.H{
		"blocked": blocked,
		"count":   len(blocked),
	})
}

// BlockIPRequest is the request body for blocking an IP.
type BlockIPRequest struct {
	IP     string `json:"ip" binding:"required"`
	Reason string `json:"reason"`
}

// BlockIP blocks an IP address.
func (h *Handler) BlockIP(c *gin.Context) {
	var req BlockIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}
	h.store.BlockIP(req.IP, req.Reason)
	c.JSON(http.StatusOK, gin.H{
		"message": "IP " + req.IP + " has been blocked",
	})
}

// UnblockIP removes an IP from the blocklist.
func (h *Handler) UnblockIP(c *gin.Context) {
	ip := c.Param("ip")
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ip parameter required"})
		return
	}
	h.store.UnblockIP(ip)
	c.JSON(http.StatusOK, gin.H{
		"message": "IP " + ip + " has been unblocked",
	})
}
