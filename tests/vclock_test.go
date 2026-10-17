package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/vclock"
)

func TestVirtualClock_BaselineAndMonotonicTick(t *testing.T) {
	baseline := time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	cfg := vclock.VirtualTimeConfig{
		Baseline:  baseline,
		TickDelta: 50 * time.Millisecond,
		Mode:      vclock.ModeStepMonotonic,
		Seed:      42,
	}

	clock := vclock.NewVirtualClock(cfg)
	if !clock.Now().Equal(baseline) {
		t.Fatalf("expected baseline %v, got %v", baseline, clock.Now())
	}
	if clock.TickCount() != 0 {
		t.Fatalf("expected initial tick count 0, got %d", clock.TickCount())
	}

	// Advance by 1 tick
	t1 := clock.Tick()
	expectedT1 := baseline.Add(50 * time.Millisecond)
	if !t1.Equal(expectedT1) {
		t.Fatalf("expected t1 %v, got %v", expectedT1, t1)
	}
	if clock.TickCount() != 1 {
		t.Fatalf("expected tick count 1, got %d", clock.TickCount())
	}

	// Advance by arbitrary duration
	clock.Advance(200 * time.Millisecond)
	expectedT2 := expectedT1.Add(200 * time.Millisecond)
	if !clock.Now().Equal(expectedT2) {
		t.Fatalf("expected time %v, got %v", expectedT2, clock.Now())
	}
	if clock.TickCount() != 2 {
		t.Fatalf("expected tick count 2, got %d", clock.TickCount())
	}
}

func TestVirtualClock_TimersAndCallbacks(t *testing.T) {
	baseline := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	cfg := vclock.DefaultConfig()
	cfg.Baseline = baseline
	cfg.TickDelta = 10 * time.Millisecond

	clock := vclock.NewVirtualClock(cfg)

	fired1 := false
	fired2 := false
	firedCancelled := false

	clock.NewTimer(50*time.Millisecond, func(vt time.Time, tick uint64) {
		fired1 = true
	})

	tCancel := clock.NewTimer(80*time.Millisecond, func(vt time.Time, tick uint64) {
		firedCancelled = true
	})

	clock.NewTimer(100*time.Millisecond, func(vt time.Time, tick uint64) {
		fired2 = true
	})

	// Cancel the intermediate timer
	if !tCancel.Stop() {
		t.Fatalf("expected timer stop to succeed")
	}

	// Advance clock by 40ms -> neither should have fired
	clock.Advance(40 * time.Millisecond)
	if fired1 || fired2 || firedCancelled {
		t.Fatalf("no timer should have fired at 40ms")
	}

	// Advance clock by 20ms (total 60ms) -> timer 1 should fire
	clock.Advance(20 * time.Millisecond)
	if !fired1 {
		t.Fatalf("timer 1 should have fired at 60ms")
	}
	if fired2 || firedCancelled {
		t.Fatalf("timer 2 and cancelled timer should not have fired")
	}

	// Advance clock by 50ms (total 110ms) -> timer 2 should fire
	clock.Advance(50 * time.Millisecond)
	if !fired2 {
		t.Fatalf("timer 2 should have fired at 110ms")
	}
	if firedCancelled {
		t.Fatalf("cancelled timer should never fire")
	}
}

func TestVirtualClock_SleepAndAfter(t *testing.T) {
	clock := vclock.NewVirtualClock(vclock.DefaultConfig())
	start := clock.Now()

	ch := clock.After(100 * time.Millisecond)

	// In virtual time, Sleep advances the clock immediately without wall-clock blocking
	clock.Sleep(100 * time.Millisecond)

	select {
	case triggerTime := <-ch:
		if !triggerTime.Equal(start.Add(100 * time.Millisecond)) {
			t.Fatalf("unexpected trigger time %v", triggerTime)
		}
	default:
		t.Fatalf("expected After channel to have triggered")
	}
}

func TestDeterministicRNG_Reproducibility(t *testing.T) {
	rng1 := vclock.NewDeterministicRNG(987654321)
	rng2 := vclock.NewDeterministicRNG(987654321)

	// Ensure sequence of 100 uint64 matches exactly
	for i := 0; i < 100; i++ {
		v1 := rng1.Uint64()
		v2 := rng2.Uint64()
		if v1 != v2 {
			t.Fatalf("mismatch at index %d: %d vs %d", i, v1, v2)
		}
	}

	// Float64 test
	f1 := rng1.Float64()
	f2 := rng2.Float64()
	if f1 != f2 || f1 < 0.0 || f1 >= 1.0 {
		t.Fatalf("float64 mismatch or out of range: %f vs %f", f1, f2)
	}

	// Intn test
	n1 := rng1.Intn(50)
	n2 := rng2.Intn(50)
	if n1 != n2 || n1 < 0 || n1 >= 50 {
		t.Fatalf("intn mismatch or out of range: %d vs %d", n1, n2)
	}
}

func TestDeterministicRNG_UUIDFormatAndReproducibility(t *testing.T) {
	rng1 := vclock.NewDeterministicRNG(1337)
	rng2 := vclock.NewDeterministicRNG(1337)

	for i := 0; i < 20; i++ {
		u1 := rng1.UUID()
		u2 := rng2.UUID()
		if u1 != u2 {
			t.Fatalf("uuid mismatch at %d: %s vs %s", i, u1, u2)
		}
		if len(u1) != 36 {
			t.Fatalf("invalid uuid length: %s", u1)
		}
		// Check RFC 4122 v4 version digit (at offset 14)
		if u1[14] != '4' {
			t.Fatalf("expected uuid v4, got %c in %s", u1[14], u1)
		}
	}
}

func TestDeterministicRNG_ForkIndependence(t *testing.T) {
	parent := vclock.NewDeterministicRNG(777)
	childA := parent.Fork("agent-alpha")
	childB := parent.Fork("agent-beta")

	valA := childA.Uint64()
	valB := childB.Uint64()

	if valA == valB {
		t.Fatalf("forked child RNGs with different domains should not collide")
	}

	// Verify reproducible fork
	parent2 := vclock.NewDeterministicRNG(777)
	childA2 := parent2.Fork("agent-alpha")
	if childA2.Uint64() != valA {
		t.Fatalf("forked child RNG stream is not reproducible from parent seed")
	}
}

func TestDeterministicRNG_ReadByteStream(t *testing.T) {
	rng := vclock.NewDeterministicRNG(555)
	buf1 := make([]byte, 32)
	buf2 := make([]byte, 32)

	n, err := rng.Read(buf1)
	if err != nil || n != 32 {
		t.Fatalf("read failed: n=%d, err=%v", n, err)
	}

	rng2 := vclock.NewDeterministicRNG(555)
	_, _ = io.ReadFull(rng2, buf2)

	for i := 0; i < 32; i++ {
		if buf1[i] != buf2[i] {
			t.Fatalf("byte mismatch at %d", i)
		}
	}
}

func TestVirtualClock_SnapshotAndRestore(t *testing.T) {
	cfg := vclock.DefaultConfig()
	clock := vclock.NewVirtualClock(cfg)

	clock.Advance(500 * time.Millisecond)
	_ = clock.UUID()
	_ = clock.UUID()

	snapshot := clock.Snapshot()
	if snapshot.TickCount != 1 {
		t.Fatalf("expected tick count 1, got %d", snapshot.TickCount)
	}

	// Advance further
	clock.Advance(1000 * time.Millisecond)
	nextUUID := clock.UUID()

	// Restore
	clock.Restore(snapshot)
	if clock.TickCount() != snapshot.TickCount {
		t.Fatalf("expected restored tick %d, got %d", snapshot.TickCount, clock.TickCount())
	}
	if !clock.Now().Equal(snapshot.CurrentTime) {
		t.Fatalf("expected restored time %v, got %v", snapshot.CurrentTime, clock.Now())
	}

	// Post-restore UUID sequence must match the exact pre-advance sequence
	restoredUUID := clock.UUID()
	if restoredUUID != nextUUID {
		t.Fatalf("expected restored UUID %s, got %s", nextUUID, restoredUUID)
	}
}

func TestTimeInterceptor_HTTPIntegration(t *testing.T) {
	clock := vclock.NewVirtualClock(vclock.DefaultConfig())
	interceptor := vclock.NewTimeInterceptor(clock)

	// Test outbound request header decoration
	req, _ := http.NewRequest(http.MethodGet, "https://api.openai.com/v1/chat/completions", nil)
	interceptor.InjectHeaders(req)

	if req.Header.Get(vclock.HeaderVirtualTime) == "" {
		t.Fatalf("missing virtual time header")
	}
	if req.Header.Get(vclock.HeaderVirtualTick) != "0" {
		t.Fatalf("expected virtual tick 0, got %s", req.Header.Get(vclock.HeaderVirtualTick))
	}
	if req.Header.Get(vclock.HeaderVirtualUUID) == "" {
		t.Fatalf("missing virtual uuid header")
	}

	// Test inbound HTTP middleware
	handler := interceptor.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))

	rec := httptest.NewRecorder()
	httpReq, _ := http.NewRequest(http.MethodPost, "/v1/agent/action", nil)
	handler.ServeHTTP(rec, httpReq)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if rec.Header().Get(vclock.HeaderVirtualTick) != "1" {
		t.Fatalf("expected virtual tick 1 in response, got %s", rec.Header().Get(vclock.HeaderVirtualTick))
	}
}
