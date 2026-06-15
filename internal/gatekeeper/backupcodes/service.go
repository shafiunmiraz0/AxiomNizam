package backupcodes

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/repositories"
)

// Service manages backup code generation, validation, and consumption.
type Service struct {
	backupCodeRepo repositories.BackupCodeRepository
}

// NewService creates a new backup code service.
func NewService(bcr repositories.BackupCodeRepository) *Service {
	return &Service{
		backupCodeRepo: bcr,
	}
}

// Generator generates backup codes in the format XXXX-XXXX-XXXX.
type Generator struct{}

// Generate creates N backup codes.
func (g *Generator) Generate(count int) ([]string, error) {
	codes := make([]string, count)
	for i := 0; i < count; i++ {
		code, err := g.generateSingle()
		if err != nil {
			return nil, err
		}
		codes[i] = code
	}
	return codes, nil
}

// generateSingle generates a single backup code.
// Format: XXXX-XXXX-XXXX (3 groups of 4 random hex digits)
func (g *Generator) generateSingle() (string, error) {
	bytes := make([]byte, 6) // 6 random bytes = 12 hex digits
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	hex := fmt.Sprintf("%012x", bytes)
	return fmt.Sprintf("%s-%s-%s", hex[:4], hex[4:8], hex[8:12]), nil
}

// Validator checks backup code format and validity.
type Validator struct{}

// IsValid checks if a code has the correct format.
func (v *Validator) IsValid(code string) bool {
	// Remove dashes
	cleanCode := strings.ReplaceAll(code, "-", "")

	// Must be 12 hex characters
	if len(cleanCode) != 12 {
		return false
	}

	// All characters must be hex digits
	for _, ch := range cleanCode {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') && (ch < 'A' || ch > 'F') {
			return false
		}
	}

	return true
}

// ConsumeBackupCode marks a backup code as used.
func (s *Service) ConsumeBackupCode(ctx context.Context, code string) (*models.BackupCode, error) {
	validator := &Validator{}
	if !validator.IsValid(code) {
		return nil, errors.New("invalid backup code format")
	}

	// Hash the provided code to look it up
	hash := hashBackupCode(code)

	// Find the unused code with this hash
	backupCode, err := s.backupCodeRepo.GetByCodeHash(ctx, hash)
	if err != nil {
		return nil, fmt.Errorf("backup code lookup failed: %w", err)
	}
	if backupCode == nil {
		return nil, errors.New("backup code not found or already used")
	}

	// Mark as used
	if err := s.backupCodeRepo.MarkUsed(ctx, backupCode.ID); err != nil {
		return nil, fmt.Errorf("failed to mark backup code as used: %w", err)
	}

	now := time.Now().UTC()
	backupCode.UsedAt = &now
	return backupCode, nil
}

// RemainingCodes returns the count of unused backup codes for a user.
func (s *Service) RemainingCodes(ctx context.Context, userID models.UserID) (int, error) {
	return s.backupCodeRepo.CountUnused(ctx, userID)
}

// RegenerateCodes generates new backup codes, replacing the old ones.
func (s *Service) RegenerateCodes(ctx context.Context, factorID models.FactorID, count int) ([]string, error) {
	// Delete old codes for this factor
	err := s.backupCodeRepo.DeleteByFactorID(ctx, factorID)
	if err != nil {
		return nil, err
	}

	// Generate new codes (persisted by caller)
	generator := &Generator{}
	return generator.Generate(count)
}

