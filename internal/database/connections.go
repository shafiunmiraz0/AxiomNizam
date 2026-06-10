package database

import (
	"context"
	"example.com/axiomnizam/internal/logging"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"example.com/axiomnizam/internal/config"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"go.mongodb.org/mongo-driver/mongo"
	elastic "github.com/elastic/go-elasticsearch/v8"
	etcdclient "go.etcd.io/etcd/client/v3"
)

// Connections holds all database connections
type Connections struct {
	MySQL         *gorm.DB
	MariaDB       *gorm.DB
	Percona       *gorm.DB
	PostgreSQL    *gorm.DB
	MongoDB       *mongo.Client
	Valkey        *redis.Client
	Elasticsearch *elastic.Client
	Etcd          *etcdclient.Client
	Oracle        *gorm.DB
	Firebase      interface{} // Placeholder for Firebase connection
}

// gormCfg is the shared GORM config that suppresses "record not found" noise.
var gormCfg = &gorm.Config{
	Logger: logger.New(
		log.New(os.Stdout, "\r\n", log.LstdFlags),
		logger.Config{
			SlowThreshold:             200 * time.Millisecond,
			LogLevel:                  logger.Warn,
			IgnoreRecordNotFoundError: true,
			Colorful:                  false,
		},
	),
}

// InitConnections initializes system database connections.
// Only PostgreSQL and etcd (when STORAGE_BACKEND=etcd) connect at startup.
// All other databases (MySQL, MariaDB, Percona, MongoDB, Oracle, etc.)
// are configured dynamically from the UI at runtime.
func InitConnections(cfg *config.Config) *Connections {
	conns := &Connections{}

	// PostgreSQL — system database, required
	if db, err := gorm.Open(postgres.Open(cfg.GetPostgresDSN()), gormCfg); err == nil {
		conns.PostgreSQL = db
		logging.Z().Info("✅ PostgreSQL connected")
	} else {
		logging.Z().Info(fmt.Sprintf("❌ PostgreSQL connection failed: %v", err))
	}

	// Valkey — used by metrics tracker and query logger (nil-safe)
	conns.Valkey = redis.NewClient(&redis.Options{
		Addr:     cfg.GetValkeyAddr(),
		Password: cfg.Valkey.Password,
	})
	pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer pingCancel()
	if _, err := conns.Valkey.Ping(pingCtx).Result(); err == nil {
		logging.Z().Info("✅ Valkey connected")
	} else {
		logging.Z().Info(fmt.Sprintf("ℹ️  Valkey not available: %v (metrics will use local fallback)", err))
	}

	// etcd — skip when using embedded Raft storage backend
	storageBackend := strings.ToLower(strings.TrimSpace(os.Getenv("STORAGE_BACKEND")))
	if storageBackend == "raft" {
		logging.Z().Info("ℹ️  etcd skipped (STORAGE_BACKEND=raft — using embedded Raft storage)")
	} else if client, err := etcdclient.New(etcdclient.Config{
		Endpoints:   []string{fmt.Sprintf("%s:%s", cfg.Etcd.Host, cfg.Etcd.Port)},
		DialTimeout: 5 * time.Second,
	}); err == nil {
		conns.Etcd = client
		logging.Z().Info("✅ etcd connected")
	} else {
		logging.Z().Info(fmt.Sprintf("❌ etcd connection failed: %v", err))
	}

	return conns
}

// Close closes all database connections
func (c *Connections) Close() {
	if c.MongoDB != nil {
		c.MongoDB.Disconnect(context.Background())
	}
	if c.Valkey != nil {
		c.Valkey.Close()
	}
	if c.Etcd != nil {
		c.Etcd.Close()
	}
}

// IsConnected returns connection status for all databases
func (c *Connections) IsConnected() map[string]bool {
	status := map[string]bool{
		"mysql":         c.MySQL != nil,
		"mariadb":       c.MariaDB != nil,
		"percona":       c.Percona != nil,
		"postgres":      c.PostgreSQL != nil,
		"mongodb":       c.MongoDB != nil,
		"valkey":        c.Valkey != nil,
		"elasticsearch": c.Elasticsearch != nil,
		"etcd":          c.Etcd != nil,
		"oracle":        c.Oracle != nil,
	}
	return status
}
