package antivirus

import (
	stderrors "errors"

	"axiomnizam.bitbd.net/axiomnizam/internal/errors"
)

// Antivirus-specific sentinel errors.
var (
	// ErrEngineNotRunning indicates the AV engine is not running.
	ErrEngineNotRunning = stderrors.New("antivirus engine is not running")

	// ErrScanTimeout indicates a scan operation timed out.
	ErrScanTimeout = stderrors.New("scan timed out")

	// ErrSignatureDBNotLoaded indicates the signature database is not loaded.
	ErrSignatureDBNotLoaded = stderrors.New("signature database not loaded")

	// ErrMalwareDetected indicates malware was found.
	ErrMalwareDetected = stderrors.New("malware detected")
)

// IsNotFound checks if an error is a not-found error.
func IsNotFound(err error) bool {
	return errors.Is(err, errors.ErrNotFound)
}

// IsTimeout checks if an error is a timeout error.
func IsTimeout(err error) bool {
	return errors.Is(err, errors.ErrTimeout)
}
