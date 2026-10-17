package vclock

import (
	"fmt"
	"sort"
	"sync"
	"time"
)

// VirtualClock provides a thread-safe, mockable, time-virtualized monotonic clock.
type VirtualClock struct {
	mu          sync.RWMutex
	cfg         VirtualTimeConfig
	currentTime time.Time
	tickCount   uint64
	frozen      bool
	rng         *DeterministicRNG
	timers      []*VirtualTimer
	nextTimerID uint64
}

// NewVirtualClock initializes a new VirtualClock with configuration.
func NewVirtualClock(cfg VirtualTimeConfig) *VirtualClock {
	if cfg.Baseline.IsZero() {
		cfg.Baseline = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	}
	if cfg.TickDelta <= 0 {
		cfg.TickDelta = 10 * time.Millisecond
	}
	if cfg.ScaleFactor <= 0 {
		cfg.ScaleFactor = 1.0
	}

	return &VirtualClock{
		cfg:         cfg,
		currentTime: cfg.Baseline,
		tickCount:   0,
		frozen:      cfg.Mode == ModeFrozen,
		rng:         NewDeterministicRNG(cfg.Seed),
		timers:      make([]*VirtualTimer, 0),
	}
}

// Now returns the current virtual time.
func (vc *VirtualClock) Now() time.Time {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.currentTime
}

// TickCount returns the total number of logical ticks elapsed.
func (vc *VirtualClock) TickCount() uint64 {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.tickCount
}

// Tick advances the virtual clock by one TickDelta, executes expired timers, and returns the new time.
func (vc *VirtualClock) Tick() time.Time {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	return vc.advanceInternal(vc.cfg.TickDelta)
}

// Advance advances the virtual clock by an arbitrary duration and triggers any due timers.
func (vc *VirtualClock) Advance(d time.Duration) time.Time {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	return vc.advanceInternal(d)
}

// advanceInternal must be called with vc.mu write-locked.
func (vc *VirtualClock) advanceInternal(d time.Duration) time.Time {
	vc.currentTime = vc.currentTime.Add(d)
	vc.tickCount++

	// Fire any timers that have expired
	var remaining []*VirtualTimer
	var toFire []*VirtualTimer

	for _, t := range vc.timers {
		t.mu.Lock()
		active := t.Active
		t.mu.Unlock()

		if !active {
			continue
		}

		if !vc.currentTime.Before(t.TargetTime) {
			toFire = append(toFire, t)
		} else {
			remaining = append(remaining, t)
		}
	}

	vc.timers = remaining

	// Sort timers to trigger in order of TargetTime
	sort.Slice(toFire, func(i, j int) bool {
		return toFire[i].TargetTime.Before(toFire[j].TargetTime)
	})

	for _, t := range toFire {
		t.mu.Lock()
		t.Active = false
		t.mu.Unlock()

		if t.Callback != nil {
			t.Callback(vc.currentTime, vc.tickCount)
		}
	}

	return vc.currentTime
}

// Sleep simulates an execution pause by advancing virtual time without blocking the OS thread.
func (vc *VirtualClock) Sleep(d time.Duration) {
	vc.Advance(d)
}

// After returns a channel that receives the virtual time after duration d has elapsed.
func (vc *VirtualClock) After(d time.Duration) <-chan time.Time {
	ch := make(chan time.Time, 1)
	vc.NewTimer(d, func(vt time.Time, _ uint64) {
		ch <- vt
	})
	return ch
}

// NewTimer registers a scheduled callback to be fired at Now() + d.
func (vc *VirtualClock) NewTimer(d time.Duration, callback TimerCallback) *VirtualTimer {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	vc.nextTimerID++
	timer := &VirtualTimer{
		ID:         fmt.Sprintf("vtimer-%d", vc.nextTimerID),
		TargetTime: vc.currentTime.Add(d),
		TargetTick: vc.tickCount + uint64(d/vc.cfg.TickDelta),
		Callback:   callback,
		Active:     true,
	}

	vc.timers = append(vc.timers, timer)
	return timer
}

// RNG returns the internal deterministic random number generator.
func (vc *VirtualClock) RNG() *DeterministicRNG {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.rng
}

// UUID returns a deterministic RFC 4122 v4 UUID string based on clock state.
func (vc *VirtualClock) UUID() string {
	vc.mu.RLock()
	defer vc.mu.RUnlock()
	return vc.rng.UUID()
}

// Snapshot creates an immutable state snapshot of the virtual clock for checkpointing.
func (vc *VirtualClock) Snapshot() ClockSnapshot {
	vc.mu.RLock()
	defer vc.mu.RUnlock()

	activeCount := 0
	for _, t := range vc.timers {
		t.mu.Lock()
		if t.Active {
			activeCount++
		}
		t.mu.Unlock()
	}

	s0, s1 := vc.rng.ExportState()
	return ClockSnapshot{
		CurrentTime:  vc.currentTime,
		TickCount:    vc.tickCount,
		RNGStateS0:   s0,
		RNGStateS1:   s1,
		ActiveTimers: activeCount,
	}
}

// Restore restores the virtual clock to a previously captured snapshot.
func (vc *VirtualClock) Restore(s ClockSnapshot) {
	vc.mu.Lock()
	defer vc.mu.Unlock()

	vc.currentTime = s.CurrentTime
	vc.tickCount = s.TickCount
	vc.rng.RestoreState(s.RNGStateS0, s.RNGStateS1)
}
