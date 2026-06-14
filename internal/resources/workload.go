package resources

import "axiomnizam.bitbd.net/axiomnizam/internal/resources/models"

// --- Type aliases for backward compatibility ---

type WorkloadResource = models.WorkloadResource
type WorkloadSpec = models.WorkloadSpec
type WorkloadTemplate = models.WorkloadTemplate
type RetryStrategy = models.RetryStrategy
type PipelineResource = models.PipelineResource
type PipelineSpec = models.PipelineSpec
type PipelineStage = models.PipelineStage
type PipelineTask = models.PipelineTask
type ScheduleResource = models.ScheduleResource
type ScheduleSpec = models.ScheduleSpec
type ExecutionResource = models.ExecutionResource
type ExecutionSpec = models.ExecutionSpec

// --- Constructor aliases ---

var NewWorkloadResource = models.NewWorkloadResource
var NewPipelineResource = models.NewPipelineResource
var NewScheduleResource = models.NewScheduleResource
var NewExecutionResource = models.NewExecutionResource
