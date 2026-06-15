package antivirus

import (
	stderrors "errors"
)

// ErrInvalidConfig indicates a nil or invalid configuration was provided.
var ErrInvalidConfig = stderrors.New("invalid configuration")

// UpdateConfig atomically replaces the engine's configuration.
// The new config is validated before application. Returns validation
// warnings (if any) and an error if validation fails fatally.
//
// Thread-safe: concurrent Scan() calls will see the new config
// on their next read.
func (e *Engine) UpdateConfig(newCfg *Config) ([]string, error) {
	if newCfg == nil {
		return nil, ErrInvalidConfig
	}

	warnings := newCfg.Validate()

	e.configMu.Lock()
	e.cfg = newCfg
	e.configMu.Unlock()

	return warnings, nil
}

// ConfigSnapshot returns a copy of the current configuration.
// The returned pointer is safe to hold without locking.
func (e *Engine) ConfigSnapshot() *Config {
	e.configMu.RLock()
	defer e.configMu.RUnlock()
	snap := *e.cfg
	return &snap
}
