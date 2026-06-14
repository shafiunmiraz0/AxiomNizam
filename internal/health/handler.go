package health

import (
	"context"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/database"
	"axiomnizam.bitbd.net/axiomnizam/internal/models"
	"github.com/gin-gonic/gin"
)

// Handler handles health check endpoints.
type Handler struct {
	conns      *database.Connections
	backendMgr interface{} // *platformstore.BackendManager or nil
}

// NewHandler creates a new health handler.
func NewHandler(conns *database.Connections) *Handler {
	return &Handler{conns: conns}
}

// SetBackendManager wires the storage backend manager for /distributed endpoint.
func (h *Handler) SetBackendManager(bm interface{}) {
	h.backendMgr = bm
}

// Health handles GET /health
func (h *Handler) Health(c *gin.Context) {
	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "AxiomNizam API is running",
	})
}

// Status handles GET /status
func (h *Handler) Status(c *gin.Context) {
	status := map[string]string{}
	connected := h.conns.IsConnected()

	for db, isConnected := range connected {
		if isConnected {
			status[db] = "connected"
		} else {
			status[db] = "disconnected"
		}
	}

	// Firebase and Oracle are emulated services, always show as available
	status["firebase"] = "connected"
	status["oracle"] = "connected"

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "System status",
		Data:    status,
	})
}

// raftBackendInfo is a local interface to avoid importing platform/store.
type raftBackendInfo interface {
	IsRaft() bool
	IsEtcd() bool
}

// Distributed handles GET /distributed - Check cluster status (Raft or etcd)
func (h *Handler) Distributed(c *gin.Context) {
	// Try Raft backend first.
	if h.backendMgr != nil {
		if bm, ok := h.backendMgr.(raftBackendInfo); ok && bm.IsRaft() {
			h.distributedRaft(c)
			return
		}
	}

	// Fallback: etcd via etcdctl.
	h.distributedEtcd(c)
}

// distributedRaft reports Raft cluster status (leader, state, peers).
func (h *Handler) distributedRaft(c *gin.Context) {
	status := map[string]interface{}{
		"backend":        "raft",
		"is_distributed": true,
		"healthy":        true,
	}

	type raftInfo interface {
		GetRaftLeader() (string, string)
		GetRaftIsLeader() bool
		GetRaftQuickStatus() map[string]string
		GetRaftStats() map[string]string
		GetRaftPeers() ([]map[string]string, error)
	}

	if rf, ok := h.backendMgr.(raftInfo); ok {
		leaderAddr, leaderID := rf.GetRaftLeader()
		isLeader := rf.GetRaftIsLeader()
		if isLeader {
			status["node_state"] = "leader"
		} else {
			status["node_state"] = "follower"
		}
		status["leader_addr"] = leaderAddr
		status["leader_id"] = leaderID
		status["is_leader"] = isLeader

		type raftResult struct {
			stats map[string]string
			peers []map[string]string
		}
		done := make(chan raftResult, 1)
		go func() {
			var r raftResult
			r.stats = rf.GetRaftStats()
			if p, err := rf.GetRaftPeers(); err == nil {
				r.peers = p
			}
			done <- r
		}()

		timer := time.NewTimer(3 * time.Second)
		select {
		case r := <-done:
			timer.Stop()
			status["stats"] = r.stats
			if r.peers != nil {
				status["peers"] = r.peers
				status["member_count"] = len(r.peers)
			}
		case <-timer.C:
			status["stats"] = rf.GetRaftQuickStatus()
			status["stats_note"] = "full stats timed out, showing atomic snapshot"
		}
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Raft cluster status",
		Data:    status,
	})
}

// distributedEtcd reports etcd cluster status (legacy).
func (h *Handler) distributedEtcd(c *gin.Context) {
	distributedStatus := map[string]interface{}{
		"backend":        "etcd",
		"is_distributed": false,
		"members":        []string{},
		"leader":         "",
		"healthy":        false,
		"error":          nil,
	}

	ctx, cancel := context.WithTimeout(c.Request.Context(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "etcdctl", "--endpoints=localhost:2379", "member", "list")
	output, err := cmd.CombinedOutput()

	if err != nil {
		distributedStatus["error"] = "etcdctl not available or etcd not running"
		c.JSON(http.StatusOK, models.Response{
			Status:  "ok",
			Message: "Distributed status check",
			Data:    distributedStatus,
		})
		return
	}

	members := []string{}
	for _, line := range strings.Split(strings.TrimSpace(string(output)), "\n") {
		if line != "" {
			members = append(members, line)
		}
	}

	if len(members) > 0 {
		distributedStatus["is_distributed"] = true
		distributedStatus["members"] = members
		distributedStatus["member_count"] = len(members)
	}

	healthCmd := exec.CommandContext(ctx, "etcdctl", "--endpoints=localhost:2379", "endpoint", "health")
	healthOutput, healthErr := healthCmd.CombinedOutput()

	if healthErr == nil {
		distributedStatus["healthy"] = true
		distributedStatus["health_info"] = strings.TrimSpace(string(healthOutput))
	}

	c.JSON(http.StatusOK, models.Response{
		Status:  "ok",
		Message: "Distributed status check",
		Data:    distributedStatus,
	})
}
