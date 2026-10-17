package vclock

import (
	"sync"
	"time"
)

// ClockMode specifies how virtual time advances.
type ClockMode string

const (
	// ModeFrozen keeps virtual time completely static until explicit Tick() or Advance().
	ModeFrozen ClockMode = "frozen"
	// ModeStepMonotonic increments virtual time by a fixed TickDelta on every step/operation.
	ModeStepMonotonic ClockMode = "step_monotonic"
	// ModeScaled accelerates or decelerates virtual time relative to wall-clock.
	ModeScaled ClockMode = "scaled"
)

// VirtualTimeConfig configures the deterministic virtual clock and hypervisor.
type VirtualTimeConfig struct {
	// Baseline is the initial fixed timestamp (defaults to 2026-01-01 00:00:00 UTC).
	Baseline time.Time `json:"baseline"`
	// TickDelta is the time increment per logical step (default 10ms).
	TickDelta time.Duration `json:"tick_delta"`
	// Mode determines virtual time progression behavior.
	Mode ClockMode `json:"mode"`
	// ScaleFactor scales time in ModeScaled (e.g. 2.0 = 2x speed, 0.5 = half speed).
	ScaleFactor float64 `json:"scale_factor"`
	// Seed is the 64-bit seed used to initialize deterministic PRNG and UUID generators.
	Seed int64 `json:"seed"`
}

// DefaultConfig returns recommended configuration for deterministic agent execution.
func DefaultConfig() VirtualTimeConfig {
	return VirtualTimeConfig{
		Baseline:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		TickDelta:   10 * time.Millisecond,
		Mode:        ModeStepMonotonic,
		ScaleFactor: 1.0,
		Seed:        1337420,
	}
}

// ClockSnapshot represents an immutable checkpoint of virtual clock state for time-travel restoration.
type ClockSnapshot struct {
	CurrentTime  time.Time `json:"current_time"`
	TickCount    uint64    `json:"tick_count"`
	RNGStateS0   uint64    `json:"rng_state_s0"`
	RNGStateS1   uint64    `json:"rng_state_s1"`
	ActiveTimers int       `json:"active_timers"`
}

// TimerCallback is executed when a virtual timer expires.
type TimerCallback func(virtualTime time.Time, tick uint64)

// VirtualTimer represents an active scheduled timer bound to virtual clock ticks.
type VirtualTimer struct {
	ID         string
	TargetTime time.Time
	TargetTick uint64
	Callback   TimerCallback
	Active     bool
	mu         sync.Mutex
}

// Stop cancels the virtual timer.
func (vt *VirtualTimer) Stop() bool {
	vt.mu.Lock()
	defer vt.mu.Unlock()
	if !vt.Active {
		return false
	}
	vt.Active = false
	return true
}
