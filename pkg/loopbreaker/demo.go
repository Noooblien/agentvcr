package loopbreaker

import (
	"time"
)

// RunDemoSuite runs the 4 production scenarios demonstrating reasoning cycle detection, argument similarity scoring, and automated circuit breaking.
func RunDemoSuite(config LoopConfig) LoopBreakerSummary {
	breaker := NewLoopBreaker(config)
	var allSteps []StepRecord
	stepIdx := 0

	// -------------------------------------------------------------------------
	// Scenario 1: Legitimate Batch Processing (Different Arguments, No Cycle)
	// -------------------------------------------------------------------------
	files := []string{"config.json", "src/main.go", "src/types.go", "db/schema.sql"}
	for _, f := range files {
		allSteps = append(allSteps, StepRecord{
			StepIndex:      stepIdx,
			Timestamp:      time.Now(),
			ToolName:       "read_file",
			Arguments:      map[string]interface{}{"path": f},
			OutputJSON:     `{"status": "ok", "lines": 42}`,
			IsError:        false,
			TokensConsumed: 180,
		})
		stepIdx++
	}

	// -------------------------------------------------------------------------
	// Scenario 2: Syntax Error Self-Loop (Period 1, 100% Arg Similarity)
	// -------------------------------------------------------------------------
	allSteps = append(allSteps, StepRecord{
		StepIndex:      stepIdx,
		Timestamp:      time.Now(),
		ToolName:       "list_tables",
		Arguments:      map[string]interface{}{"schema": "public"},
		OutputJSON:     `{"tables": ["users", "orders", "audit_log"]}`,
		IsError:        false,
		TokensConsumed: 120,
	})
	stepIdx++

	for i := 0; i < 3; i++ {
		allSteps = append(allSteps, StepRecord{
			StepIndex:      stepIdx,
			Timestamp:      time.Now(),
			ToolName:       "execute_sql",
			Arguments:      map[string]interface{}{"query": "SELECT * FORM users WHERE active=1;"},
			OutputJSON:     `{"error": "SyntaxError: syntax error at or near 'FORM'"}`,
			IsError:        true,
			TokensConsumed: 320,
		})
		stepIdx++
	}

	// -------------------------------------------------------------------------
	// Scenario 3: Bipartite Ping-Pong Oscillation (Period 2 Deadlock)
	// -------------------------------------------------------------------------
	for i := 0; i < 3; i++ {
		allSteps = append(allSteps, StepRecord{
			StepIndex:      stepIdx,
			Timestamp:      time.Now(),
			ToolName:       "format_code",
			Arguments:      map[string]interface{}{"target": "src/api.go"},
			OutputJSON:     `{"formatted": true, "diff": "trailing comma removed"}`,
			IsError:        false,
			TokensConsumed: 250,
		})
		stepIdx++

		allSteps = append(allSteps, StepRecord{
			StepIndex:      stepIdx,
			Timestamp:      time.Now(),
			ToolName:       "lint_code",
			Arguments:      map[string]interface{}{"target": "src/api.go"},
			OutputJSON:     `{"error": "LinterError: missing trailing comma in multiline struct"}`,
			IsError:        true,
			TokensConsumed: 280,
		})
		stepIdx++
	}

	// -------------------------------------------------------------------------
	// Scenario 4: Circular Web Scraping Deadlock (Period 3 Multi-Step Cycle)
	// -------------------------------------------------------------------------
	for i := 0; i < 3; i++ {
		allSteps = append(allSteps, StepRecord{
			StepIndex:      stepIdx,
			Timestamp:      time.Now(),
			ToolName:       "search_web",
			Arguments:      map[string]interface{}{"query": "q3 enterprise financial metrics 2026"},
			OutputJSON:     `{"results": ["https://ir.corp.com/q3"]}`,
			IsError:        false,
			TokensConsumed: 400,
		})
		stepIdx++

		allSteps = append(allSteps, StepRecord{
			StepIndex:      stepIdx,
			Timestamp:      time.Now(),
			ToolName:       "fetch_url",
			Arguments:      map[string]interface{}{"url": "https://ir.corp.com/q3"},
			OutputJSON:     `{"html": "<div id='empty-report'>No data available</div>"}`,
			IsError:        false,
			TokensConsumed: 450,
		})
		stepIdx++

		allSteps = append(allSteps, StepRecord{
			StepIndex:      stepIdx,
			Timestamp:      time.Now(),
			ToolName:       "extract_table",
			Arguments:      map[string]interface{}{"selector": "#financials"},
			OutputJSON:     `{"error": "TableNotFound: element #financials missing"}`,
			IsError:        true,
			TokensConsumed: 300,
		})
		stepIdx++
	}

	summary := breaker.InspectTrace(allSteps)
	return *summary
}
