package streamanalytics

import (
	"axiomnizam.bitbd.net/axiomnizam/internal/streamanalytics/models"
)

// --- Constants (aliases) ---

const (
	StreamJobKind       = models.StreamJobKind
	StreamJobAPIVersion = models.StreamJobAPIVersion
)

// --- Type aliases ---

type StreamSource = models.StreamSource
type WindowSpec = models.WindowSpec
type AggregationSpec = models.AggregationSpec
type FilterSpec = models.FilterSpec
type StreamSink = models.StreamSink
type StreamJobSpec = models.StreamJobSpec
type StreamJobResourceStatus = models.StreamJobResourceStatus
type StreamJobResource = models.StreamJobResource
