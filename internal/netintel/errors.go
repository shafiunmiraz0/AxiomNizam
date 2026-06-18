package netintel

import (
	"errors"
	"fmt"
)

var (
	// ErrParserNotFound is returned when a parser ID doesn't match any known parser.
	ErrParserNotFound = errors.New("netintel: parser not found")

	// ErrParserNameRequired is returned when creating a parser without a name.
	ErrParserNameRequired = errors.New("netintel: parser name is required")

	// ErrEntryInvalid is returned when an ingested log entry fails validation.
	ErrEntryInvalid = errors.New("netintel: invalid log entry")

	// ErrNodeNotFound is returned when a topology node ID doesn't match.
	ErrNodeNotFound = errors.New("netintel: topology node not found")

	// ErrAnomalyNotFound is returned when an anomaly ID doesn't match.
	ErrAnomalyNotFound = errors.New("netintel: anomaly not found")

	// ErrAlertNotFound is returned when an alert ID doesn't match.
	ErrAlertNotFound = errors.New("netintel: alert not found")

	// ErrForecastNotFound is returned when a forecast metric doesn't exist.
	ErrForecastNotFound = errors.New("netintel: forecast not found")

	// ErrTrackNotFound is returned when a device track MAC doesn't exist.
	ErrTrackNotFound = errors.New("netintel: track not found")

	// ErrRateLimited is returned when ingestion rate exceeds limits.
	ErrRateLimited = errors.New("netintel: rate limited")

	// ErrEngineStopped is returned when operations are attempted on a stopped engine.
	ErrEngineStopped = errors.New("netintel: engine stopped")

	// ErrWiFiScanFailed is returned when the OS-level WiFi scan command fails.
	ErrWiFiScanFailed = errors.New("netintel: WiFi scan failed")

	// ErrWiFiNotSupported is returned when WiFi scanning is not supported on the current platform.
	ErrWiFiNotSupported = errors.New("netintel: WiFi scanning not supported on this platform")

	// ErrWiFiInterfaceNotFound is returned when no wireless interface is detected.
	ErrWiFiInterfaceNotFound = errors.New("netintel: no wireless interface found")
)

// ParserError wraps a parser operation failure with context.
type ParserError struct {
	ParserID string
	Op       string
	Err      error
}

func (e *ParserError) Error() string {
	return fmt.Sprintf("netintel: parser %q %s: %v", e.ParserID, e.Op, e.Err)
}

func (e *ParserError) Unwrap() error {
	return e.Err
}

// ValidationError wraps input validation failures.
type ValidationError struct {
	Field   string
	Value   interface{}
	Message string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("netintel: validation failed for %q: %s", e.Field, e.Message)
}

func (e *ValidationError) Unwrap() error {
	return ErrEntryInvalid
}

// IngestionError wraps log ingestion failures.
type IngestionError struct {
	Source  string
	LogType string
	Err     error
}

func (e *IngestionError) Error() string {
	return fmt.Sprintf("netintel: ingestion failed for %s/%s: %v", e.Source, e.LogType, e.Err)
}

func (e *IngestionError) Unwrap() error {
	return e.Err
}
