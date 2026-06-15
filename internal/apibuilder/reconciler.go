package apibuilder

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"axiomnizam.bitbd.net/axiomnizam/internal/reconciler"
	amodels "axiomnizam.bitbd.net/axiomnizam/internal/apibuilder/models"
)

// CustomAPIReconciler reconciles CustomAPIResource objects.
type CustomAPIReconciler struct {
	handler *APIBuilderHandler
}

// NewCustomAPIReconciler creates a new reconciler.
func NewCustomAPIReconciler(handler *APIBuilderHandler) *CustomAPIReconciler {
	return &CustomAPIReconciler{handler: handler}
}

// Reconcile ensures the custom API is registered in the in-memory store.
func (r *CustomAPIReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	res, ok := obj.(*amodels.CustomAPIResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("expected *CustomAPIResource, got %T", obj)}
	}

	r.handler.mu.Lock()
	defer r.handler.mu.Unlock()

	apiID := res.Name
	existing := r.handler.customAPIs[apiID]
	if existing == nil {
		// Create the API entry from the resource spec
		api := &CustomAPI{
			ID:             apiID,
			APIType:        string(res.Spec.APIType),
			Name:           res.Spec.Name,
			Method:         string(res.Spec.Method),
			Path:           res.Spec.Path,
			SQLTemplate:    res.Spec.SQLTemplate,
			SQLPolicyMode:  res.Spec.SQLPolicyMode,
			GraphQLQuery:   res.Spec.GraphQLQuery,
			GraphQLOpName:  res.Spec.GraphQLOpName,
			Description:    res.Spec.Description,
			Category:       res.Spec.Category,
			SourceDatabase: res.Spec.SourceDatabase,
			SourceServer:   res.Spec.SourceServer,
			AuthRequired:   res.Spec.AuthRequired,
			RateLimit:      res.Spec.RateLimit,
			CacheEnabled:   res.Spec.CacheEnabled,
			CacheTTL:       res.Spec.CacheTTL,
			RequestSchema:  nil,
			ResponseSchema: nil,
			Headers:        res.Spec.Headers,
			Status:         "active",
			CreatedBy:      "reconciler",
			CreatedAt:      time.Now().UTC(),
			UpdatedAt:      time.Now().UTC(),
			rateBuckets:    make(map[string]*apiRuntimeRateBucket),
		}
		r.handler.customAPIs[apiID] = api
		r.handler.persistStateLocked()
		logging.Z().Info(fmt.Sprintf("apibuilder reconcile: created API %s/%s", res.Namespace, res.Name))
	}

	return reconciler.ReconcileResult{
		Requeue:      true,
		RequeueAfter: 5 * time.Minute,
	}
}

// CSVUploadReconciler reconciles CSVUploadResource objects.
type CSVUploadReconciler struct{}

// NewCSVUploadReconciler creates a new reconciler.
func NewCSVUploadReconciler() *CSVUploadReconciler {
	return &CSVUploadReconciler{}
}

// Reconcile processes CSV upload resources.
func (r *CSVUploadReconciler) Reconcile(ctx context.Context, obj reconciler.Resource) reconciler.ReconcileResult {
	_, ok := obj.(*amodels.CSVUploadResource)
	if !ok {
		return reconciler.ReconcileResult{Error: fmt.Errorf("expected *CSVUploadResource, got %T", obj)}
	}
	// CSV uploads are processed on-demand; reconciler just tracks state.
	return reconciler.ReconcileResult{}
}
