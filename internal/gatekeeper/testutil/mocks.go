package testutil

import (
	"context"

	"github.com/google/uuid"
	"axiomnizam.bitbd.net/axiomnizam/internal/gatekeeper/models"
)

// MockFactorRepository is a test double for FactorRepository.
type MockFactorRepository struct {
	Factors map[uuid.UUID]*models.Factor
	Err     error
}

// NewMockFactorRepository creates a new mock factor repository.
func NewMockFactorRepository() *MockFactorRepository {
	return &MockFactorRepository{
		Factors: make(map[uuid.UUID]*models.Factor),
	}
}

func (m *MockFactorRepository) Create(ctx context.Context, factor *models.Factor) (*models.Factor, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	m.Factors[factor.ID] = factor
	return factor, nil
}

func (m *MockFactorRepository) Get(ctx context.Context, id uuid.UUID) (*models.Factor, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	f, ok := m.Factors[id]
	if !ok {
		return nil, nil
	}
	cp := *f
	return &cp, nil
}

func (m *MockFactorRepository) GetByUserID(ctx context.Context, userID uuid.UUID) ([]*models.Factor, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	var result []*models.Factor
	for _, f := range m.Factors {
		if f.UserID == userID {
			cp := *f
			result = append(result, &cp)
		}
	}
	return result, nil
}

func (m *MockFactorRepository) Update(ctx context.Context, factor *models.Factor) (*models.Factor, error) {
	if m.Err != nil {
		return nil, m.Err
	}
	m.Factors[factor.ID] = factor
	return factor, nil
}

func (m *MockFactorRepository) Delete(ctx context.Context, id uuid.UUID) error {
	if m.Err != nil {
		return m.Err
	}
	delete(m.Factors, id)
	return nil
}
