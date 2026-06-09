// Package encryption — Phase 19: Auto-encryption GORM callbacks.
//
// Provides GORM callback hooks that automatically encrypt classified fields
// before persisting to the database and decrypt after loading. This eliminates
// the need for manual EncryptField/DecryptField calls at every write/read site.
//
// Wire into GORM via:
//
//	db.Callback().Create().Before("gorm:create").Register("encrypt:create", enc.BeforeCreate)
//	db.Callback().Create().After("gorm:create").Register("decrypt:create", enc.AfterCreate)
//	db.Callback().Query().After("gorm:query").Register("decrypt:query", enc.AfterQuery)
//	db.Callback().Update().Before("gorm:update").Register("encrypt:update", enc.BeforeUpdate)

package encryption

import (
	"log"

	"gorm.io/gorm"
)

// GORMCallbacks provides automatic encryption/decryption hooks for GORM.
type GORMCallbacks struct {
	encryptor *AutoEncryptor
	enabled   bool
}

// NewGORMCallbacks creates a new GORM callback manager.
// The encryptor handles the actual field-level encryption based on struct tags.
func NewGORMCallbacks(encryptor *AutoEncryptor) *GORMCallbacks {
	return &GORMCallbacks{
		encryptor: encryptor,
		enabled:   true,
	}
}

// Register registers all encryption callbacks on the given GORM DB instance.
// Call this after opening the database connection, before any queries.
func (gc *GORMCallbacks) Register(db *gorm.DB) {
	if !gc.enabled {
		log.Println("ℹ️  Auto-encryption GORM callbacks disabled")
		return
	}

	// Create hooks — encrypt before insert, decrypt after insert (for returning)
	db.Callback().Create().Before("gorm:create").Register("axm:encrypt_create", gc.BeforeCreate)
	db.Callback().Create().After("gorm:create").Register("axm:decrypt_create", gc.AfterCreate)

	// Query hooks — decrypt after loading from database
	db.Callback().Query().After("gorm:query").Register("axm:decrypt_query", gc.AfterQuery)

	// Update hooks — encrypt before update
	db.Callback().Update().Before("gorm:update").Register("axm:encrypt_update", gc.BeforeUpdate)

	// Row hooks — decrypt after scanning
	db.Callback().Row().After("gorm:row").Register("axm:decrypt_row", gc.AfterRow)

	log.Println("✅ Auto-encryption GORM callbacks registered")
}

// Disable turns off all auto-encryption callbacks.
func (gc *GORMCallbacks) Disable() {
	gc.enabled = false
}

// Enable turns on all auto-encryption callbacks.
func (gc *GORMCallbacks) Enable() {
	gc.enabled = true
}

// BeforeCreate encrypts classified fields before INSERT.
func (gc *GORMCallbacks) BeforeCreate(db *gorm.DB) {
	if !gc.enabled {
		return
	}
	gc.encryptModel(db)
}

// AfterCreate decrypts classified fields after INSERT (so the caller sees plaintext).
func (gc *GORMCallbacks) AfterCreate(db *gorm.DB) {
	if !gc.enabled {
		return
	}
	gc.decryptModel(db)
}

// AfterQuery decrypts classified fields after SELECT.
func (gc *GORMCallbacks) AfterQuery(db *gorm.DB) {
	if !gc.enabled {
		return
	}
	gc.decryptModel(db)
}

// BeforeUpdate encrypts classified fields before UPDATE.
func (gc *GORMCallbacks) BeforeUpdate(db *gorm.DB) {
	if !gc.enabled {
		return
	}
	gc.encryptModel(db)
}

// AfterRow decrypts classified fields after raw row scan.
func (gc *GORMCallbacks) AfterRow(db *gorm.DB) {
	if !gc.enabled {
		return
	}
	gc.decryptModel(db)
}

// encryptModel encrypts the model in the GORM statement.
func (gc *GORMCallbacks) encryptModel(db *gorm.DB) {
	if db.Statement.Schema == nil {
		return
	}

	// Get the model value from the statement
	model := db.Statement.Model
	if model == nil {
		return
	}

	// Check if the model has any classified fields before attempting encryption
	if !HasEncryptedFields(model) {
		return
	}

	if err := gc.encryptor.EncryptStruct(model); err != nil {
		db.AddError(err)
	}
}

// decryptModel decrypts the model(s) in the GORM statement.
func (gc *GORMCallbacks) decryptModel(db *gorm.DB) {
	if db.Statement.Schema == nil {
		return
	}

	// For queries, the result may be a slice or a single model
	dest := db.Statement.Dest
	if dest == nil {
		return
	}

	if !HasEncryptedFields(dest) {
		return
	}

	if err := gc.encryptor.DecryptStruct(dest); err != nil {
		db.AddError(err)
	}
}
