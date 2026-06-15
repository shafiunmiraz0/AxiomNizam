package ipaccess

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// Handler provides HTTP endpoints for IP access log viewing, IP blocking,
// and per-IP rate limiting.
type Handler struct {
	tracker *Tracker
}

// NewHandler creates a new IP access handler.
func NewHandler(tracker *Tracker) *Handler {
	return &Handler{tracker: tracker}
}

// --- Access Log ---

// GetAccessLog returns recent access log entries.
func (h *Handler) GetAccessLog(c *gin.Context) {
	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}
	if limit > 1000 {
		limit = 1000
	}

	ipFilter := c.Query("ip")
	entries := h.tracker.GetRecentEntries(limit)

	if ipFilter != "" {
		var filtered []AccessEntry
		for _, e := range entries {
			if e.IP == ipFilter {
				filtered = append(filtered, e)
			}
		}
		entries = filtered
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": entries,
		"count":   len(entries),
	})
}

// --- IP List ---

// GetIPList returns all unique IPs with summary stats.
func (h *Handler) GetIPList(c *gin.Context) {
	filter := c.DefaultQuery("filter", "all")
	ips := h.tracker.GetIPSnapshots()

	var filtered []*IPSummary
	for _, ip := range ips {
		switch filter {
		case "public":
			if !ip.IsPrivate {
				filtered = append(filtered, ip)
			}
		case "private":
			if ip.IsPrivate {
				filtered = append(filtered, ip)
			}
		default:
			filtered = append(filtered, ip)
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"ips":   filtered,
		"count": len(filtered),
	})
}

// --- IP Detail ---

// GetIPDetail returns detailed stats for a specific IP.
func (h *Handler) GetIPDetail(c *gin.Context) {
	ip := c.Param("ip")
	summary := h.tracker.GetIPSummary(ip)
	if summary == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "IP not found", "code": "NOT_FOUND"})
		return
	}
	c.JSON(http.StatusOK, summary)
}

// GetIPLog returns recent access entries for a specific IP.
func (h *Handler) GetIPLog(c *gin.Context) {
	ip := c.Param("ip")
	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	entries := h.tracker.GetEntriesForIP(ip, limit)
	c.JSON(http.StatusOK, gin.H{
		"ip":      ip,
		"entries": entries,
		"count":   len(entries),
	})
}

// --- Block / Unblock ---

// BlockIPRequest is the request body for blocking an IP.
type BlockIPRequest struct {
	IP     string `json:"ip" binding:"required"`
	Reason string `json:"reason"`
}

// BlockIP adds an IP to the blocklist.
func (h *Handler) BlockIP(c *gin.Context) {
	var req BlockIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	blockedBy := "admin"
	if username, exists := c.Get("username"); exists {
		if u, ok := username.(string); ok {
			blockedBy = u
		}
	}

	h.tracker.BlockIP(req.IP, req.Reason, blockedBy)
	c.JSON(http.StatusOK, gin.H{
		"message": "IP blocked successfully",
		"ip":      req.IP,
		"reason":  req.Reason,
	})
}

// UnblockIPRequest is the request body for unblocking an IP.
type UnblockIPRequest struct {
	IP string `json:"ip" binding:"required"`
}

// UnblockIP removes an IP from the blocklist.
func (h *Handler) UnblockIP(c *gin.Context) {
	var req UnblockIPRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	h.tracker.UnblockIP(req.IP)
	c.JSON(http.StatusOK, gin.H{
		"message": "IP unblocked successfully",
		"ip":      req.IP,
	})
}

// GetBlockedIPs returns all blocked IPs.
func (h *Handler) GetBlockedIPs(c *gin.Context) {
	blocked := h.tracker.GetBlockedIPs()
	c.JSON(http.StatusOK, gin.H{
		"blocked": blocked,
		"count":   len(blocked),
	})
}

// --- Rate Limiting ---

// SetRateLimitRequest is the request body for setting a rate limit.
type SetRateLimitRequest struct {
	IP          string `json:"ip" binding:"required"`
	MaxRequests int    `json:"maxRequests" binding:"required"`
	Window      string `json:"window"`
}

// GetRateLimits returns all rate limit configs.
func (h *Handler) GetRateLimits(c *gin.Context) {
	limits := h.tracker.GetAllRateLimits()
	c.JSON(http.StatusOK, gin.H{
		"rateLimits": limits,
		"count":      len(limits),
	})
}

// SetRateLimit sets a per-IP rate limit.
func (h *Handler) SetRateLimit(c *gin.Context) {
	var req SetRateLimitRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	ip := req.IP
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ip is required"})
		return
	}

	window := req.Window
	if window == "" {
		window = "1m"
	}

	h.tracker.SetRateLimit(ip, RateLimitConfig{
		MaxRequests: req.MaxRequests,
		Window:      window,
	})

	c.JSON(http.StatusOK, gin.H{
		"message":     "Rate limit set successfully",
		"ip":          ip,
		"maxRequests": req.MaxRequests,
		"window":      window,
	})
}

// RemoveRateLimit removes a per-IP rate limit.
func (h *Handler) RemoveRateLimit(c *gin.Context) {
	ip := c.Param("ip")
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ip parameter required"})
		return
	}

	h.tracker.RemoveRateLimit(ip)
	c.JSON(http.StatusOK, gin.H{
		"message": "Rate limit removed successfully",
		"ip":      ip,
	})
}

// GetRateLimitStatus returns rate limit status for a specific IP.
func (h *Handler) GetRateLimitStatus(c *gin.Context) {
	ip := c.Param("ip")
	if ip == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "ip parameter required"})
		return
	}

	status := h.tracker.GetRateLimitStatus(ip)
	c.JSON(http.StatusOK, status)
}
