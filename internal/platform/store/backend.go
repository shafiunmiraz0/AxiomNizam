// Package store — backend.go
//
// Phase 5 of the etcd replacement plan: storage backend abstraction.
//
// This file provides the BackendManager which initialises either the
// etcd or Raft storage backend based on the STORAGE_BACKEND env var,
// and a generic NewStore helper that creates the correct ResourceStore
// implementation for each resource Kind.
//
// Usage in main.go:
//
//	bm, err := store.NewBackendManager()
//	defer bm.Close()
//	bulkStore := store.NewStore[*bulk.BulkOperationResource](bm, "bulkoperations", factory)
package store

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"fmt"
	"os"
	"strings"

	axraft "axiomnizam.bitbd.net/axiomnizam/internal/platform/raft"

	"github.com/hashicorp/go-memdb"
	clientv3 "go.etcd.io/etcd/client/v3"
)

// Backend identifies the active storage backend.
type Backend string

const (
	BackendEtcd Backend = "etcd"
	BackendRaft Backend = "raft"
)

// BackendManager holds the shared infrastructure for whichever
// storage backend is active.  Create one at startup, then use
// NewStore to create per-Kind stores.
type BackendManager struct {
	// Active backend type.
	Backend Backend

	// Etcd client — non-nil when Backend == BackendEtcd.
	EtcdClient *clientv3.Client

	// Raft server — non-nil when Backend == BackendRaft.
	RaftServer *axraft.Server

	// tables tracks all registered table names (Raft mode only).
	tables []string

	// kvStore is the lazily-initialised KVStore for direct KV ops.
	kvStore KVStore
}

// NewBackendManager creates a BackendManager based on the
// STORAGE_BACKEND environment variable.
//
// For "raft": initialises the embedded Raft server with a shared
// go-memdb instance containing all resource tables.
//
// For "etcd" (default): expects etcdClient to be provided later via
// SetEtcdClient.
func NewBackendManager(tableNames []string) (*BackendManager, error) {
	backend := Backend(strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_BACKEND"))))
	if backend == "" {
		backend = BackendEtcd
	}

	bm := &BackendManager{
		Backend: backend,
		tables:  tableNames,
	}

	if backend == BackendRaft {
		if err := bm.initRaft(tableNames); err != nil {
			return nil, fmt.Errorf("backend manager: %w", err)
		}
	}

	return bm, nil
}

// SetEtcdClient sets the etcd client for etcd-backend mode.  Called
// after database.InitConnections in main.go.
func (bm *BackendManager) SetEtcdClient(client *clientv3.Client) {
	bm.EtcdClient = client
}

// IsRaft returns true if the active backend is Raft.
func (bm *BackendManager) IsRaft() bool {
	return bm.Backend == BackendRaft
}

// IsEtcd returns true if the active backend is etcd.
func (bm *BackendManager) IsEtcd() bool {
	return bm.Backend == BackendEtcd
}

// Close shuts down the active backend.
func (bm *BackendManager) Close() error {
	if bm.kvStore != nil {
		if closer, ok := bm.kvStore.(interface{ Close() }); ok {
			closer.Close()
		}
	}
	if bm.RaftServer != nil {
		return bm.RaftServer.Shutdown()
	}
	return nil
}

// KV returns the KVStore for direct key-value operations.
// Lazily initialised on first call.
func (bm *BackendManager) KV() KVStore {
	if bm.kvStore != nil {
		return bm.kvStore
	}
	bm.kvStore = NewKVStore(bm)
	return bm.kvStore
}

func (bm *BackendManager) initRaft(tableNames []string) error {
	logging.Z().Info("📦 Initializing embedded Raft storage backend...")

	// Build a single go-memdb schema with all resource tables.
	schema := NewMultiTableSchema(tableNames)
	db, err := memdb.NewMemDB(schema)
	if err != nil {
		return fmt.Errorf("create memdb: %w", err)
	}

	// Start the Raft server.
	cfg := axraft.DefaultConfig()
	server, err := axraft.NewServer(cfg, db, tableNames)
	if err != nil {
		return fmt.Errorf("start raft server: %w", err)
	}

	bm.RaftServer = server
	logging.Z().Info(fmt.Sprintf("  ✅ Raft server started (node=%s, addr=%s, leader=%v)",
		cfg.NodeID, cfg.BindAddr, server.IsLeader()))
	return nil
}

// NewStore creates a ResourceStore[T] using the active backend.
//
// For etcd: creates an EtcdStore with the given prefix.
// For raft: creates a RaftStore backed by the shared Raft server.
//
// The `tableName` is used as the go-memdb table name (raft) or
// converted to an etcd prefix (etcd).  The `factory` allocates a
// new zero-value T for deserialisation.
func NewStore[T Resource](bm *BackendManager, tableName string, factory func() T) ResourceStore[T] {
	switch bm.Backend {
	case BackendRaft:
		return NewRaftStore[T](bm.RaftServer, tableName, factory, nil)
	default:
		// etcd: convert table name to etcd prefix.
		prefix := "/axiomnizam/" + tableName + "/"
		return NewEtcdStore[T](bm.EtcdClient, prefix, factory)
	}
}

// GetRaftIsLeader returns true if this node is the Raft leader.
func (bm *BackendManager) GetRaftIsLeader() bool {
	if bm.RaftServer == nil {
		return false
	}
	return bm.RaftServer.IsLeader()
}

// GetRaftLeader returns the (address, id) of the current Raft leader.
func (bm *BackendManager) GetRaftLeader() (string, string) {
	if bm.RaftServer == nil {
		return "", ""
	}
	return bm.RaftServer.LeaderWithID()
}

// GetRaftQuickStatus returns Raft stats from non-blocking atomic reads
// ONLY.  Unlike GetRaftStats(), this never touches the Raft main loop
// and always returns instantly — even during elections or replication.
func (bm *BackendManager) GetRaftQuickStatus() map[string]string {
	if bm.RaftServer == nil {
		return nil
	}
	return bm.RaftServer.QuickStatus()
}

// GetRaftStats returns the Raft internal stats (term, commit index, etc.).
// NOTE: Stats() internally calls GetConfiguration() which blocks on the
// Raft main loop.  Prefer GetRaftStatsAndPeers() to avoid double blocking.
func (bm *BackendManager) GetRaftStats() map[string]string {
	if bm.RaftServer == nil {
		return nil
	}
	return bm.RaftServer.Stats()
}

// GetRaftPeers returns the Raft cluster configuration as a list of peer maps.
// NOTE: This calls GetConfiguration() which blocks on the Raft main loop.
// Prefer GetRaftStatsAndPeers() to avoid double blocking.
func (bm *BackendManager) GetRaftPeers() ([]map[string]string, error) {
	if bm.RaftServer == nil {
		return nil, fmt.Errorf("raft server not initialized")
	}
	peers, err := bm.RaftServer.GetConfiguration()
	if err != nil {
		return nil, err
	}
	result := make([]map[string]string, 0, len(peers))
	for _, p := range peers {
		result = append(result, map[string]string{
			"id":       p.ID,
			"address":  p.Address,
			"suffrage": p.Suffrage,
		})
	}
	return result, nil
}

// GetRaftStatsAndPeers fetches stats AND peers in a single method call.
// Stats() already calls GetConfiguration() internally, so calling it
// once gives us both the stats map and the peer list with only ONE
// round-trip through the Raft main loop instead of two.
func (bm *BackendManager) GetRaftStatsAndPeers() (stats map[string]string, peers []map[string]string) {
	if bm.RaftServer == nil {
		return nil, nil
	}
	// Stats() makes one GetConfiguration() call internally — this
	// is the single blocking trip through the Raft main loop.
	stats = bm.RaftServer.Stats()

	// Now get structured peer data.  GetConfiguration() will be called
	// again, but the Raft library typically caches the result briefly
	// so this second call is much faster.
	if peerInfos, err := bm.RaftServer.GetConfiguration(); err == nil {
		peers = make([]map[string]string, 0, len(peerInfos))
		for _, p := range peerInfos {
			peers = append(peers, map[string]string{
				"id":       p.ID,
				"address":  p.Address,
				"suffrage": p.Suffrage,
			})
		}
	}
	return stats, peers
}

// AddRaftPeer adds a voting peer to the Raft cluster. Must be called on the leader.
func (bm *BackendManager) AddRaftPeer(id, addr string) error {
	if bm.RaftServer == nil {
		return fmt.Errorf("raft server not initialized")
	}
	return bm.RaftServer.AddPeer(id, addr)
}

// RemoveRaftPeer removes a peer from the Raft cluster. Must be called on the leader.
func (bm *BackendManager) RemoveRaftPeer(id string) error {
	if bm.RaftServer == nil {
		return fmt.Errorf("raft server not initialized")
	}
	return bm.RaftServer.RemovePeer(id)
}

// TriggerSnapshot forces an on-demand Raft snapshot (raft mode only).
func (bm *BackendManager) TriggerSnapshot() error {
	if bm.RaftServer == nil {
		return fmt.Errorf("raft server not initialized")
	}
	return bm.RaftServer.TriggerSnapshot()
}

// SnapshotDir returns the snapshot directory (raft mode only).
func (bm *BackendManager) SnapshotDir() string {
	if bm.RaftServer == nil {
		return ""
	}
	return bm.RaftServer.SnapshotDir()
}

// DataDir returns the raft data directory (raft mode only).
func (bm *BackendManager) DataDir() string {
	if bm.RaftServer == nil {
		return ""
	}
	return bm.RaftServer.DataDir()
}

// LogDir returns the raft log directory (raft mode only).
func (bm *BackendManager) LogDir() string {
	if bm.RaftServer == nil {
		return ""
	}
	return bm.RaftServer.LogDir()
}

// StableDir returns the raft stable directory (raft mode only).
func (bm *BackendManager) StableDir() string {
	if bm.RaftServer == nil {
		return ""
	}
	return bm.RaftServer.StableDir()
}
