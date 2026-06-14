package repositories

import (
	"fmt"

	"axiomnizam.bitbd.net/axiomnizam/internal/models"

	"gorm.io/gorm"
)

// LineageRepository interface for lineage operations
type LineageRepository interface {
	CreateNode(node *models.LineageNodeModel) error
	GetNode(id string) (*models.LineageNodeModel, error)
	ListNodes(tenantID, nodeType string, limit, offset int) ([]*models.LineageNodeModel, error)
	UpdateNode(node *models.LineageNodeModel) error
	DeleteNode(id string) error
	CreateEdge(edge *models.LineageEdgeModel) error
	GetEdge(id string) (*models.LineageEdgeModel, error)
	ListIncomingEdges(targetNodeID string) ([]*models.LineageEdgeModel, error)
	ListOutgoingEdges(sourceNodeID string) ([]*models.LineageEdgeModel, error)
	DeleteEdge(id string) error
	CreateProcess(process *models.LineageProcessModel) error
	GetProcess(id string) (*models.LineageProcessModel, error)
	ListProcesses(tenantID string) ([]*models.LineageProcessModel, error)
}

// LineageRepositoryImpl implements LineageRepository
type LineageRepositoryImpl struct {
	db *gorm.DB
}

// NewLineageRepository creates lineage repository
func NewLineageRepository(db *gorm.DB) LineageRepository {
	return &LineageRepositoryImpl{db: db}
}

// CreateNode creates lineage node
func (r *LineageRepositoryImpl) CreateNode(node *models.LineageNodeModel) error {
	return r.db.Create(node).Error
}

// GetNode retrieves node by ID
func (r *LineageRepositoryImpl) GetNode(id string) (*models.LineageNodeModel, error) {
	var node models.LineageNodeModel
	err := r.db.Preload("OutgoingEdges").Preload("IncomingEdges").Where("id = ?", id).First(&node).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("node not found")
	}
	return &node, err
}

// ListNodes lists lineage nodes
func (r *LineageRepositoryImpl) ListNodes(tenantID, nodeType string, limit, offset int) ([]*models.LineageNodeModel, error) {
	var nodes []*models.LineageNodeModel
	query := r.db.Preload("OutgoingEdges").Preload("IncomingEdges").Where("tenant_id = ?", tenantID)
	if nodeType != "" {
		query = query.Where("node_type = ?", nodeType)
	}
	if limit > 0 {
		query = query.Limit(limit)
	}
	if offset > 0 {
		query = query.Offset(offset)
	}
	err := query.Order("created_at DESC").Find(&nodes).Error
	return nodes, err
}

// UpdateNode updates lineage node
func (r *LineageRepositoryImpl) UpdateNode(node *models.LineageNodeModel) error {
	return r.db.Save(node).Error
}

// DeleteNode deletes node
func (r *LineageRepositoryImpl) DeleteNode(id string) error {
	return r.db.Delete(&models.LineageNodeModel{}, "id = ?", id).Error
}

// CreateEdge creates lineage edge
func (r *LineageRepositoryImpl) CreateEdge(edge *models.LineageEdgeModel) error {
	return r.db.Create(edge).Error
}

// GetEdge retrieves edge by ID
func (r *LineageRepositoryImpl) GetEdge(id string) (*models.LineageEdgeModel, error) {
	var edge models.LineageEdgeModel
	err := r.db.Preload("SourceNode").Preload("TargetNode").Where("id = ?", id).First(&edge).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("edge not found")
	}
	return &edge, err
}

// ListIncomingEdges lists incoming edges to a node
func (r *LineageRepositoryImpl) ListIncomingEdges(targetNodeID string) ([]*models.LineageEdgeModel, error) {
	var edges []*models.LineageEdgeModel
	err := r.db.Preload("SourceNode").Where("target_node_id = ?", targetNodeID).Find(&edges).Error
	return edges, err
}

// ListOutgoingEdges lists outgoing edges from a node
func (r *LineageRepositoryImpl) ListOutgoingEdges(sourceNodeID string) ([]*models.LineageEdgeModel, error) {
	var edges []*models.LineageEdgeModel
	err := r.db.Preload("TargetNode").Where("source_node_id = ?", sourceNodeID).Find(&edges).Error
	return edges, err
}

// DeleteEdge deletes edge
func (r *LineageRepositoryImpl) DeleteEdge(id string) error {
	return r.db.Delete(&models.LineageEdgeModel{}, "id = ?", id).Error
}

// CreateProcess creates lineage process
func (r *LineageRepositoryImpl) CreateProcess(process *models.LineageProcessModel) error {
	return r.db.Create(process).Error
}

// GetProcess retrieves process by ID
func (r *LineageRepositoryImpl) GetProcess(id string) (*models.LineageProcessModel, error) {
	var process models.LineageProcessModel
	err := r.db.Where("id = ?", id).First(&process).Error
	if err == gorm.ErrRecordNotFound {
		return nil, fmt.Errorf("process not found")
	}
	return &process, err
}

// ListProcesses lists processes
func (r *LineageRepositoryImpl) ListProcesses(tenantID string) ([]*models.LineageProcessModel, error) {
	var processes []*models.LineageProcessModel
	err := r.db.Where("tenant_id = ?", tenantID).Order("created_at DESC").Find(&processes).Error
	return processes, err
}
