package chaos

import (
	"fmt"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// DemoScenario wraps a synthetic session with a specific chaos configuration.
type DemoScenario struct {
	Name        string
	Description string
	Session     *trace.Session
	Config      ChaosExperimentConfig
}

// GenerateDemoScenarios returns 5 canonical chaos evaluation workloads.
func GenerateDemoScenarios() []DemoScenario {
	baseSession := generateBaseSession("agent-coder-prod", 8)

	return []DemoScenario{
		{
			Name:        "Nominal Baseline (Control Group)",
			Description: "Deterministic execution without fault injection (control group baseline)",
			Session:     baseSession,
			Config: ChaosExperimentConfig{
				ExperimentName: "Nominal Baseline",
				Rules:          []FaultRule{},
				ChaosSeed:      42,
				StrictMode:     true,
			},
		},
		{
			Name:        "Transient Rate Limit Storm (HTTP 429)",
			Description: "Downstream API returns 429 rate limits; agent must backoff and retry",
			Session:     baseSession,
			Config: ChaosExperimentConfig{
				ExperimentName: "Rate Limit Storm",
				Rules: []FaultRule{
					{
						ID:             "rule-rate-limit",
						TargetTool:     "web_search",
						Type:           FaultRateLimit,
						Probability:    1.0,
						MaxOccurrences: 2,
					},
				},
				ChaosSeed:  42,
				StrictMode: false,
			},
		},
		{
			Name:        "Downstream Tool Timeout & Fallback",
			Description: "Tool execution hangs for 3,000ms; agent must timeout and dispatch fallback tool",
			Session:     baseSession,
			Config: ChaosExperimentConfig{
				ExperimentName: "Tool Timeout Fallback",
				Rules: []FaultRule{
					{
						ID:             "rule-tool-timeout",
						TargetTool:     "bash_exec",
						Type:           FaultToolTimeout,
						InjectedDelay:  3000 * time.Millisecond,
						Probability:    1.0,
						MaxOccurrences: 1,
					},
				},
				ChaosSeed:  42,
				StrictMode: false,
			},
		},
		{
			Name:        "Intermittent Network Transport Flakiness",
			Description: "Flaky TCP socket drops tool call; agent must re-establish session and resume",
			Session:     baseSession,
			Config: ChaosExperimentConfig{
				ExperimentName: "Intermittent Flakiness",
				Rules: []FaultRule{
					{
						ID:             "rule-flaky-net",
						TargetTool:     "sql_query",
						Type:           FaultToolFlaky,
						Probability:    1.0,
						MaxOccurrences: 1,
					},
					{
						ID:             "rule-corrupt-json",
						TargetTool:     "read_file",
						Type:           FaultPayloadCorrupt,
						Probability:    1.0,
						MaxOccurrences: 1,
					},
				},
				ChaosSeed:  42,
				StrictMode: false,
			},
		},
		{
			Name:        "Upstream Outage & Cascading State Failure",
			Description: "Permanent 503 outage and state schema mutation; testing graceful degradation",
			Session:     baseSession,
			Config: ChaosExperimentConfig{
				ExperimentName: "Cascading Outage",
				Rules: []FaultRule{
					{
						ID:             "rule-upstream-503",
						TargetTool:     "deploy_cluster",
						Type:           FaultUpstreamOutage,
						Probability:    1.0,
						MaxOccurrences: 1,
					},
					{
						ID:             "rule-state-corruption",
						TargetTool:     "update_config",
						Type:           FaultStateCorruption,
						Probability:    1.0,
						MaxOccurrences: 1,
					},
				},
				ChaosSeed:  42,
				StrictMode: false,
			},
		},
	}
}

// RunDemoSuite runs all scenarios and compiles the benchmark suite summary.
func RunDemoSuite(strict bool) (*ChaosBenchmarkSuite, error) {
	scenarios := GenerateDemoScenarios()
	engine := NewChaosEngine(42)

	suite := &ChaosBenchmarkSuite{
		Timestamp:      time.Now(),
		TotalScenarios: len(scenarios),
		Reports:        make([]ChaosExperimentReport, 0, len(scenarios)),
	}

	totalScore := 0.0
	for _, sc := range scenarios {
		cfg := sc.Config
		if strict {
			cfg.StrictMode = true
		}

		report, err := engine.EvaluateSession(sc.Session, cfg)
		if err != nil {
			return nil, fmt.Errorf("failed executing scenario %s: %w", sc.Name, err)
		}

		suite.Reports = append(suite.Reports, *report)
		if report.Passed {
			suite.PassedScenarios++
		}
		totalScore += report.ResilienceScore
		suite.TotalFaultsInjected += report.TotalFaultsInjected
		suite.TotalFaultsRecovered += report.TotalFaultsRecovered
	}

	if suite.TotalScenarios > 0 {
		suite.OverallResilience = totalScore / float64(suite.TotalScenarios)
	}

	if suite.TotalFaultsInjected > 0 {
		suite.OverallRecoveryRate = float64(suite.TotalFaultsRecovered) / float64(suite.TotalFaultsInjected) * 100.0
	} else {
		suite.OverallRecoveryRate = 100.0
	}

	return suite, nil
}

func generateBaseSession(agentName string, stepCount int) *trace.Session {
	now := time.Now().Add(-10 * time.Minute)
	events := make([]trace.TraceEvent, 0, stepCount)

	tools := []string{
		"read_file",
		"web_search",
		"bash_exec",
		"read_file",
		"sql_query",
		"deploy_cluster",
		"update_config",
		"verify_status",
	}

	prevChecksum := "seed_genesis_hash"
	for i := 0; i < stepCount; i++ {
		tool := tools[i%len(tools)]
		payload := map[string]interface{}{
			"tool":     tool,
			"query":    fmt.Sprintf("query_param_step_%d", i+1),
			"tokens":   150 + i*15,
			"path":     fmt.Sprintf("/workspace/src/file_%d.go", i+1),
		}

		cs := trace.ComputeChecksum(prevChecksum, i+1, trace.EventToolCall, []byte(fmt.Sprintf("%s_%d", tool, i)))
		prevChecksum = cs

		events = append(events, trace.TraceEvent{
			ID:         fmt.Sprintf("evt_demo_%03d", i+1),
			Index:      i + 1,
			Timestamp:  now.Add(time.Duration(i*300) * time.Millisecond),
			Type:       trace.EventToolCall,
			DurationMs: int64(45 + i*5),
			Payload:    payload,
			Checksum:   cs,
		})
	}

	return &trace.Session{
		ID:        "session_chaos_benchmark_001",
		AgentName: agentName,
		CreatedAt: now,
		UpdatedAt: now.Add(time.Duration(stepCount*300) * time.Millisecond),
		Events:    events,
		Status:    "completed",
		Metadata: map[string]string{
			"environment": "benchmark",
			"suite":       "chaos_resilience",
		},
	}
}
