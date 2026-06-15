package integration

import (
	"context"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/client"
)

// ========== CLI APPLY INTEGRATION ==========

// ApplyIntegration handles the complete apply → controller → status loop
type ApplyIntegration struct {
	resourceClient client.ResourceClient
	reconciliation *ReconciliationIntegration
	timeout        time.Duration
}

// NewApplyIntegration creates apply integration
func NewApplyIntegration(
	resourceClient client.ResourceClient,
	reconciliation *ReconciliationIntegration,
) *ApplyIntegration {
	return &ApplyIntegration{
		resourceClient: resourceClient,
		reconciliation: reconciliation,
		timeout:        30 * time.Second,
	}
}

// ApplyResult holds result of apply operation
type ApplyResult struct {
	Kind             string
	Name             string
	Namespace        string
	Generation       int64
	Phase            string
	Ready            bool
	Message          string
	ReconciliationID string
}

// Apply applies resource and waits for reconciliation
func (ai *ApplyIntegration) Apply(ctx context.Context, kind, namespace string, resource map[string]interface{}) (*ApplyResult, error) {
	// Set timeout if not already set
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, ai.timeout)
		defer cancel()
	}

	// Step 1: Apply resource via API (this enqueues controller work)
	_, err := ai.resourceClient.Apply(ctx, &client.Resource{
		APIVersion: "v1",
		Kind:       kind,
		Metadata: client.ResourceMetadata{
			Name:      resource["name"].(string),
			Namespace: namespace,
		},
		Spec: resource,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to apply: %v", err)
	}

	name := resource["name"].(string)

	// Step 2: Enqueue for reconciliation
	ai.reconciliation.Enqueue(kind, namespace, name)

	// Step 3: Wait for reconciliation
	err = ai.reconciliation.WaitForReady(ctx, kind, namespace, name)
	if err != nil {
		// Get status even if wait failed
		status := ai.reconciliation.GetStatus(kind, namespace, name)
		if status != nil {
			return &ApplyResult{
				Kind:             kind,
				Name:             name,
				Namespace:        namespace,
				Generation:       status.Generation,
				Phase:            status.Phase,
				Ready:            status.Ready,
				Message:          status.Message,
				ReconciliationID: status.ReconciliationID,
			}, err
		}
		return nil, err
	}

	// Step 4: Get final status
	status := ai.reconciliation.GetStatus(kind, namespace, name)
	if status == nil {
		return nil, fmt.Errorf("no status found after apply")
	}

	return &ApplyResult{
		Kind:             kind,
		Name:             name,
		Namespace:        namespace,
		Generation:       status.Generation,
		Phase:            status.Phase,
		Ready:            status.Ready,
		Message:          status.Message,
		ReconciliationID: status.ReconciliationID,
	}, nil
}

// ApplyWithOptions applies with custom options
type ApplyOptions struct {
	DryRun    bool
	Force     bool
	Timeout   time.Duration
	Namespace string
}

// ApplyWithOpts applies with options
func (ai *ApplyIntegration) ApplyWithOpts(ctx context.Context, kind string, resource map[string]interface{}, opts ApplyOptions) (*ApplyResult, error) {
	if opts.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, opts.Timeout)
		defer cancel()
	}

	if opts.DryRun {
		// For dry run, don't actually apply
		return &ApplyResult{
			Kind:      kind,
			Namespace: opts.Namespace,
			Phase:     "DryRun",
			Ready:     false,
			Message:   "Dry run - no changes applied",
		}, nil
	}

	return ai.Apply(ctx, kind, opts.Namespace, resource)
}

// ========== APPLY STATUS WATCHER ==========

// ApplyStatusWatcher watches apply status in real-time
type ApplyStatusWatcher struct {
	reconciliation *ReconciliationIntegration
	pollInterval   time.Duration
}

// NewApplyStatusWatcher creates watcher
func NewApplyStatusWatcher(reconciliation *ReconciliationIntegration) *ApplyStatusWatcher {
	return &ApplyStatusWatcher{
		reconciliation: reconciliation,
		pollInterval:   500 * time.Millisecond,
	}
}

// WatchApplyProgress watches apply progress
func (asw *ApplyStatusWatcher) WatchApplyProgress(ctx context.Context, kind, namespace, name string) <-chan *ApplyResult {
	resultCh := make(chan *ApplyResult, 10)

	go func() {
		defer close(resultCh)

		ticker := time.NewTicker(asw.pollInterval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				status := asw.reconciliation.GetStatus(kind, namespace, name)
				if status != nil {
					result := &ApplyResult{
						Kind:             kind,
						Name:             name,
						Namespace:        namespace,
						Generation:       status.Generation,
						Phase:            status.Phase,
						Ready:            status.Ready,
						Message:          status.Message,
						ReconciliationID: status.ReconciliationID,
					}

					select {
					case resultCh <- result:
					case <-ctx.Done():
						return
					}

					// Stop if done
					if status.Ready || status.Phase == "Failed" {
						return
					}
				}
			}
		}
	}()

	return resultCh
}

// ========== BATCH APPLY ==========

// BatchApplyRequest represents batch apply request
type BatchApplyRequest struct {
	Kind      string
	Namespace string
	Resources []map[string]interface{}
}

// BatchApplyResult represents batch apply result
type BatchApplyResult struct {
	Kind           string
	TotalResources int
	SuccessCount   int
	FailureCount   int
	Results        []*ApplyResult
}

// ApplyBatch applies multiple resources
func (ai *ApplyIntegration) ApplyBatch(ctx context.Context, requests []BatchApplyRequest) []*BatchApplyResult {
	results := make([]*BatchApplyResult, len(requests))

	for i, req := range requests {
		batchResult := &BatchApplyResult{
			Kind:           req.Kind,
			TotalResources: len(req.Resources),
			Results:        make([]*ApplyResult, 0),
		}

		for _, resource := range req.Resources {
			result, err := ai.Apply(ctx, req.Kind, req.Namespace, resource)
			if err != nil {
				batchResult.FailureCount++
				result = &ApplyResult{
					Kind:      req.Kind,
					Namespace: req.Namespace,
					Phase:     "Failed",
					Message:   err.Error(),
				}
			} else {
				batchResult.SuccessCount++
			}
			batchResult.Results = append(batchResult.Results, result)
		}

		results[i] = batchResult
	}

	return results
}

// ========== DRY RUN MODE ==========

// DryRunValidator validates apply without executing
type DryRunValidator struct {
	reconciliation *ReconciliationIntegration
}

// NewDryRunValidator creates validator
func NewDryRunValidator(reconciliation *ReconciliationIntegration) *DryRunValidator {
	return &DryRunValidator{
		reconciliation: reconciliation,
	}
}

// DryRun performs dry run validation
func (drv *DryRunValidator) DryRun(ctx context.Context, kind, namespace string, resource map[string]interface{}) (*DryRunResult, error) {
	result := &DryRunResult{
		Kind:      kind,
		Namespace: namespace,
		Valid:     true,
	}

	// Validate resource structure
	if _, ok := resource["name"].(string); !ok {
		result.Valid = false
		result.Errors = append(result.Errors, "missing 'name' field")
	}

	if !result.Valid {
		result.Message = "Resource validation failed"
		return result, nil
	}

	// Check if would conflict
	name := resource["name"].(string)
	// Additional validation could happen here

	result.Message = fmt.Sprintf("Would apply %s/%s", kind, name)
	return result, nil
}

// DryRunResult represents dry run result
type DryRunResult struct {
	Kind      string
	Name      string
	Namespace string
	Valid     bool
	Message   string
	Errors    []string
	Warnings  []string
}
