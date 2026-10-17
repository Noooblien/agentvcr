package gateway

import (
	"fmt"
	"sync"
	"time"
)

// CircuitBreaker guards an individual upstream provider against cascading failures.
type CircuitBreaker struct {
	mu                  sync.RWMutex
	providerID          string
	config              CircuitBreakerConfig
	state               CircuitState
	consecutiveFailures int
	consecutiveSuccess  int
	lastStateChange     time.Time
	lastError           string
	rollingSamples      []bool // true for success, false for error
	totalRequests       int64
	successfulRequests  int64
	failedRequests      int64
}

// NewCircuitBreaker initializes a circuit breaker in the CLOSED state.
func NewCircuitBreaker(providerID string, config CircuitBreakerConfig) *CircuitBreaker {
	if config.FailureThreshold <= 0 {
		config.FailureThreshold = 3
	}
	if config.RollingWindowSize <= 0 {
		config.RollingWindowSize = 10
	}
	if config.CooldownDuration <= 0 {
		config.CooldownDuration = 5 * time.Second
	}
	if config.HalfOpenProbes <= 0 {
		config.HalfOpenProbes = 2
	}
	if config.ErrorRateThreshold <= 0 {
		config.ErrorRateThreshold = 0.5
	}

	return &CircuitBreaker{
		providerID:      providerID,
		config:          config,
		state:           CircuitClosed,
		lastStateChange: time.Now(),
		rollingSamples:  make([]bool, 0, config.RollingWindowSize),
	}
}

// Allow checks if a request is permitted to proceed to the upstream provider.
func (cb *CircuitBreaker) Allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	now := time.Now()

	switch cb.state {
	case CircuitClosed:
		return true

	case CircuitOpen:
		// Check if cooldown period has elapsed to permit half-open probe
		if now.Sub(cb.lastStateChange) >= cb.config.CooldownDuration {
			cb.state = CircuitHalfOpen
			cb.lastStateChange = now
			cb.consecutiveSuccess = 0
			return true
		}
		return false

	case CircuitHalfOpen:
		// In HALF_OPEN, permit canary probe
		return true

	default:
		return true
	}
}

// RecordSuccess records a completed valid response from the upstream.
func (cb *CircuitBreaker) RecordSuccess() {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++
	cb.successfulRequests++
	cb.appendSample(true)

	if cb.state == CircuitHalfOpen {
		cb.consecutiveSuccess++
		if cb.consecutiveSuccess >= cb.config.HalfOpenProbes {
			// Sufficient successful probes; heal circuit back to CLOSED
			cb.state = CircuitClosed
			cb.lastStateChange = time.Now()
			cb.consecutiveFailures = 0
			cb.consecutiveSuccess = 0
			cb.lastError = ""
		}
	} else if cb.state == CircuitClosed {
		cb.consecutiveFailures = 0
	}
}

// RecordFailure registers an upstream timeout, 429 rate limit, 5xx error, or network drop.
func (cb *CircuitBreaker) RecordFailure(err error) {
	cb.mu.Lock()
	defer cb.mu.Unlock()

	cb.totalRequests++
	cb.failedRequests++
	cb.consecutiveFailures++
	cb.consecutiveSuccess = 0
	if err != nil {
		cb.lastError = err.Error()
	}
	cb.appendSample(false)

	if cb.state == CircuitHalfOpen {
		// Single probe failure trips circuit back to OPEN
		cb.state = CircuitOpen
		cb.lastStateChange = time.Now()
		return
	}

	if cb.state == CircuitClosed {
		// Condition 1: Consecutive failure threshold reached
		if cb.consecutiveFailures >= cb.config.FailureThreshold {
			cb.tripOpen()
			return
		}

		// Condition 2: Rolling window error rate threshold exceeded
		if len(cb.rollingSamples) >= 3 {
			errorCount := 0
			for _, success := range cb.rollingSamples {
				if !success {
					errorCount++
				}
			}
			rate := float64(errorCount) / float64(len(cb.rollingSamples))
			if rate >= cb.config.ErrorRateThreshold {
				cb.tripOpen()
				return
			}
		}
	}
}

func (cb *CircuitBreaker) tripOpen() {
	cb.state = CircuitOpen
	cb.lastStateChange = time.Now()
}

func (cb *CircuitBreaker) appendSample(success bool) {
	if len(cb.rollingSamples) >= cb.config.RollingWindowSize {
		cb.rollingSamples = cb.rollingSamples[1:]
	}
	cb.rollingSamples = append(cb.rollingSamples, success)
}

// ForceState overrides circuit breaker state (useful for testing and simulated chaos).
func (cb *CircuitBreaker) ForceState(state CircuitState) {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	cb.state = state
	cb.lastStateChange = time.Now()
}

// State returns the current circuit state safely.
func (cb *CircuitBreaker) State() CircuitState {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state
}

// Snapshot returns live telemetry for the circuit breaker.
func (cb *CircuitBreaker) Snapshot() (CircuitState, int, int64, int64, string) {
	cb.mu.RLock()
	defer cb.mu.RUnlock()
	return cb.state, cb.consecutiveFailures, cb.successfulRequests, cb.failedRequests, cb.lastError
}

// String returns formatted summary string.
func (cb *CircuitBreaker) String() string {
	state, fails, succ, errs, _ := cb.Snapshot()
	return fmt.Sprintf("Circuit[%s: State=%s, ConsecutiveFailures=%d, Successes=%d, Errors=%d]",
		cb.providerID, state, fails, succ, errs)
}
