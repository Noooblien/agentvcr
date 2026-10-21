package counterfactual

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// DefaultDemoScenarios builds 4 realistic baseline traces and corresponding what-if mutations.
func DefaultDemoScenarios() []DemoScenario {
	now := time.Now().UTC()

	// Scenario 1: API 429 Rate Limit -> Cache Fallback Tool Substitution
	base1 := &trace.Session{
		ID:        "trace_demo_ratelimit_429",
		AgentName: "agent-researcher-01",
		CreatedAt: now.Add(-10 * time.Minute),
		Status:    "failed",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, DurationMs: 15, Payload: map[string]interface{}{"query": "fetch remote market price", "tokens": 150}},
			{Index: 2, Type: trace.EventToolCall, DurationMs: 350, Payload: map[string]interface{}{"tool": "remote_market_api", "arguments": map[string]interface{}{"ticker": "AAPL"}, "tokens": 80}},
			{Index: 3, Type: trace.EventToolResult, DurationMs: 10, Payload: map[string]interface{}{"status_code": 429, "error": "rate limit exceeded", "status": "error", "tokens": 40}},
			{Index: 4, Type: trace.EventError, DurationMs: 5, Payload: map[string]interface{}{"error": "Upstream 429 fatal error", "tokens": 30}},
		},
	}
	mut1 := CounterfactualMutation{
		ID:              "whatif_cache_fallback",
		Name:            "Rate Limit 429 -> Local Cache Fallback",
		Description:     "What if the agent had fallen back to local_market_cache instead of crashing on remote API 429?",
		BranchStep:      2,
		Type:            MutateToolSubstitution,
		TargetTool:      "remote_market_api",
		ReplacementTool: "local_market_cache",
		ArgumentOverrides: map[string]interface{}{
			"ticker": "AAPL",
			"ttl":    "5m",
		},
		MockResponse: map[string]interface{}{
			"price":  182.50,
			"source": "local_cache",
			"status": "success",
		},
	}

	// Scenario 2: SQL Column Name Error -> Argument Override Schema Correction
	base2 := &trace.Session{
		ID:        "trace_demo_sql_error",
		AgentName: "agent-data-analyst",
		CreatedAt: now.Add(-8 * time.Minute),
		Status:    "failed",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, DurationMs: 20, Payload: map[string]interface{}{"query": "find all active customer emails", "tokens": 180}},
			{Index: 2, Type: trace.EventToolCall, DurationMs: 45, Payload: map[string]interface{}{"tool": "sql_query", "arguments": map[string]interface{}{"query": "SELECT usr_name, eml FROM customers WHERE act = 1"}, "tokens": 90}},
			{Index: 3, Type: trace.EventToolResult, DurationMs: 15, Payload: map[string]interface{}{"status": "error", "error": "column 'usr_name' does not exist", "status_code": 400, "tokens": 45}},
			{Index: 4, Type: trace.EventError, DurationMs: 5, Payload: map[string]interface{}{"error": "SQL execution aborted", "tokens": 30}},
		},
	}
	mut2 := CounterfactualMutation{
		ID:          "whatif_sql_schema_correction",
		Name:        "SQL Column Name Error -> Schema Argument Override",
		Description: "What if the agent had supplied the schema-correct column names (username, email, is_active)?",
		BranchStep:  2,
		Type:        MutateArgumentOverride,
		TargetTool:  "sql_query",
		ArgumentOverrides: map[string]interface{}{
			"query": "SELECT username, email FROM customers WHERE is_active = true",
		},
		MockResponse: map[string]interface{}{
			"status": "success",
			"rows":   []interface{}{map[string]interface{}{"username": "alice", "email": "alice@corp.internal"}},
		},
	}

	// Scenario 3: Runaway Vague Search Loop -> Human Steering Prompt Injection
	base3 := &trace.Session{
		ID:        "trace_demo_search_loop",
		AgentName: "agent-doc-searcher",
		CreatedAt: now.Add(-5 * time.Minute),
		Status:    "completed",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, DurationMs: 25, Payload: map[string]interface{}{"query": "find auth docs", "tokens": 200}},
			{Index: 2, Type: trace.EventToolCall, DurationMs: 120, Payload: map[string]interface{}{"tool": "web_search", "arguments": map[string]interface{}{"q": "oauth specs"}, "tokens": 350}},
			{Index: 3, Type: trace.EventToolResult, DurationMs: 30, Payload: map[string]interface{}{"results": 5, "tokens": 450}},
			{Index: 4, Type: trace.EventToolCall, DurationMs: 150, Payload: map[string]interface{}{"tool": "web_search", "arguments": map[string]interface{}{"q": "oauth enterprise auth headers"}, "tokens": 400}},
			{Index: 5, Type: trace.EventToolResult, DurationMs: 25, Payload: map[string]interface{}{"results": 8, "tokens": 520}},
			{Index: 6, Type: trace.EventToolCall, DurationMs: 140, Payload: map[string]interface{}{"tool": "web_search", "arguments": map[string]interface{}{"q": "internal auth corp manual v2"}, "tokens": 480}},
			{Index: 7, Type: trace.EventToolResult, DurationMs: 30, Payload: map[string]interface{}{"results": 4, "tokens": 510}},
			{Index: 8, Type: trace.EventLLMResponse, DurationMs: 300, Payload: map[string]interface{}{"content": "Located docs after 3 iterations", "tokens": 650}},
		},
	}
	mut3 := CounterfactualMutation{
		ID:             "whatif_human_steering",
		Name:           "Runaway Search Loop -> Human Steering Prompt",
		Description:    "What if a human operator injected precise steering guidance at Step 2 to bypass 3 search loops?",
		BranchStep:     2,
		Type:           MutateSteeringPrompt,
		TargetTool:     "fetch_url",
		SteeringPrompt: "Directly open internal developer portal at https://internal.corp/docs/auth-v2",
		ArgumentOverrides: map[string]interface{}{
			"url": "https://internal.corp/docs/auth-v2",
		},
	}

	// Scenario 4: Redundant File Read Pruning -> Response Mocking
	base4 := &trace.Session{
		ID:        "trace_demo_redundant_reads",
		AgentName: "agent-builder-04",
		CreatedAt: now.Add(-2 * time.Minute),
		Status:    "completed",
		Events: []trace.TraceEvent{
			{Index: 1, Type: trace.EventLLMRequest, DurationMs: 20, Payload: map[string]interface{}{"query": "build project", "tokens": 150}},
			{Index: 2, Type: trace.EventToolCall, DurationMs: 80, Payload: map[string]interface{}{"tool": "read_file", "arguments": map[string]interface{}{"path": "config.yaml"}, "tokens": 120}},
			{Index: 3, Type: trace.EventToolResult, DurationMs: 10, Payload: map[string]interface{}{"content": "version: 2", "tokens": 180}},
			{Index: 4, Type: trace.EventToolCall, DurationMs: 90, Payload: map[string]interface{}{"tool": "read_file", "arguments": map[string]interface{}{"path": "config.yaml"}, "tokens": 120}},
			{Index: 5, Type: trace.EventToolResult, DurationMs: 10, Payload: map[string]interface{}{"content": "version: 2", "tokens": 180}},
			{Index: 6, Type: trace.EventLLMResponse, DurationMs: 120, Payload: map[string]interface{}{"content": "Build complete", "tokens": 200}},
		},
	}
	mut4 := CounterfactualMutation{
		ID:          "whatif_mock_cached_read",
		Name:        "Redundant File Read -> Instant Mock Cache",
		Description: "What if the second redundant read of config.yaml was served instantly from memory mock?",
		BranchStep:  2,
		Type:        MutateResponseMock,
		TargetTool:  "read_file",
		ArgumentOverrides: map[string]interface{}{
			"path": "config.yaml",
		},
		MockResponse: map[string]interface{}{
			"content": "version: 2",
			"cached":  true,
		},
	}

	return []DemoScenario{
		{Name: mut1.Name, Description: mut1.Description, Baseline: base1, Mutation: mut1},
		{Name: mut2.Name, Description: mut2.Description, Baseline: base2, Mutation: mut2},
		{Name: mut3.Name, Description: mut3.Description, Baseline: base3, Mutation: mut3},
		{Name: mut4.Name, Description: mut4.Description, Baseline: base4, Mutation: mut4},
	}
}

// RunDemoSuite runs all pre-packaged counterfactual scenarios and produces comparisons and aggregate metrics.
func RunDemoSuite() ([]*BranchComparison, *SimulationSummary, error) {
	startTime := time.Now()
	sim := NewSimulator()
	scenarios := DefaultDemoScenarios()

	var comparisons []*BranchComparison
	summary := &SimulationSummary{
		TotalScenarios: len(scenarios),
	}

	totalTokenPctSum := 0.0

	for _, sc := range scenarios {
		_, comp, err := sim.Simulate(sc.Baseline, sc.Mutation)
		if err != nil {
			return nil, nil, err
		}
		comparisons = append(comparisons, comp)

		if comp.Outcome == OutcomeResolvedSuccess {
			summary.ResolvedSuccessCount++
		} else if comp.Outcome == OutcomeAlternativeEquiv {
			summary.AlternativeEquivCount++
		} else if comp.Outcome == OutcomeRegressionFailure {
			summary.RegressionCount++
		}

		if comp.TokenDelta < 0 {
			summary.NetTokensSaved += -comp.TokenDelta
		}
		summary.NetCostSavingsUSD += -comp.CostDeltaUSD
		if comp.LatencyDeltaMs < 0 {
			summary.NetLatencySavedMs += -comp.LatencyDeltaMs
		}

		totalTokenPctSum += comp.TokenSavingsPct
	}

	if len(scenarios) > 0 {
		summary.AverageTokenSavingsPct = totalTokenPctSum / float64(len(scenarios))
	}
	summary.Comparisons = comparisons
	summary.Duration = time.Since(startTime)

	return comparisons, summary, nil
}
