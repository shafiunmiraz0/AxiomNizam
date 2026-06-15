package jobs

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/logging"
	"fmt"
	"math"
	"sync"
	"time"
)

// FairnessScheduler implements fair scheduling algorithms
type FairnessScheduler struct {
	mu              sync.RWMutex
	jobTypes        map[JobType]*TypeStats
	weights         map[JobType]float64
	agingFactor     float64
	starvationLimit time.Duration
	lastScheduled   map[JobType]time.Time
}

// TypeStats tracks statistics per job type
type TypeStats struct {
	TotalProcessed int64
	TotalTime      time.Duration
	AverageTime    time.Duration
	RunCount       int64
	SkippedCount   int64
	LastScheduled  time.Time
	Priority       float64
}

// NewFairnessScheduler creates a new fairness scheduler
func NewFairnessScheduler() *FairnessScheduler {
	return &FairnessScheduler{
		jobTypes:        make(map[JobType]*TypeStats),
		weights:         make(map[JobType]float64),
		agingFactor:     0.1,
		starvationLimit: 5 * time.Minute,
		lastScheduled:   make(map[JobType]time.Time),
	}
}

// SetWeight sets the weight for a job type (0.0-1.0)
func (fs *FairnessScheduler) SetWeight(jobType JobType, weight float64) error {
	if weight < 0 || weight > 1 {
		return fmt.Errorf("weight must be between 0 and 1")
	}

	fs.mu.Lock()
	fs.weights[jobType] = weight
	fs.mu.Unlock()

	logging.Z().Info(fmt.Sprintf("Weight set for %s: %.2f", jobType, weight))
	return nil
}

// SelectNextJob selects next job using weighted round-robin
func (fs *FairnessScheduler) SelectNextJob(availableJobs map[JobType][]*Job) (*Job, JobType) {
	if len(availableJobs) == 0 {
		return nil, ""
	}

	fs.mu.Lock()
	defer fs.mu.Unlock()

	bestJob := (*Job)(nil)
	bestType := JobType("")
	bestScore := math.Inf(-1)

	for jobType, jobs := range availableJobs {
		if len(jobs) == 0 {
			continue
		}

		// Initialize stats if not exists
		if _, exists := fs.jobTypes[jobType]; !exists {
			fs.jobTypes[jobType] = &TypeStats{
				LastScheduled: time.Now(),
			}
		}

		stats := fs.jobTypes[jobType]
		weight := fs.weights[jobType]
		if weight == 0 && len(fs.weights) > 0 {
			weight = 0.5 / float64(len(fs.weights))
		}

		// Calculate score using multiple factors
		score := fs.calculateFairnessScore(jobType, stats, weight)

		if score > bestScore {
			bestScore = score
			bestType = jobType
			bestJob = jobs[0]
		}
	}

	if bestJob != nil {
		fs.lastScheduled[bestType] = time.Now()
	}

	return bestJob, bestType
}

// calculateFairnessScore calculates fairness score for a job type
func (fs *FairnessScheduler) calculateFairnessScore(jobType JobType, stats *TypeStats, weight float64) float64 {
	// Base weight contribution
	score := weight * 100

	// Time since last scheduled (aging factor)
	if !stats.LastScheduled.IsZero() {
		timeSinceScheduled := time.Since(stats.LastScheduled)
		aging := math.Min(float64(timeSinceScheduled.Seconds())*fs.agingFactor, 50)
		score += aging
	}

	// Starvation prevention
	if !stats.LastScheduled.IsZero() && time.Since(stats.LastScheduled) > fs.starvationLimit {
		score += 200 // Boost starved jobs significantly
	}

	// Load balancing factor (inverse of recent activity)
	if stats.RunCount > 0 {
		avgLoadFactor := 1.0 / (1.0 + float64(stats.RunCount)*0.01)
		score *= avgLoadFactor
	}

	return score
}

// RecordExecution records job execution for stats
func (fs *FairnessScheduler) RecordExecution(jobType JobType, duration time.Duration) {
	fs.mu.Lock()
	defer fs.mu.Unlock()

	if _, exists := fs.jobTypes[jobType]; !exists {
		fs.jobTypes[jobType] = &TypeStats{}
	}

	stats := fs.jobTypes[jobType]
	stats.RunCount++
	stats.TotalTime += duration
	stats.AverageTime = stats.TotalTime / time.Duration(stats.RunCount)
	stats.TotalProcessed++
	stats.LastScheduled = time.Now()

	logging.Z().Info(fmt.Sprintf("Execution recorded for %s: %v (avg: %v)", jobType, duration, stats.AverageTime))
}

// GetTypeStats returns statistics for a job type
func (fs *FairnessScheduler) GetTypeStats(jobType JobType) *TypeStats {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	if stats, exists := fs.jobTypes[jobType]; exists {
		return stats
	}
	return nil
}

// GetFairnessReport returns fairness metrics for all job types
func (fs *FairnessScheduler) GetFairnessReport() map[string]interface{} {
	fs.mu.RLock()
	defer fs.mu.RUnlock()

	report := make(map[string]interface{})
	totalProcessed := int64(0)

	typeStats := make(map[string]map[string]interface{})

	for jobType, stats := range fs.jobTypes {
		totalProcessed += stats.TotalProcessed
		weight := fs.weights[JobType(jobType)]

		typeStats[string(jobType)] = map[string]interface{}{
			"weight":         weight,
			"run_count":      stats.RunCount,
			"total_time":     stats.TotalTime,
			"average_time":   stats.AverageTime,
			"last_scheduled": stats.LastScheduled,
			"skipped_count":  stats.SkippedCount,
		}
	}

	report["total_processed"] = totalProcessed
	report["job_types"] = typeStats

	return report
}

// AgeingScheduler implements aging-based priority boost
type AgeingScheduler struct {
	mu             sync.RWMutex
	jobs           map[string]*JobAgingInfo
	baseAge        time.Duration
	ageBoost       float64
	starvationTime time.Duration
}

// JobAgingInfo tracks aging information for a job
type JobAgingInfo struct {
	JobID       string
	CreatedAt   time.Time
	Priority    float64
	Age         time.Duration
	LastBoosted time.Time
	BoostCount  int
}

// NewAgeingScheduler creates a new aging scheduler
func NewAgeingScheduler() *AgeingScheduler {
	return &AgeingScheduler{
		jobs:           make(map[string]*JobAgingInfo),
		baseAge:        1 * time.Minute,
		ageBoost:       0.5,
		starvationTime: 10 * time.Minute,
	}
}

// AddJob adds a job to aging tracking
func (as *AgeingScheduler) AddJob(jobID string, basePriority float64) {
	as.mu.Lock()
	defer as.mu.Unlock()

	as.jobs[jobID] = &JobAgingInfo{
		JobID:       jobID,
		CreatedAt:   time.Now(),
		Priority:    basePriority,
		LastBoosted: time.Now(),
	}
}

// UpdatePriority updates job priority based on age
func (as *AgeingScheduler) UpdatePriority(jobID string) float64 {
	as.mu.Lock()
	defer as.mu.Unlock()

	info, exists := as.jobs[jobID]
	if !exists {
		return 0
	}

	info.Age = time.Since(info.CreatedAt)

	// Calculate priority boost
	if info.Age > as.baseAge {
		boostLevels := int(info.Age / as.baseAge)
		boost := float64(boostLevels) * as.ageBoost
		info.Priority += boost
		info.BoostCount = boostLevels
		info.LastBoosted = time.Now()
	}

	// Extreme starvation protection
	if info.Age > as.starvationTime {
		info.Priority *= 10 // Massive priority increase
	}

	return info.Priority
}

// RemoveJob removes a job from aging tracking
func (as *AgeingScheduler) RemoveJob(jobID string) {
	as.mu.Lock()
	delete(as.jobs, jobID)
	as.mu.Unlock()
}

// GetAgeingStats returns aging information
func (as *AgeingScheduler) GetAgeingStats(jobID string) map[string]interface{} {
	as.mu.RLock()
	defer as.mu.RUnlock()

	info, exists := as.jobs[jobID]
	if !exists {
		return nil
	}

	return map[string]interface{}{
		"job_id":       jobID,
		"age":          info.Age,
		"priority":     info.Priority,
		"boost_count":  info.BoostCount,
		"last_boosted": info.LastBoosted,
		"created_at":   info.CreatedAt,
	}
}

// JobAffinityScheduler implements job affinity for worker placement
type JobAffinityScheduler struct {
	mu          sync.RWMutex
	affinities  map[string]string // jobID -> workerID
	workerLoads map[string]int
	affinityMap map[string][]string // workerID -> compatible job types
	stickiness  float64 // 0.0-1.0 probability of sticking to same worker
}

// NewJobAffinityScheduler creates a new affinity scheduler
func NewJobAffinityScheduler() *JobAffinityScheduler {
	return &JobAffinityScheduler{
		affinities:  make(map[string]string),
		workerLoads: make(map[string]int),
		affinityMap: make(map[string][]string),
		stickiness:  0.7,
	}
}

// SetWorkerAffinity sets compatible job types for a worker
func (jas *JobAffinityScheduler) SetWorkerAffinity(workerID string, jobTypes []JobType) {
	jas.mu.Lock()
	defer jas.mu.Unlock()

	types := make([]string, len(jobTypes))
	for i, jt := range jobTypes {
		types[i] = string(jt)
	}

	jas.affinityMap[workerID] = types
	logging.Z().Info(fmt.Sprintf("Worker affinity set: %s -> %v", workerID, types))
}

// SelectWorkerForJob selects a worker for job placement
func (jas *JobAffinityScheduler) SelectWorkerForJob(jobID string, jobType JobType, availableWorkers []string) string {
	jas.mu.Lock()
	defer jas.mu.Unlock()

	// Check if job has previous affinity
	if prevWorker, exists := jas.affinities[jobID]; exists {
		for _, w := range availableWorkers {
			if w == prevWorker {
				// Check if worker is compatible
				if jas.isWorkerCompatible(w, string(jobType)) {
					return w
				}
			}
		}
	}

	// Find least loaded compatible worker
	var bestWorker string
	minLoad := math.MaxInt

	for _, worker := range availableWorkers {
		if jas.isWorkerCompatible(worker, string(jobType)) {
			load := jas.workerLoads[worker]
			if load < minLoad {
				minLoad = load
				bestWorker = worker
			}
		}
	}

	if bestWorker != "" {
		jas.affinities[jobID] = bestWorker
		jas.workerLoads[bestWorker]++
	}

	return bestWorker
}

// isWorkerCompatible checks if worker can handle job type
func (jas *JobAffinityScheduler) isWorkerCompatible(workerID string, jobType string) bool {
	types, exists := jas.affinityMap[workerID]
	if !exists {
		return true // No restrictions
	}

	for _, t := range types {
		if t == jobType {
			return true
		}
	}
	return false
}

// ReleaseJob removes job-worker affinity and decreases load
func (jas *JobAffinityScheduler) ReleaseJob(jobID string) {
	jas.mu.Lock()
	defer jas.mu.Unlock()

	if worker, exists := jas.affinities[jobID]; exists {
		delete(jas.affinities, jobID)
		if jas.workerLoads[worker] > 0 {
			jas.workerLoads[worker]--
		}
	}
}

// GetWorkerLoad returns the current load for a worker
func (jas *JobAffinityScheduler) GetWorkerLoad(workerID string) int {
	jas.mu.RLock()
	defer jas.mu.RUnlock()

	return jas.workerLoads[workerID]
}

// WorkloadBalancer implements load balancing across workers
type WorkloadBalancer struct {
	mu              sync.RWMutex
	workerMetrics   map[string]*WorkerMetrics
	balancingPolicy string // "round-robin", "least-loaded", "weighted"
	weights         map[string]float64
}

// WorkerMetrics tracks metrics for each worker
type WorkerMetrics struct {
	WorkerID         string
	TotalJobs        int64
	ActiveJobs       int
	FailureRate      float64
	AverageJobTime   time.Duration
	TotalProcessTime time.Duration
	Health           float64 // 0.0-1.0
	LastHealthCheck  time.Time
}

// NewWorkloadBalancer creates a new workload balancer
func NewWorkloadBalancer() *WorkloadBalancer {
	return &WorkloadBalancer{
		workerMetrics:   make(map[string]*WorkerMetrics),
		balancingPolicy: "least-loaded",
		weights:         make(map[string]float64),
	}
}

// SelectWorkerForJob selects a worker based on load balancing policy
func (wb *WorkloadBalancer) SelectWorkerForJob(availableWorkers []string) string {
	wb.mu.RLock()
	defer wb.mu.RUnlock()

	if len(availableWorkers) == 0 {
		return ""
	}

	switch wb.balancingPolicy {
	case "round-robin":
		return availableWorkers[0]

	case "least-loaded":
		return wb.selectLeastLoaded(availableWorkers)

	case "weighted":
		return wb.selectWeighted(availableWorkers)

	default:
		return availableWorkers[0]
	}
}

// selectLeastLoaded selects worker with least active jobs
func (wb *WorkloadBalancer) selectLeastLoaded(workers []string) string {
	var bestWorker string
	minLoad := math.MaxInt

	for _, w := range workers {
		metrics, exists := wb.workerMetrics[w]
		if !exists {
			return w // Prefer new workers
		}

		if metrics.ActiveJobs < minLoad {
			minLoad = metrics.ActiveJobs
			bestWorker = w
		}
	}

	return bestWorker
}

// selectWeighted selects worker based on weighted preference
func (wb *WorkloadBalancer) selectWeighted(workers []string) string {
	type weightedWorker struct {
		worker string
		score  float64
	}

	var candidates []weightedWorker

	for _, w := range workers {
		metrics, exists := wb.workerMetrics[w]
		if !exists {
			candidates = append(candidates, weightedWorker{w, 1.0})
			continue
		}

		// Score based on health, load, and weight
		weight := wb.weights[w]
		if weight == 0 {
			weight = 1.0
		}

		score := weight * metrics.Health / float64(1+metrics.ActiveJobs)
		candidates = append(candidates, weightedWorker{w, score})
	}

	if len(candidates) == 0 {
		return ""
	}

	// Select highest scoring worker
	best := candidates[0]
	for _, c := range candidates[1:] {
		if c.score > best.score {
			best = c
		}
	}

	return best.worker
}

// RecordJobExecution records execution metrics
func (wb *WorkloadBalancer) RecordJobExecution(workerID string, duration time.Duration, success bool) {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	metrics, exists := wb.workerMetrics[workerID]
	if !exists {
		metrics = &WorkerMetrics{
			WorkerID:        workerID,
			Health:          1.0,
			LastHealthCheck: time.Now(),
		}
		wb.workerMetrics[workerID] = metrics
	}

	metrics.TotalJobs++
	metrics.TotalProcessTime += duration
	metrics.AverageJobTime = metrics.TotalProcessTime / time.Duration(metrics.TotalJobs)

	if !success {
		metrics.FailureRate = float64(metrics.FailureRate*0.9 + 0.1)
	}

	// Recalculate health
	metrics.Health = 1.0 - metrics.FailureRate
}

// UpdateWorkerLoad updates active job count
func (wb *WorkloadBalancer) UpdateWorkerLoad(workerID string, delta int) {
	wb.mu.Lock()
	defer wb.mu.Unlock()

	metrics, exists := wb.workerMetrics[workerID]
	if !exists {
		metrics = &WorkerMetrics{
			WorkerID:        workerID,
			Health:          1.0,
			LastHealthCheck: time.Now(),
		}
		wb.workerMetrics[workerID] = metrics
	}

	metrics.ActiveJobs = max(0, metrics.ActiveJobs+delta)
}

// GetWorkerMetrics returns metrics for a worker
func (wb *WorkloadBalancer) GetWorkerMetrics(workerID string) *WorkerMetrics {
	wb.mu.RLock()
	defer wb.mu.RUnlock()

	if metrics, exists := wb.workerMetrics[workerID]; exists {
		return metrics
	}
	return nil
}

// SetBalancingPolicy sets the load balancing policy
func (wb *WorkloadBalancer) SetBalancingPolicy(policy string) {
	wb.mu.Lock()
	wb.balancingPolicy = policy
	wb.mu.Unlock()

	logging.Z().Info(fmt.Sprintf("Balancing policy set to: %s", policy))
}

// Helper function
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
