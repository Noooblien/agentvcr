package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/mcp"
)

func TestMCP_SchemaFuzzer_GenerateMutations(t *testing.T) {
	tool := mcp.Tool{
		Name:        "k8s_exec",
		Description: "Execute kubectl command against cluster",
		InputSchema: mcp.ToolInputSchema{
			Type:     "object",
			Required: []string{"pod_name", "command"},
			Properties: map[string]interface{}{
				"pod_name": map[string]interface{}{"type": "string"},
				"command":  map[string]interface{}{"type": "string"},
				"retries":  map[string]interface{}{"type": "integer"},
				"verbose":  map[string]interface{}{"type": "boolean"},
			},
		},
	}

	fuzzer := mcp.NewSchemaFuzzer(12345)
	mutations := fuzzer.GenerateMutations(tool)

	if len(mutations) == 0 {
		t.Fatalf("Expected fuzzer to generate mutations, got 0")
	}

	// Verify required strategies exist
	strategiesFound := make(map[mcp.MutationStrategy]bool)
	for _, m := range mutations {
		strategiesFound[m.Strategy] = true
	}

	expectedStrategies := []mcp.MutationStrategy{
		mcp.MutateMissingRequired,
		mcp.MutateTypeConfusion,
		mcp.MutateBoundaryValue,
		mcp.MutateUnicodeExploit,
		mcp.MutateBufferOverflow,
		mcp.MutatePromptInjection,
		mcp.MutateNullByteInjection,
		mcp.MutateNestedRecursion,
	}

	for _, s := range expectedStrategies {
		if !strategiesFound[s] {
			t.Errorf("Expected mutation strategy %s was not generated", s)
		}
	}
}

func TestMCP_SchemaFuzzer_ExecuteTrials_ZeroPanic(t *testing.T) {
	tool := mcp.Tool{
		Name:        "db_query",
		Description: "Execute database SQL query",
		InputSchema: mcp.ToolInputSchema{
			Type:     "object",
			Required: []string{"query"},
			Properties: map[string]interface{}{
				"query":   map[string]interface{}{"type": "string"},
				"timeout": map[string]interface{}{"type": "integer"},
			},
		},
	}

	server := mcp.NewVirtualMCPServer("Test-Fuzz-Target", "1.0.0")
	server.RegisterTool(tool)

	fuzzer := mcp.NewSchemaFuzzer(999)
	mutations := fuzzer.GenerateMutations(tool)

	panics := 0
	defended := 0

	for _, m := range mutations {
		trial := fuzzer.ExecuteTrial(tool, m, server)
		if trial.ServerPanic {
			panics++
		}
		if trial.BlockedByVal || trial.ServerHandled {
			defended++
		}
	}

	if panics > 0 {
		t.Fatalf("Adversarial mutation caused %d server panics!", panics)
	}

	resilience := float64(defended) / float64(len(mutations)) * 100.0
	if resilience < 90.0 {
		t.Fatalf("Resilience score too low: %.1f%% (expected >= 90.0%%)", resilience)
	}
}

func TestMCP_FuzzScorecardAndMermaid(t *testing.T) {
	report := mcp.FuzzReport{
		ToolName:        "file_system_exec",
		TotalMutations:  18,
		BlockedCount:    16,
		HandledCount:    2,
		PanicCount:      0,
		ResilienceScore: 100.0,
		Results: []mcp.FuzzTrialResult{
			{
				CaseID:        "fuzz-001",
				Strategy:      mcp.MutatePromptInjection,
				TargetField:   "command",
				BlockedByVal:  true,
				ServerHandled: true,
			},
		},
	}

	scorecard := mcp.RenderFuzzScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR // MODEL CONTEXT PROTOCOL (MCP) ADVERSARIAL SCHEMA FUZZ") {
		t.Fatalf("Scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "100.0%") || !strings.Contains(scorecard, "file_system_exec") {
		t.Fatalf("Scorecard missing key metrics: %s", scorecard)
	}

	mermaid := mcp.RenderFuzzMermaid(report)
	if !strings.Contains(mermaid, "graph TD") || !strings.Contains(mermaid, "file_system_exec") {
		t.Fatalf("Mermaid diagram missing graph structure: %s", mermaid)
	}
}
