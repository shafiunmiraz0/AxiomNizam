package database

import (
	"fmt"
	"net/http"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/models"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var dbIdentifierPattern = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]*$`)
var serverKeySanitizer = regexp.MustCompile(`[^a-zA-Z0-9_-]+`)

// DatabaseServerRecord is a GORM model for persisting custom database server connections.
type DatabaseServerRecord struct {
	ID              uint      `gorm:"primaryKey" json:"id"`
	ServerKey       string    `gorm:"uniqueIndex;not null" json:"server_key"`
	ServerName      string    `gorm:"not null" json:"server_name"`
	DBType          string    `gorm:"not null" json:"db_type"`
	Host            string    `gorm:"not null" json:"host"`
	Port            int       `gorm:"not null" json:"port"`
	Username        string    `gorm:"not null" json:"username"`
	Password        string    `gorm:"not null" json:"password"`
	DefaultDatabase string    `json:"default_database"`
	SSLMode         string    `json:"ssl_mode"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

func (DatabaseServerRecord) TableName() string {
	return "database_servers"
}

// Handler handles admin operations like database and table creation
type Handler struct {
	mu              sync.RWMutex
	connections     map[string]*gorm.DB
	connectionTypes map[string]string
	serverMeta      map[string]DatabaseServerInfo
	primaryDB       *gorm.DB // primary DB for persisting server configs
}

// NewHandler creates a new admin handler. primaryDB is used to persist custom server connections (may be nil).
func NewHandler(connections map[string]*gorm.DB, primaryDB *gorm.DB) *Handler {
	h := &Handler{
		connections:     connections,
		connectionTypes: make(map[string]string, len(connections)),
		serverMeta:      make(map[string]DatabaseServerInfo, len(connections)),
		primaryDB:       primaryDB,
	}

	for key, db := range connections {
		h.connectionTypes[key] = key
		h.serverMeta[key] = DatabaseServerInfo{
			Key:       key,
			Name:      strings.ToUpper(key) + " (default)",
			DBType:    key,
			Host:      "configured",
			Port:      0,
			Source:    "default",
			Connected: db != nil,
		}
	}

	// Auto-migrate persistence table and restore saved connections
	if primaryDB != nil {
		if err := primaryDB.AutoMigrate(&DatabaseServerRecord{}); err != nil {
			logging.Z().Warn("failed to migrate database_servers table", zap.Error(err))
		} else {
			h.restoreSavedServers()
		}
	}

	return h
}

// restoreSavedServers reconnects previously persisted custom database servers.
func (h *Handler) restoreSavedServers() {
	var records []DatabaseServerRecord
	if err := h.primaryDB.Find(&records).Error; err != nil {
		logging.Z().Warn("failed to load saved database servers", zap.Error(err))
		return
	}

	for _, rec := range records {
		var (
			db  *gorm.DB
			err error
		)

		switch rec.DBType {
		case "mysql", "mariadb", "percona":
			dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", rec.Username, rec.Password, rec.Host, rec.Port, rec.DefaultDatabase)
			db, err = gorm.Open(mysql.Open(dsn), gormCfg)
		case "postgres":
			sslMode := rec.SSLMode
			if sslMode == "" {
				sslMode = "disable"
			}
			dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=UTC", rec.Host, rec.Username, rec.Password, rec.DefaultDatabase, rec.Port, sslMode)
			db, err = gorm.Open(postgres.Open(dsn), gormCfg)
		default:
			logging.Z().Warn("skipping saved server: unsupported db type", zap.String("key", rec.ServerKey), zap.String("dbType", rec.DBType))
			continue
		}

		connected := true
		if err != nil {
			logging.Z().Warn("could not reconnect saved server", zap.String("key", rec.ServerKey), zap.String("dbType", rec.DBType), zap.Error(err))
			connected = false
		} else if sqlDB, dbErr := db.DB(); dbErr != nil || sqlDB.Ping() != nil {
			logging.Z().Warn("saved server not reachable", zap.String("key", rec.ServerKey), zap.String("dbType", rec.DBType))
			connected = false
		}

		h.mu.Lock()
		if connected {
			h.connections[rec.ServerKey] = db
		}
		h.connectionTypes[rec.ServerKey] = rec.DBType
		h.serverMeta[rec.ServerKey] = DatabaseServerInfo{
			Key:       rec.ServerKey,
			Name:      rec.ServerName,
			DBType:    rec.DBType,
			Host:      rec.Host,
			Port:      rec.Port,
			Source:    "custom",
			Connected: connected,
		}
		h.mu.Unlock()

		if connected {
			logging.Z().Info("restored saved DB server", zap.String("key", rec.ServerKey), zap.String("name", rec.ServerName), zap.String("dbType", rec.DBType), zap.String("host", rec.Host), zap.Int("port", rec.Port))
		}
	}

	if len(records) > 0 {
		logging.Z().Info("loaded saved database servers", zap.Int("count", len(records)))
	}
}

// CreateDatabaseRequest represents a request to create a database
type CreateDatabaseRequest struct {
	DatabaseName string `json:"database_name" binding:"required"`
	DBType       string `json:"db_type" binding:"required"` // mysql, postgres, mongodb, etc.
	DBServer     string `json:"db_server,omitempty"`        // optional server key (default/custom)
}

// ConnectDatabaseServerRequest represents a request to connect a new database server.
type ConnectDatabaseServerRequest struct {
	ServerName      string `json:"server_name" binding:"required"`
	DBType          string `json:"db_type" binding:"required"`
	Host            string `json:"host" binding:"required"`
	Port            int    `json:"port"`
	Username        string `json:"username" binding:"required"`
	Password        string `json:"password"`
	DefaultDatabase string `json:"default_database"`
	SSLMode         string `json:"ssl_mode"`
}

// DatabaseServerInfo describes a database server entry exposed to UI clients.
type DatabaseServerInfo struct {
	Key       string `json:"key"`
	Name      string `json:"name"`
	DBType    string `json:"db_type"`
	Host      string `json:"host"`
	Port      int    `json:"port"`
	Source    string `json:"source"` // default, custom
	Connected bool   `json:"connected"`
}

// CreateTableRequest represents a request to create a table
type CreateTableRequest struct {
	TableName string `json:"table_name" binding:"required"`
	DBType    string `json:"db_type" binding:"required"`
	Columns   []struct {
		Name     string `json:"name" binding:"required"`
		Type     string `json:"type" binding:"required"` // varchar, int, text, etc.
		Size     int    `json:"size"`
		Nullable bool   `json:"nullable"`
		Primary  bool   `json:"primary"`
	} `json:"columns" binding:"required,min=1"`
}

// CreateDatabase creates a new database on the specified DB type
func (h *Handler) CreateDatabase(c *gin.Context) {
	var req CreateDatabaseRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	req.DBType = strings.ToLower(strings.TrimSpace(req.DBType))
	req.DBServer = strings.TrimSpace(req.DBServer)
	if !dbIdentifierPattern.MatchString(req.DatabaseName) {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "Invalid database_name. Use letters, numbers, and underscores only (must start with letter or underscore)",
		})
		return
	}

	// Resolve target connection: explicit server key when provided, otherwise db type default.
	targetKey := req.DBType
	if req.DBServer != "" {
		targetKey = req.DBServer
	}

	h.mu.RLock()
	db, exists := h.connections[targetKey]
	resolvedDBType := req.DBType
	if t, ok := h.connectionTypes[targetKey]; ok && t != "" {
		resolvedDBType = t
	}
	serverInfo, hasServerInfo := h.serverMeta[targetKey]
	h.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Database server '%s' not supported", targetKey),
		})
		return
	}

	if db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Database server '%s' is not connected", targetKey),
		})
		return
	}

	if req.DBType != "" && resolvedDBType != "" && req.DBType != resolvedDBType {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Server '%s' is configured for '%s', but request asked for '%s'", targetKey, resolvedDBType, req.DBType),
		})
		return
	}

	// Create database based on type
	var createSQL string
	switch resolvedDBType {
	case "mysql", "mariadb", "percona":
		createSQL = fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`", req.DatabaseName)
	case "postgres":
		createSQL = fmt.Sprintf("CREATE DATABASE IF NOT EXISTS %s", req.DatabaseName)
	default:
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Database creation not supported for '%s'", resolvedDBType),
		})
		return
	}

	// Execute create database query
	if result := db.Exec(createSQL); result.Error != nil {
		logging.Z().Error("failed to create database", zap.String("database", req.DatabaseName), zap.String("server", targetKey), zap.String("dbType", resolvedDBType), zap.Error(result.Error))
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Failed to create database: %v", result.Error),
		})
		return
	}

	serverName := targetKey
	if hasServerInfo && serverInfo.Name != "" {
		serverName = serverInfo.Name
	}

	logging.Z().Info("database created", zap.String("database", req.DatabaseName), zap.String("server", targetKey), zap.String("dbType", resolvedDBType))
	c.JSON(http.StatusCreated, CreateDatabaseResponse{
		Status:     "success",
		Message:    fmt.Sprintf("Database '%s' created successfully", req.DatabaseName),
		Database:   req.DatabaseName,
		DBType:     resolvedDBType,
		DBServer:   targetKey,
		ServerName: serverName,
	})
}

// ConnectDatabaseServer establishes a connection to a new database server and stores it for admin operations.
func (h *Handler) ConnectDatabaseServer(c *gin.Context) {
	var req ConnectDatabaseServerRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	serverName := strings.TrimSpace(req.ServerName)
	dbType := strings.ToLower(strings.TrimSpace(req.DBType))
	host := strings.TrimSpace(req.Host)
	if serverName == "" || host == "" {
		c.JSON(http.StatusBadRequest, models.Response{Status: "error", Error: "server_name and host are required"})
		return
	}
	if dbType != "mysql" && dbType != "mariadb" && dbType != "percona" && dbType != "postgres" {
		c.JSON(http.StatusBadRequest, models.Response{Status: "error", Error: "db_type must be mysql, mariadb, percona, or postgres"})
		return
	}

	serverKey := normalizeServerKey(serverName)
	if serverKey == "" {
		c.JSON(http.StatusBadRequest, models.Response{Status: "error", Error: "server_name contains no valid characters"})
		return
	}

	defaultPort := defaultPortForDBType(dbType)
	port := req.Port
	if port <= 0 {
		port = defaultPort
	}

	h.mu.RLock()
	if existing, ok := h.serverMeta[serverKey]; ok && existing.Source == "default" {
		h.mu.RUnlock()
		c.JSON(http.StatusBadRequest, models.Response{Status: "error", Error: "server_name conflicts with a default server key; choose another name"})
		return
	}
	h.mu.RUnlock()

	defaultDB := strings.TrimSpace(req.DefaultDatabase)
	if defaultDB == "" {
		if dbType == "postgres" {
			defaultDB = "postgres"
		} else {
			defaultDB = "mysql"
		}
	}

	var (
		db  *gorm.DB
		err error
	)

	switch dbType {
	case "mysql", "mariadb", "percona":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", req.Username, req.Password, host, port, defaultDB)
		db, err = gorm.Open(mysql.Open(dsn), gormCfg)
	case "postgres":
		sslMode := strings.TrimSpace(req.SSLMode)
		if sslMode == "" {
			sslMode = "disable"
		}
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=UTC", host, req.Username, req.Password, defaultDB, port, sslMode)
		db, err = gorm.Open(postgres.Open(dsn), gormCfg)
	}

	if err != nil {
		c.JSON(http.StatusBadGateway, models.Response{Status: "error", Error: fmt.Sprintf("failed to open DB connection: %v", err)})
		return
	}

	sqlDB, err := db.DB()
	if err != nil {
		c.JSON(http.StatusBadGateway, models.Response{Status: "error", Error: fmt.Sprintf("failed to initialize DB connection: %v", err)})
		return
	}
	if err := sqlDB.Ping(); err != nil {
		c.JSON(http.StatusBadGateway, models.Response{Status: "error", Error: fmt.Sprintf("failed to connect to database server: %v", err)})
		return
	}

	h.mu.Lock()
	h.connections[serverKey] = db
	h.connectionTypes[serverKey] = dbType
	h.serverMeta[serverKey] = DatabaseServerInfo{
		Key:       serverKey,
		Name:      serverName,
		DBType:    dbType,
		Host:      host,
		Port:      port,
		Source:    "custom",
		Connected: true,
	}
	h.mu.Unlock()

	// Persist to database so it survives restarts
	if h.primaryDB != nil {
		rec := DatabaseServerRecord{
			ServerKey:       serverKey,
			ServerName:      serverName,
			DBType:          dbType,
			Host:            host,
			Port:            port,
			Username:        req.Username,
			Password:        req.Password,
			DefaultDatabase: defaultDB,
			SSLMode:         strings.TrimSpace(req.SSLMode),
		}
		if err := h.primaryDB.Where("server_key = ?", serverKey).Assign(rec).FirstOrCreate(&rec).Error; err != nil {
			logging.Z().Warn("connected server but failed to persist", zap.String("key", serverKey), zap.Error(err))
		} else {
			logging.Z().Info("persisted custom DB server", zap.String("key", serverKey))
		}
	}

	logging.Z().Info("connected custom DB server", zap.String("key", serverKey), zap.String("name", serverName), zap.String("dbType", dbType), zap.String("host", host), zap.Int("port", port))
	c.JSON(http.StatusCreated, ConnectDatabaseServerResponse{
		Status:  "success",
		Message: "Database server connected",
		Server: DatabaseServerInfo{
			Key:       serverKey,
			Name:      serverName,
			DBType:    dbType,
			Host:      host,
			Port:      port,
			Source:    "custom",
			Connected: true,
		},
	})
}

// ListDatabaseServers lists default and custom database server connections.
func (h *Handler) ListDatabaseServers(c *gin.Context) {
	h.mu.RLock()
	servers := make([]DatabaseServerInfo, 0, len(h.serverMeta))
	for key, info := range h.serverMeta {
		if db, ok := h.connections[key]; ok {
			info.Connected = db != nil
		}
		servers = append(servers, info)
	}
	h.mu.RUnlock()

	sort.Slice(servers, func(i, j int) bool {
		if servers[i].DBType == servers[j].DBType {
			return servers[i].Name < servers[j].Name
		}
		return servers[i].DBType < servers[j].DBType
	})

	c.JSON(http.StatusOK, ListDatabaseServersResponse{
		Status:  "success",
		Count:   len(servers),
		Servers: servers,
	})
}

// CreateTable creates a new table on the specified database
func (h *Handler) CreateTable(c *gin.Context) {
	var req CreateTableRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Invalid request: %v", err),
		})
		return
	}

	// Validate database type
	db, exists := h.connections[req.DBType]
	if !exists {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Database type '%s' not supported", req.DBType),
		})
		return
	}

	if db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Database '%s' is not connected", req.DBType),
		})
		return
	}

	// Build CREATE TABLE SQL based on database type
	var createSQL string
	switch req.DBType {
	case "mysql", "mariadb", "percona":
		createSQL = h.buildMySQLCreateTable(req)
	case "postgres":
		createSQL = h.buildPostgresCreateTable(req)
	default:
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Table creation not supported for '%s'", req.DBType),
		})
		return
	}

	// Execute create table query
	if result := db.Exec(createSQL); result.Error != nil {
		logging.Z().Error("failed to create table", zap.String("table", req.TableName), zap.String("dbType", req.DBType), zap.Error(result.Error))
		c.JSON(http.StatusInternalServerError, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Failed to create table: %v", result.Error),
		})
		return
	}

	logging.Z().Info("table created", zap.String("table", req.TableName), zap.String("dbType", req.DBType))
	c.JSON(http.StatusCreated, CreateTableResponse{
		Status:  "success",
		Message: fmt.Sprintf("Table '%s' created successfully", req.TableName),
		Table:   req.TableName,
		DBType:  req.DBType,
		Columns: len(req.Columns),
	})
}

// buildMySQLCreateTable builds MySQL CREATE TABLE statement
func (h *Handler) buildMySQLCreateTable(req CreateTableRequest) string {
	sql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS `%s` (\n", req.TableName)

	for i, col := range req.Columns {
		sql += fmt.Sprintf("  `%s` %s", col.Name, col.Type)

		if col.Size > 0 {
			sql += fmt.Sprintf("(%d)", col.Size)
		}

		if col.Primary {
			sql += " PRIMARY KEY"
		}

		if !col.Nullable {
			sql += " NOT NULL"
		}

		if i < len(req.Columns)-1 {
			sql += ",\n"
		} else {
			sql += "\n"
		}
	}

	sql += ")"
	return sql
}

// buildPostgresCreateTable builds PostgreSQL CREATE TABLE statement
func (h *Handler) buildPostgresCreateTable(req CreateTableRequest) string {
	sql := fmt.Sprintf("CREATE TABLE IF NOT EXISTS %s (\n", req.TableName)

	for i, col := range req.Columns {
		sql += fmt.Sprintf("  %s %s", col.Name, col.Type)

		if col.Size > 0 {
			sql += fmt.Sprintf("(%d)", col.Size)
		}

		if col.Primary {
			sql += " PRIMARY KEY"
		}

		if !col.Nullable {
			sql += " NOT NULL"
		}

		if i < len(req.Columns)-1 {
			sql += ",\n"
		} else {
			sql += "\n"
		}
	}

	sql += ")"
	return sql
}

// ListDatabases lists all databases on the specified DB type
func (h *Handler) ListDatabases(c *gin.Context) {
	dbType := c.Query("db_type")
	if dbType == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "db_type query parameter is required",
		})
		return
	}

	h.mu.RLock()
	db, exists := h.connections[dbType]
	h.mu.RUnlock()
	if !exists {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Database type '%s' not supported", dbType),
		})
		return
	}

	if db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Database '%s' is not connected", dbType),
		})
		return
	}

	var databases []string

	switch dbType {
	case "mysql", "mariadb", "percona":
		// Query: SHOW DATABASES
		rows, err := db.Raw("SHOW DATABASES").Rows()
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Status: "error",
				Error:  fmt.Sprintf("Failed to list databases: %v", err),
			})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var dbName string
			if err := rows.Scan(&dbName); err != nil {
				continue
			}
			databases = append(databases, dbName)
		}

	case "postgres":
		// Query: SELECT datname FROM pg_database
		rows, err := db.Raw("SELECT datname FROM pg_database WHERE datistemplate = false").Rows()
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Status: "error",
				Error:  fmt.Sprintf("Failed to list databases: %v", err),
			})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var dbName string
			if err := rows.Scan(&dbName); err != nil {
				continue
			}
			databases = append(databases, dbName)
		}

	default:
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Listing databases not supported for '%s'", dbType),
		})
		return
	}

	c.JSON(http.StatusOK, ListDatabasesResponse{
		Status:    "success",
		DBType:    dbType,
		Databases: databases,
		Count:     len(databases),
	})
}

// ListTables lists all tables in the specified database
func (h *Handler) ListTables(c *gin.Context) {
	dbType := c.Query("db_type")
	if dbType == "" {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  "db_type query parameter is required",
		})
		return
	}

	h.mu.RLock()
	db, exists := h.connections[dbType]
	h.mu.RUnlock()
	if !exists {
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Database type '%s' not supported", dbType),
		})
		return
	}

	if db == nil {
		c.JSON(http.StatusServiceUnavailable, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Database '%s' is not connected", dbType),
		})
		return
	}

	var tables []string

	switch dbType {
	case "mysql", "mariadb", "percona":
		// Query: SHOW TABLES
		rows, err := db.Raw("SHOW TABLES").Rows()
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Status: "error",
				Error:  fmt.Sprintf("Failed to list tables: %v", err),
			})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var tableName string
			if err := rows.Scan(&tableName); err != nil {
				continue
			}
			tables = append(tables, tableName)
		}

	case "postgres":
		// Query: SELECT table_name FROM information_schema
		rows, err := db.Raw("SELECT table_name FROM information_schema.tables WHERE table_schema='public'").Rows()
		if err != nil {
			c.JSON(http.StatusInternalServerError, models.Response{
				Status: "error",
				Error:  fmt.Sprintf("Failed to list tables: %v", err),
			})
			return
		}
		defer rows.Close()

		for rows.Next() {
			var tableName string
			if err := rows.Scan(&tableName); err != nil {
				continue
			}
			tables = append(tables, tableName)
		}

	default:
		c.JSON(http.StatusBadRequest, models.Response{
			Status: "error",
			Error:  fmt.Sprintf("Listing tables not supported for '%s'", dbType),
		})
		return
	}

	c.JSON(http.StatusOK, ListTablesResponse{
		Status: "success",
		DBType: dbType,
		Tables: tables,
		Count:  len(tables),
	})
}

func normalizeServerKey(serverName string) string {
	key := strings.TrimSpace(strings.ToLower(serverName))
	key = strings.ReplaceAll(key, " ", "-")
	key = serverKeySanitizer.ReplaceAllString(key, "")
	key = strings.Trim(key, "-")
	return key
}

func defaultPortForDBType(dbType string) int {
	switch dbType {
	case "postgres":
		return 5432
	default:
		return 3306
	}
}

// UpdateDatabaseServer updates connection details for a custom database server, reconnects, and persists.
func (h *Handler) UpdateDatabaseServer(c *gin.Context) {
	serverKey := c.Param("key")
	if serverKey == "" {
		c.JSON(http.StatusBadRequest, models.Response{Status: "error", Error: "server key is required"})
		return
	}

	h.mu.RLock()
	existing, exists := h.serverMeta[serverKey]
	h.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, models.Response{Status: "error", Error: fmt.Sprintf("server '%s' not found", serverKey)})
		return
	}
	if existing.Source == "default" {
		c.JSON(http.StatusForbidden, models.Response{Status: "error", Error: "default servers cannot be edited"})
		return
	}

	var req ConnectDatabaseServerRequest
	if err := c.BindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, models.Response{Status: "error", Error: fmt.Sprintf("Invalid request: %v", err)})
		return
	}

	serverName := strings.TrimSpace(req.ServerName)
	dbType := strings.ToLower(strings.TrimSpace(req.DBType))
	host := strings.TrimSpace(req.Host)
	if serverName == "" || host == "" {
		c.JSON(http.StatusBadRequest, models.Response{Status: "error", Error: "server_name and host are required"})
		return
	}
	if dbType != "mysql" && dbType != "mariadb" && dbType != "percona" && dbType != "postgres" {
		c.JSON(http.StatusBadRequest, models.Response{Status: "error", Error: "db_type must be mysql, mariadb, percona, or postgres"})
		return
	}

	port := req.Port
	if port <= 0 {
		port = defaultPortForDBType(dbType)
	}

	defaultDB := strings.TrimSpace(req.DefaultDatabase)
	if defaultDB == "" {
		if dbType == "postgres" {
			defaultDB = "postgres"
		} else {
			defaultDB = "mysql"
		}
	}

	// Close old connection
	h.mu.RLock()
	if oldDB, ok := h.connections[serverKey]; ok && oldDB != nil {
		if sqlDB, err := oldDB.DB(); err == nil {
			sqlDB.Close()
		}
	}
	h.mu.RUnlock()

	var (
		db  *gorm.DB
		err error
	)

	switch dbType {
	case "mysql", "mariadb", "percona":
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?charset=utf8mb4&parseTime=True&loc=Local", req.Username, req.Password, host, port, defaultDB)
		db, err = gorm.Open(mysql.Open(dsn), gormCfg)
	case "postgres":
		sslMode := strings.TrimSpace(req.SSLMode)
		if sslMode == "" {
			sslMode = "disable"
		}
		dsn := fmt.Sprintf("host=%s user=%s password=%s dbname=%s port=%d sslmode=%s TimeZone=UTC", host, req.Username, req.Password, defaultDB, port, sslMode)
		db, err = gorm.Open(postgres.Open(dsn), gormCfg)
	}

	if err != nil {
		c.JSON(http.StatusBadGateway, models.Response{Status: "error", Error: fmt.Sprintf("failed to open DB connection: %v", err)})
		return
	}

	sqlDB, err := db.DB()
	if err != nil {
		c.JSON(http.StatusBadGateway, models.Response{Status: "error", Error: fmt.Sprintf("failed to initialize DB connection: %v", err)})
		return
	}
	if err := sqlDB.Ping(); err != nil {
		c.JSON(http.StatusBadGateway, models.Response{Status: "error", Error: fmt.Sprintf("failed to connect to database server: %v", err)})
		return
	}

	h.mu.Lock()
	h.connections[serverKey] = db
	h.connectionTypes[serverKey] = dbType
	h.serverMeta[serverKey] = DatabaseServerInfo{
		Key:       serverKey,
		Name:      serverName,
		DBType:    dbType,
		Host:      host,
		Port:      port,
		Source:    "custom",
		Connected: true,
	}
	h.mu.Unlock()

	if h.primaryDB != nil {
		rec := DatabaseServerRecord{
			ServerKey:       serverKey,
			ServerName:      serverName,
			DBType:          dbType,
			Host:            host,
			Port:            port,
			Username:        req.Username,
			Password:        req.Password,
			DefaultDatabase: defaultDB,
			SSLMode:         strings.TrimSpace(req.SSLMode),
		}
		if err := h.primaryDB.Where("server_key = ?", serverKey).Assign(rec).FirstOrCreate(&rec).Error; err != nil {
			logging.Z().Warn("updated server in memory but failed to persist", zap.String("key", serverKey), zap.Error(err))
		}
	}

	logging.Z().Info("updated custom DB server", zap.String("key", serverKey), zap.String("name", serverName), zap.String("dbType", dbType), zap.String("host", host), zap.Int("port", port))
	c.JSON(http.StatusOK, ConnectDatabaseServerResponse{
		Status:  "success",
		Message: "Database server updated",
		Server: DatabaseServerInfo{
			Key:       serverKey,
			Name:      serverName,
			DBType:    dbType,
			Host:      host,
			Port:      port,
			Source:    "custom",
			Connected: true,
		},
	})
}

// DeleteDatabaseServer removes a custom database server connection and deletes the persisted record.
func (h *Handler) DeleteDatabaseServer(c *gin.Context) {
	serverKey := c.Param("key")
	if serverKey == "" {
		c.JSON(http.StatusBadRequest, models.Response{Status: "error", Error: "server key is required"})
		return
	}

	h.mu.RLock()
	existing, exists := h.serverMeta[serverKey]
	h.mu.RUnlock()

	if !exists {
		c.JSON(http.StatusNotFound, models.Response{Status: "error", Error: fmt.Sprintf("server '%s' not found", serverKey)})
		return
	}
	if existing.Source == "default" {
		c.JSON(http.StatusForbidden, models.Response{Status: "error", Error: "default servers cannot be deleted"})
		return
	}

	// Close the connection
	h.mu.Lock()
	if db, ok := h.connections[serverKey]; ok && db != nil {
		if sqlDB, err := db.DB(); err == nil {
			sqlDB.Close()
		}
	}
	delete(h.connections, serverKey)
	delete(h.connectionTypes, serverKey)
	delete(h.serverMeta, serverKey)
	h.mu.Unlock()

	// Remove persisted record
	if h.primaryDB != nil {
		if err := h.primaryDB.Where("server_key = ?", serverKey).Delete(&DatabaseServerRecord{}).Error; err != nil {
			logging.Z().Warn("removed server from memory but failed to delete from DB", zap.String("key", serverKey), zap.Error(err))
		}
	}

	logging.Z().Info("deleted custom DB server", zap.String("key", serverKey))
	c.JSON(http.StatusOK, StatusResponse{
		Status:  "success",
		Message: fmt.Sprintf("Database server '%s' deleted", serverKey),
	})
}
