package jobs

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"context"
	"fmt"
	"sync"
	"time"
)

// JobDependency represents a dependency between jobs
type JobDependency struct {
	JobID          string    `json:"job_id"`
	DependsOnJobID string    `json:"depends_on_job_id"`
	Status         string    `json:"status"`       // pending, satisfied, failed
	FailureMode    string    `json:"failure_mode"` // block, skip, retry
	CreatedAt      time.Time `json:"created_at"`
}

// JobPipeline represents a sequence of dependent jobs
type JobPipeline struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Description string     `json:"description"`
	Jobs        []string   `json:"jobs"`   // Job IDs in order
	Status      string     `json:"status"` // pending, running, completed, failed
	CreatedBy   string     `json:"created_by"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
	Progress    int        `json:"progress"` // 0-100
	FailedJobs  []string   `json:"failed_jobs,omitempty"`
}

// ConditionalJob allows conditional job submission based on previous results
type ConditionalJob struct {
	ID        string          `json:"id"`
	Condition func(*Job) bool `json:"-"` // Condition function
	Job       *Job            `json:"job"`
	ParentJob string          `json:"parent_job"` // Job ID that must complete first
	OnSuccess bool            `json:"on_success"` // Execute if parent succeeded
	OnFailure bool            `json:"on_failure"` // Execute if parent failed
	CreatedAt time.Time       `json:"created_at"`
}

// DependencyManager manages job dependencies
type DependencyManager struct {
	mu           sync.RWMutex
	dependencies map[string][]*JobDependency // job_id -> dependencies
	pipelines    map[string]*JobPipeline
	conditionals map[string]*ConditionalJob
	queue        Queue
	repository   JobRepository
}

// NewDependencyManager creates a new dependency manager
func NewDependencyManager(queue Queue, repo JobRepository) *DependencyManager {
	return &DependencyManager{
		dependencies: make(map[string][]*JobDependency),
		pipelines:    make(map[string]*JobPipeline),
		conditionals: make(map[string]*ConditionalJob),
		queue:        queue,
		repository:   repo,
	}
}

// AddDependency adds a dependency between two jobs
func (dm *DependencyManager) AddDependency(ctx context.Context, jobID string, dependsOnJobID string, failureMode string) error {
	if jobID == "" || dependsOnJobID == "" {
		return fmt.Errorf("job IDs cannot be empty")
	}

	dm.mu.Lock()
	defer dm.mu.Unlock()

	dep := &JobDependency{
		JobID:          jobID,
		DependsOnJobID: dependsOnJobID,
		Status:         "pending",
		FailureMode:    failureMode, // "block", "skip", "retry"
		CreatedAt:      time.Now(),
	}

	dm.dependencies[jobID] = append(dm.dependencies[jobID], dep)
	logging.Z().Info(fmt.Sprintf("Dependency added: %s depends on %s (mode: %s)", jobID, dependsOnJobID, failureMode))

	return nil
}

// CanJobRun checks if a job can run based on its dependencies
func (dm *DependencyManager) CanJobRun(ctx context.Context, jobID string) (bool, error) {
	dm.mu.RLock()
	deps, exists := dm.dependencies[jobID]
	dm.mu.RUnlock()

	if !exists || len(deps) == 0 {
		return true, nil // No dependencies
	}

	for _, dep := range deps {
		parentJob, err := dm.repository.Get(ctx, dep.DependsOnJobID)
		if err != nil {
			return false, fmt.Errorf("cannot check dependency: %v", err)
		}

		switch parentJob.Status {
		case JobStatusPending, JobStatusRunning, JobStatusRetrying:
			return false, nil // Parent not done
		case JobStatusCancelled:
			if dep.FailureMode == "block" {
				return false, fmt.Errorf("parent job cancelled")
			}
		case JobStatusFailed:
			if dep.FailureMode == "block" {
				return false, fmt.Errorf("parent job failed")
			}
		case JobStatusCompleted:
			// Parent succeeded
		}
	}

	return true, nil
}

// CreatePipeline creates a new job pipeline
func (dm *DependencyManager) CreatePipeline(ctx context.Context, pipeline *JobPipeline) error {
	if pipeline.ID == "" {
		pipeline.ID = generateJobID()
	}
	if pipeline.CreatedAt.IsZero() {
		pipeline.CreatedAt = time.Now()
	}
	pipeline.Status = "pending"
	pipeline.Progress = 0

	dm.mu.Lock()
	dm.pipelines[pipeline.ID] = pipeline
	dm.mu.Unlock()

	logging.Z().Info(fmt.Sprintf("Pipeline created: %s with %d jobs", pipeline.ID, len(pipeline.Jobs)))
	return nil
}

// SubmitPipeline submits all jobs in a pipeline
func (dm *DependencyManager) SubmitPipeline(ctx context.Context, pipelineID string, jobConfigs map[string]map[string]interface{}) error {
	dm.mu.RLock()
	pipeline, exists := dm.pipelines[pipelineID]
	dm.mu.RUnlock()

	if !exists {
		return fmt.Errorf("pipeline not found: %s", pipelineID)
	}

	pipeline.Status = "running"

	// Submit jobs in sequence
	for i, jobID := range pipeline.Jobs {
		config := jobConfigs[jobID]
		if config == nil {
			config = make(map[string]interface{})
		}

		job := CreateJob(JobType(config["type"].(string)), config)
		job.ID = jobID
		job.AddTag(fmt.Sprintf("pipeline-%s", pipelineID))

		// Add dependencies
		if i > 0 {
			prevJobID := pipeline.Jobs[i-1]
			dm.AddDependency(ctx, jobID, prevJobID, "block")
		}

		if err := dm.queue.Submit(ctx, job); err != nil {
			pipeline.Status = "failed"
			pipeline.FailedJobs = append(pipeline.FailedJobs, jobID)
			logging.Z().Info(fmt.Sprintf("Error submitting pipeline job: %v", err))
			return err
		}
	}

	logging.Z().Info(fmt.Sprintf("Pipeline %s submitted with %d jobs", pipelineID, len(pipeline.Jobs)))
	return nil
}

// GetPipelineStatus returns pipeline status
func (dm *DependencyManager) GetPipelineStatus(ctx context.Context, pipelineID string) (*JobPipeline, error) {
	dm.mu.RLock()
	pipeline, exists := dm.pipelines[pipelineID]
	dm.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("pipeline not found")
	}

	// Update progress
	completed := 0
	for _, jobID := range pipeline.Jobs {
		job, err := dm.repository.Get(ctx, jobID)
		if err != nil {
			continue
		}

		if job.Status == JobStatusCompleted {
			completed++
		} else if job.Status == JobStatusFailed {
			pipeline.FailedJobs = append(pipeline.FailedJobs, jobID)
		}
	}

	pipeline.Progress = (completed * 100) / len(pipeline.Jobs)

	if completed == len(pipeline.Jobs) {
		pipeline.Status = "completed"
		now := time.Now()
		pipeline.CompletedAt = &now
	} else if len(pipeline.FailedJobs) > 0 {
		pipeline.Status = "failed"
	}

	return pipeline, nil
}

// ConditionalJobHandler handles conditional job execution
type ConditionalJobHandler struct {
	manager *DependencyManager
}

// NewConditionalJobHandler creates a conditional job handler
func NewConditionalJobHandler(manager *DependencyManager) *ConditionalJobHandler {
	return &ConditionalJobHandler{
		manager: manager,
	}
}

// RegisterConditionalJob registers a conditional job
func (cjh *ConditionalJobHandler) RegisterConditionalJob(parentJobID string, condJob *ConditionalJob) error {
	condJob.ParentJob = parentJobID
	condJob.ID = generateJobID()
	condJob.CreatedAt = time.Now()

	cjh.manager.mu.Lock()
	cjh.manager.conditionals[condJob.ID] = condJob
	cjh.manager.mu.Unlock()

	logging.Z().Info(fmt.Sprintf("Conditional job registered: %s (depends on %s)", condJob.ID, parentJobID))
	return nil
}

// EvaluateAndSubmit evaluates condition and submits job if satisfied
func (cjh *ConditionalJobHandler) EvaluateAndSubmit(ctx context.Context, parentJob *Job) error {
	cjh.manager.mu.RLock()
	conditionals := make([]*ConditionalJob, 0)
	for _, cond := range cjh.manager.conditionals {
		if cond.ParentJob == parentJob.ID {
			conditionals = append(conditionals, cond)
		}
	}
	cjh.manager.mu.RUnlock()

	for _, condJob := range conditionals {
		// Check execution condition
		shouldExecute := false

		if parentJob.Status == JobStatusCompleted && condJob.OnSuccess {
			shouldExecute = true
		} else if parentJob.Status == JobStatusFailed && condJob.OnFailure {
			shouldExecute = true
		}

		// Evaluate custom condition if present
		if shouldExecute && condJob.Condition != nil {
			shouldExecute = condJob.Condition(parentJob)
		}

		if shouldExecute {
			if err := cjh.manager.queue.Submit(ctx, condJob.Job); err != nil {
				logging.Z().Info(fmt.Sprintf("Error submitting conditional job: %v", err))
				return err
			}
			logging.Z().Info(fmt.Sprintf("Conditional job submitted: %s", condJob.ID))
		}
	}

	return nil
}

// JobDAG represents a directed acyclic graph of jobs
type JobDAG struct {
	ID      string
	Nodes   map[string]*Job
	Edges   map[string][]string // job_id -> list of dependencies
	Status  string
	Created time.Time
}

// NewJobDAG creates a new job DAG
func NewJobDAG(id string) *JobDAG {
	return &JobDAG{
		ID:      id,
		Nodes:   make(map[string]*Job),
		Edges:   make(map[string][]string),
		Status:  "pending",
		Created: time.Now(),
	}
}

// AddNode adds a job node to the DAG
func (jd *JobDAG) AddNode(job *Job) {
	jd.Nodes[job.ID] = job
	if _, exists := jd.Edges[job.ID]; !exists {
		jd.Edges[job.ID] = []string{}
	}
}

// AddEdge adds a dependency edge
func (jd *JobDAG) AddEdge(jobID string, dependsOnJobID string) error {
	if _, exists := jd.Nodes[jobID]; !exists {
		return fmt.Errorf("job not found: %s", jobID)
	}
	if _, exists := jd.Nodes[dependsOnJobID]; !exists {
		return fmt.Errorf("dependency job not found: %s", dependsOnJobID)
	}

	jd.Edges[jobID] = append(jd.Edges[jobID], dependsOnJobID)
	return nil
}

// GetReadyJobs returns jobs with all dependencies satisfied
func (jd *JobDAG) GetReadyJobs(ctx context.Context, repository JobRepository) ([]*Job, error) {
	ready := make([]*Job, 0)

	for jobID, deps := range jd.Edges {
		if len(deps) == 0 {
			ready = append(ready, jd.Nodes[jobID])
			continue
		}

		// Check if all dependencies are satisfied
		allSatisfied := true
		for _, depID := range deps {
			depJob, err := repository.Get(ctx, depID)
			if err != nil {
				allSatisfied = false
				break
			}

			if depJob.Status != JobStatusCompleted {
				allSatisfied = false
				break
			}
		}

		if allSatisfied {
			ready = append(ready, jd.Nodes[jobID])
		}
	}

	return ready, nil
}

// BatchJobSubmitter submits batches of jobs with dependencies
type BatchJobSubmitter struct {
	manager   *DependencyManager
	batchSize int
}

// NewBatchJobSubmitter creates a batch job submitter
func NewBatchJobSubmitter(manager *DependencyManager, batchSize int) *BatchJobSubmitter {
	if batchSize <= 0 {
		batchSize = 100
	}

	return &BatchJobSubmitter{
		manager:   manager,
		batchSize: batchSize,
	}
}

// SubmitBatch submits a batch of jobs with dependencies
func (bjs *BatchJobSubmitter) SubmitBatch(ctx context.Context, jobs []*Job, dependencies map[string][]string) error {
	// Submit jobs in batches
	for i := 0; i < len(jobs); i += bjs.batchSize {
		end := i + bjs.batchSize
		if end > len(jobs) {
			end = len(jobs)
		}

		batch := jobs[i:end]

		// Submit batch
		for _, job := range batch {
			if err := bjs.manager.queue.Submit(ctx, job); err != nil {
				logging.Z().Info(fmt.Sprintf("Error submitting job %s: %v", job.ID, err))
				return err
			}

			// Add dependencies
			if deps, exists := dependencies[job.ID]; exists {
				for _, depID := range deps {
					bjs.manager.AddDependency(ctx, job.ID, depID, "block")
				}
			}
		}

		logging.Z().Info(fmt.Sprintf("Submitted batch of %d jobs", len(batch)))
		time.Sleep(100 * time.Millisecond) // Rate limit
	}

	return nil
}

// ParallelJobRunner submits jobs that can run in parallel
type ParallelJobRunner struct {
	manager *DependencyManager
}

// NewParallelJobRunner creates a parallel job runner
func NewParallelJobRunner(manager *DependencyManager) *ParallelJobRunner {
	return &ParallelJobRunner{
		manager: manager,
	}
}

// SubmitParallel submits jobs to run in parallel with a common dependency
func (pjr *ParallelJobRunner) SubmitParallel(ctx context.Context, parentJobID string, parallelJobs []*Job) ([]string, error) {
	jobIDs := make([]string, 0)

	for _, job := range parallelJobs {
		if err := pjr.manager.queue.Submit(ctx, job); err != nil {
			logging.Z().Info(fmt.Sprintf("Error submitting parallel job: %v", err))
			return jobIDs, err
		}

		// Add dependency on parent
		if parentJobID != "" {
			pjr.manager.AddDependency(ctx, job.ID, parentJobID, "block")
		}

		jobIDs = append(jobIDs, job.ID)
	}

	logging.Z().Info(fmt.Sprintf("Submitted %d parallel jobs", len(parallelJobs)))
	return jobIDs, nil
}
