package loopbreaker

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func TestPeriod1SelfLoop(t *testing.T) {
	cfg := DefaultLoopConfig()
	breaker := NewLoopBreaker(cfg)

	// Step 0: Initial normal call
	d0 := breaker.RecordStep(StepRecord{
		StepIndex:     0,
		ToolName:      "init",
		ArgumentsJSON: `{"k":"v"}`,
	})
	if d0.Detected {
		t.Fatalf("Step 0 should not trigger cycle detection")
	}

	// Steps 1, 2, 3: Same failing tool call
	var lastDetection *CycleDetection
	for i := 1; i <= 3; i++ {
		lastDetection = breaker.RecordStep(StepRecord{
			StepIndex:      i,
			ToolName:       "execute_query",
			ArgumentsJSON:  `{"query":"SELECT * FORM users"}`,
			IsError:        true,
			TokensConsumed: 200,
		})
	}

	if !lastDetection.Detected {
		t.Fatalf("Expected Period 1 cycle detected by step 3")
	}
	if lastDetection.Period != 1 {
		t.Errorf("Expected Period 1, got %d", lastDetection.Period)
	}
	if lastDetection.RecurrenceCount < 2 {
		t.Errorf("Expected at least 2 recurrences, got %d", lastDetection.RecurrenceCount)
	}
	if lastDetection.Action == ActionAllow {
		t.Errorf("Expected intervention action, got ActionAllow")
	}
}

func TestPeriod2BipartiteOscillation(t *testing.T) {
	cfg := DefaultLoopConfig()
	breaker := NewLoopBreaker(cfg)

	var lastDetection *CycleDetection
	for i := 0; i < 6; i++ {
		tool := "tool_a"
		if i%2 == 1 {
			tool = "tool_b"
		}
		lastDetection = breaker.RecordStep(StepRecord{
			StepIndex:      i,
			ToolName:       tool,
			ArgumentsJSON:  `{"flag": true}`,
			TokensConsumed: 150,
		})
	}

	if !lastDetection.Detected {
		t.Fatalf("Expected Period 2 oscillation detected by step 5")
	}
	if lastDetection.Period != 2 {
		t.Errorf("Expected Period 2, got %d", lastDetection.Period)
	}
	if lastDetection.Pattern != PatternBipartiteOscillation {
		t.Errorf("Expected PatternBipartiteOscillation, got %s", lastDetection.Pattern)
	}
}

func TestPeriod3CircularDeadlock(t *testing.T) {
	cfg := DefaultLoopConfig()
	breaker := NewLoopBreaker(cfg)

	tools := []string{"search", "fetch", "parse"}
	var lastDetection *CycleDetection
	stepIdx := 0

	for cycle := 0; cycle < 3; cycle++ {
		for _, tool := range tools {
			lastDetection = breaker.RecordStep(StepRecord{
				StepIndex:      stepIdx,
				ToolName:       tool,
				ArgumentsJSON:  `{"target": "https://example.com"}`,
				TokensConsumed: 250,
			})
			stepIdx++
		}
	}

	if !lastDetection.Detected {
		t.Fatalf("Expected Period 3 cycle detected")
	}
	if lastDetection.Period != 3 {
		t.Errorf("Expected Period 3, got %d", lastDetection.Period)
	}
	if lastDetection.Pattern != PatternMultiStepCycle {
		t.Errorf("Expected PatternMultiStepCycle, got %s", lastDetection.Pattern)
	}
}

func TestLegitimateBatchNoFalsePositives(t *testing.T) {
	cfg := DefaultLoopConfig()
	breaker := NewLoopBreaker(cfg)

	files := []string{"file_a.txt", "file_b.txt", "file_c.txt", "file_d.txt", "file_e.txt"}
	for i, f := range files {
		d := breaker.RecordStep(StepRecord{
			StepIndex:      i,
			ToolName:       "read_file",
			ArgumentsJSON:  `{"path": "` + f + `"}`,
			IsError:        false,
			TokensConsumed: 100,
		})
		if d.Detected {
			t.Fatalf("False positive: legitimate batch reading file %s was falsely flagged as cycle", f)
		}
	}
}

func TestJaccardSimilarityCalculation(t *testing.T) {
	s1 := `{"query": "SELECT * FROM users;"}`
	s2 := `{"query": "SELECT * FROM users;"}`
	sim1 := CalculateSimilarity(s1, s2)
	if sim1 != 1.0 {
		t.Errorf("Identical strings must have similarity 1.0, got %.3f", sim1)
	}

	s3 := `{"completely": "different"}`
	sim2 := CalculateSimilarity(s1, s3)
	if sim2 > 0.40 {
		t.Errorf("Disjoint strings should have low similarity, got %.3f", sim2)
	}
}

func TestSteeringSynthesis(t *testing.T) {
	prompt := SynthesizeSteeringPrompt(PatternSelfLoop, 1, []string{"execute_sql"}, "syntax error at FORM", 0.98)
	if !strings.Contains(prompt, "CONSTITUTIONAL INTERVENTION") {
		t.Errorf("Prompt missing CONSTITUTIONAL INTERVENTION header")
	}
	if !strings.Contains(prompt, "execute_sql") {
		t.Errorf("Prompt missing offending tool name")
	}
	if !strings.Contains(prompt, "MANDATORY DIRECTIVE") {
		t.Errorf("Prompt missing MANDATORY DIRECTIVE section")
	}
}

func TestConcurrency(t *testing.T) {
	cfg := DefaultLoopConfig()
	breaker := NewLoopBreaker(cfg)

	var wg sync.WaitGroup
	workers := 50
	wg.Add(workers)

	for i := 0; i < workers; i++ {
		go func(id int) {
			defer wg.Done()
			breaker.RecordStep(StepRecord{
				StepIndex:      id,
				Timestamp:      time.Now(),
				ToolName:       "tool_worker",
				ArgumentsJSON:  `{"worker_id": 1}`,
				TokensConsumed: 100,
			})
		}(i)
	}
	wg.Wait()
}

func TestDemoSuiteExecution(t *testing.T) {
	cfg := DefaultLoopConfig()
	summary := RunDemoSuite(cfg)

	if summary.TotalStepsEvaluated <= 0 {
		t.Fatalf("Expected steps evaluated, got %d", summary.TotalStepsEvaluated)
	}
	if summary.CyclesDetected == 0 {
		t.Fatalf("Expected at least one cycle detected in demo suite")
	}
	if summary.TotalTokensSaved == 0 {
		t.Fatalf("Expected tokens saved > 0")
	}
	if summary.EstimatedSavingsUSD <= 0 {
		t.Fatalf("Expected estimated cost savings > 0")
	}

	scorecard := RenderLoopScorecard(summary)
	if !strings.Contains(scorecard, "AGENTVCR REASONING CYCLE & LOOP BREAKER") {
		t.Fatalf("Scorecard missing expected title")
	}

	mermaid := RenderLoopMermaid(summary.Detections)
	if !strings.Contains(mermaid, "stateDiagram-v2") {
		t.Fatalf("Mermaid diagram missing stateDiagram-v2 tag")
	}

	markdown := RenderLoopMarkdownReport(summary)
	if !strings.Contains(markdown, "Executive Summary") {
		t.Fatalf("Markdown report missing Executive Summary")
	}
}
