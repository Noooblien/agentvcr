package vclock

import (
	"fmt"
	"net/http"
	"strconv"
	"time"
)

const (
	// HeaderVirtualTime is the ISO 8601 string of virtual time.
	HeaderVirtualTime = "X-AgentVCR-Virtual-Time"
	// HeaderVirtualTick is the uint64 step counter.
	HeaderVirtualTick = "X-AgentVCR-Virtual-Tick"
	// HeaderVirtualUUID is the deterministic execution trace ID.
	HeaderVirtualUUID = "X-AgentVCR-Virtual-UUID"
)

// TimeInterceptor injects deterministic virtual time headers into HTTP workflows.
type TimeInterceptor struct {
	clock *VirtualClock
}

// NewTimeInterceptor creates an HTTP middleware interceptor bound to a VirtualClock.
func NewTimeInterceptor(clock *VirtualClock) *TimeInterceptor {
	return &TimeInterceptor{clock: clock}
}

// InjectHeaders decorates an outgoing HTTP request with virtual clock telemetry.
func (ti *TimeInterceptor) InjectHeaders(req *http.Request) {
	now := ti.clock.Now()
	tick := ti.clock.TickCount()
	uuid := ti.clock.UUID()

	req.Header.Set(HeaderVirtualTime, now.Format(time.RFC3339Nano))
	req.Header.Set(HeaderVirtualTick, strconv.FormatUint(tick, 10))
	req.Header.Set(HeaderVirtualUUID, uuid)
}

// Middleware wraps an http.Handler to automatically stamp responses and step virtual time on requests.
func (ti *TimeInterceptor) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Step clock forward on inbound request
		ti.clock.Tick()

		now := ti.clock.Now()
		tick := ti.clock.TickCount()
		uuid := ti.clock.UUID()

		w.Header().Set(HeaderVirtualTime, now.Format(time.RFC3339Nano))
		w.Header().Set(HeaderVirtualTick, fmt.Sprintf("%d", tick))
		w.Header().Set(HeaderVirtualUUID, uuid)

		next.ServeHTTP(w, r)
	})
}
