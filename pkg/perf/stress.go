package perf

import (
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// StressRunner executes multi-goroutine concurrent stress testing on telemetry capture and interception.
type StressRunner struct {
	concurrency int
	iterations  int
}

// NewStressRunner creates a new stress runner with the designated worker count.
func NewStressRunner(concurrency, iterations int) *StressRunner {
	if concurrency <= 0 {
		concurrency = 4
	}
	if iterations <= 0 {
		iterations = 100
	}
	return &StressRunner{
		concurrency: concurrency,
		iterations:  iterations,
	}
}

// StressResult holds the metrics gathered during a multi-threaded stress run.
type StressResult struct {
	Concurrency      int                `json:"concurrency"`
	TotalOperations  int                `json:"total_operations"`
	ErrorCount       int64              `json:"error_count"`
	DurationMs       float64            `json:"duration_ms"`
	ThroughputOpsSec float64            `json:"throughput_ops_sec"`
	Percentiles      LatencyPercentiles `json:"percentiles"`
}

// RunConcurrentFlightRecorder simulates concurrent agent workers emitting telemetry events in parallel.
func (sr *StressRunner) RunConcurrentFlightRecorder() (*StressResult, error) {
	var totalErrors int64
	var wg sync.WaitGroup

	samplesPerWorker := sr.iterations / sr.concurrency
	if samplesPerWorker <= 0 {
		samplesPerWorker = 1
	}

	allDurations := make([][]time.Duration, sr.concurrency)
	for i := range allDurations {
		allDurations[i] = make([]time.Duration, samplesPerWorker)
	}

	start := time.Now()

	for workerID := 0; workerID < sr.concurrency; workerID++ {
		wg.Add(1)
		go func(wid int) {
			defer wg.Done()
			for op := 0; op < samplesPerWorker; op++ {
				t0 := time.Now()
				ev := trace.TraceEvent{
					Index:      op,
					Timestamp:  t0,
					Type:       trace.EventToolCall,
					DurationMs: 5,
					Payload: map[string]interface{}{
						"worker_id": wid,
						"op":        op,
						"tool":      "search_code",
						"query":     fmt.Sprintf("worker-%d-query-%d", wid, op),
					},
				}
				payloadBytes, _ := json.Marshal(ev.Payload)
				chk := trace.ComputeChecksum("genesis", op, ev.Type, payloadBytes)
				if chk == "" {
					atomic.AddInt64(&totalErrors, 1)
				}
				allDurations[wid][op] = time.Since(t0)
			}
		}(workerID)
	}

	wg.Wait()
	totalDuration := time.Since(start)

	var flattenedDurations []time.Duration
	for _, workerDurs := range allDurations {
		flattenedDurations = append(flattenedDurations, workerDurs...)
	}

	pct := CalculatePercentiles(flattenedDurations, totalDuration, 0, 0)

	return &StressResult{
		Concurrency:      sr.concurrency,
		TotalOperations:  len(flattenedDurations),
		ErrorCount:       totalErrors,
		DurationMs:       float64(totalDuration.Milliseconds()),
		ThroughputOpsSec: pct.OpsPerSec,
		Percentiles:      pct,
	}, nil
}
