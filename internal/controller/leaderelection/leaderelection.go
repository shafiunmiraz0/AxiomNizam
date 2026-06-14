// Package leaderelection implements the classic
// acquire-renew-or-observe election loop on top of a resourcelock.
// It is the same state machine client-go runs: a single goroutine
// tries to acquire the lock; once held, it renews on an interval; if
// renewal fails, the caller's OnStoppedLeading is invoked and the
// loop returns.
package leaderelection

import (
	"context"
	"errors"
	"fmt"
	"time"

	"axiomnizam.bitbd.net/axiomnizam/internal/controller/leaderelection/resourcelock"
)

// Config bundles the election knobs.  Zero defaults mirror upstream
// client-go: LeaseDuration=15s, RenewDeadline=10s, RetryPeriod=2s.
type Config struct {
	// Lock is the resourcelock to contend on.
	Lock resourcelock.Interface
	// LeaseDuration is how long a holder owns the lock without
	// renewal before contenders may steal it.
	LeaseDuration time.Duration
	// RenewDeadline is the budget a holder has to successfully renew
	// each round; missing it forfeits leadership.
	RenewDeadline time.Duration
	// RetryPeriod is the gap between renewal attempts.
	RetryPeriod time.Duration

	// Callbacks fire on leadership transitions.
	OnStartedLeading func(context.Context)
	OnStoppedLeading func()
	OnNewLeader      func(identity string)
}

// applyDefaults fills in the zero-value knobs.
func (c *Config) applyDefaults() {
	if c.LeaseDuration == 0 {
		c.LeaseDuration = 15 * time.Second
	}
	if c.RenewDeadline == 0 {
		c.RenewDeadline = 10 * time.Second
	}
	if c.RetryPeriod == 0 {
		c.RetryPeriod = 2 * time.Second
	}
}

// validate enforces the constraint LeaseDuration > RenewDeadline >
// RetryPeriod — without it, a slow renewer can hold a lock it has
// already effectively lost.
func (c *Config) validate() error {
	if c.Lock == nil {
		return errors.New("leaderelection: nil Lock")
	}
	if c.LeaseDuration <= c.RenewDeadline {
		return fmt.Errorf("leaderelection: LeaseDuration (%s) must be > RenewDeadline (%s)", c.LeaseDuration, c.RenewDeadline)
	}
	if c.RenewDeadline <= c.RetryPeriod {
		return fmt.Errorf("leaderelection: RenewDeadline (%s) must be > RetryPeriod (%s)", c.RenewDeadline, c.RetryPeriod)
	}
	return nil
}

// Run blocks until ctx is cancelled or the election permanently
// fails.  The election loop runs in the caller's goroutine; wrap the
// call in `go` if a non-blocking contender is desired.
func Run(ctx context.Context, cfg Config) error {
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return err
	}
	for ctx.Err() == nil {
		acquired := acquire(ctx, cfg)
		if !acquired {
			// Another contender holds the lock; wait RetryPeriod then probe again.
			sleep(ctx, cfg.RetryPeriod)
			continue
		}
		// Leadership acquired — spin up OnStartedLeading and begin renewing.
		leaderCtx, cancel := context.WithCancel(ctx)
		if cfg.OnStartedLeading != nil {
			go cfg.OnStartedLeading(leaderCtx)
		}
		renew(leaderCtx, cfg)
		cancel()
		if cfg.OnStoppedLeading != nil {
			cfg.OnStoppedLeading()
		}
	}
	return ctx.Err()
}

// acquire attempts to create or steal the lock.  Returns true when
// this contender now holds it.
func acquire(ctx context.Context, cfg Config) bool {
	now := time.Now()
	cur, err := cfg.Lock.Get(ctx)
	switch {
	case errors.Is(err, resourcelock.ErrNotFound):
		rec := resourcelock.LeaderElectionRecord{
			HolderIdentity:       cfg.Lock.Identity(),
			LeaseDurationSeconds: int(cfg.LeaseDuration.Seconds()),
			AcquireTime:          now,
			RenewTime:            now,
			LeaderTransitions:    1,
		}
		if err := cfg.Lock.Create(ctx, rec); err == nil {
			notifyLeader(cfg, rec.HolderIdentity)
			return true
		}
		return false
	case err != nil:
		return false
	}
	if cur.HolderIdentity == cfg.Lock.Identity() {
		// Already ours — treat as reacquire by updating RenewTime.
		cur.RenewTime = now
		_ = cfg.Lock.Update(ctx, *cur)
		return true
	}
	if !cur.IsExpired(now) {
		notifyLeader(cfg, cur.HolderIdentity)
		return false
	}
	// Lock is stale — steal.
	cur.HolderIdentity = cfg.Lock.Identity()
	cur.LeaseDurationSeconds = int(cfg.LeaseDuration.Seconds())
	cur.AcquireTime = now
	cur.RenewTime = now
	cur.LeaderTransitions++
	if err := cfg.Lock.Update(ctx, *cur); err != nil {
		return false
	}
	notifyLeader(cfg, cur.HolderIdentity)
	return true
}

// renew holds the lock until renewal fails or ctx is cancelled.
func renew(ctx context.Context, cfg Config) {
	ticker := time.NewTicker(cfg.RetryPeriod)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		renewCtx, cancel := context.WithTimeout(ctx, cfg.RenewDeadline)
		ok := attemptRenew(renewCtx, cfg)
		cancel()
		if !ok {
			return
		}
	}
}

// attemptRenew refreshes RenewTime.
func attemptRenew(ctx context.Context, cfg Config) bool {
	cur, err := cfg.Lock.Get(ctx)
	if err != nil {
		return false
	}
	if cur.HolderIdentity != cfg.Lock.Identity() {
		// We lost the lock — contender stole it.
		return false
	}
	cur.RenewTime = time.Now()
	return cfg.Lock.Update(ctx, *cur) == nil
}

// notifyLeader invokes OnNewLeader if set.
func notifyLeader(cfg Config, id string) {
	if cfg.OnNewLeader != nil {
		cfg.OnNewLeader(id)
	}
}

// sleep is a cancel-aware time.Sleep.
func sleep(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
}
