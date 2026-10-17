package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/research"
)

func TestResearch_GoalDecompositionAndTreeStructure(t *testing.T) {
	cfg := research.DefaultResearchConfig()
	cfg.MaxDepth = 3
	cfg.MaxBranchesPerNode = 3

	engine := research.NewEngine(cfg)

	corpus := []research.Document{
		{
			URI:        "file:///pkg/dashboard/hub.go",
			Content:    "package dashboard\n\ntype EventHub struct {\n\tsubscribers map[chan TelemetryEvent]bool\n\tmu sync.RWMutex\n}\n\nfunc (h *EventHub) Broadcast(ev TelemetryEvent) {\n\th.mu.RLock()\n\tdefer h.mu.RUnlock()\n\tfor ch := range h.subscribers {\n\t\tch <- ev\n\t}\n}",
			SourceType: research.SourceLocalCode,
		},
		{
			URI:        "file:///docs/concurrency.md",
			Content:    "The telemetry event hub provides thread-safe broadcasting using sync.RWMutex.\nSubscribers are notified asynchronously via buffered channels to prevent head-of-line blocking.",
			SourceType: research.SourceDocs,
		},
	}

	report, err := engine.ExecuteResearch("Analyze concurrency patterns and thread-safe lock contention in telemetry event hub", corpus)
	if err != nil {
		t.Fatalf("ExecuteResearch failed: %v", err)
	}

	if report.Tree == nil {
		t.Fatal("Expected non-nil ResearchTree")
	}

	if report.Tree.TotalNodes < 2 {
		t.Errorf("Expected at least 2 nodes in tree, got %d", report.Tree.TotalNodes)
	}

	if len(report.VerifiedClaims) == 0 {
		t.Error("Expected at least one verified claim from corpus")
	}

	if len(report.Citations) == 0 {
		t.Error("Expected at least one citation to be extracted")
	}

	for _, cit := range report.Citations {
		if cit.ContentHash == "" || len(cit.ContentHash) != 64 {
			t.Errorf("Invalid SHA-256 content hash: %s", cit.ContentHash)
		}
	}

	if !report.PassedAudit {
		t.Errorf("Expected audit to pass for well-grounded corpus, but PassedAudit=false")
	}
}

func TestResearch_EvidenceGatheringAndCitationHashing(t *testing.T) {
	cfg := research.DefaultResearchConfig()
	engine := research.NewEngine(cfg)

	doc := research.Document{
		URI:        "https://agentvcr.dev/security/rbac",
		Content:    "Zero-trust RBAC policy audit mandates strict namespace isolation.\nAll API tokens are hashed using SHA-256 digests.",
		SourceType: research.SourceWeb,
	}

	report, err := engine.ExecuteResearch("Audit security authorization and namespace isolation in RBAC", []research.Document{doc})
	if err != nil {
		t.Fatalf("ExecuteResearch failed: %v", err)
	}

	found := false
	for _, cit := range report.Citations {
		if cit.SourceURI == "https://agentvcr.dev/security/rbac" {
			found = true
			if cit.SourceType != research.SourceWeb {
				t.Errorf("Expected SourceWeb, got %s", cit.SourceType)
			}
			break
		}
	}

	if !found {
		t.Errorf("Expected citation from https://agentvcr.dev/security/rbac")
	}
}

func TestResearch_HallucinationPruning(t *testing.T) {
	cfg := research.DefaultResearchConfig()
	cfg.GroundingThreshold = 0.60
	engine := research.NewEngine(cfg)

	// Empty corpus should trigger ungrounded claims and prune them
	report, err := engine.ExecuteResearch("Speculate on unrecorded quantum entanglement protocols", []research.Document{})
	if err != nil {
		t.Fatalf("ExecuteResearch failed: %v", err)
	}

	if len(report.PrunedClaims) == 0 {
		t.Errorf("Expected ungrounded claims to be pruned, got 0 pruned claims")
	}

	if len(report.VerifiedClaims) > 0 {
		t.Errorf("Expected 0 verified claims for empty corpus, got %d", len(report.VerifiedClaims))
	}

	if report.PassedAudit {
		t.Error("Expected audit to fail (PassedAudit=false) when hallucinated claims exist")
	}
}

func TestResearch_ContradictionDetectionAndResolution(t *testing.T) {
	cfg := research.DefaultResearchConfig()
	cfg.EnableContradictionDetection = true
	engine := research.NewEngine(cfg)

	corpus := []research.Document{
		{
			URI:        "file:///pkg/proxy/server.go",
			Content:    "The proxy server implementation is thread-safe across all concurrent requests.",
			SourceType: research.SourceLocalCode,
		},
		{
			URI:        "file:///tests/stress_test.go",
			Content:    "Under extreme load we identified a critical race condition leading to data corruption.",
			SourceType: research.SourceLocalCode,
		},
	}

	report, err := engine.ExecuteResearch("Investigate thread-safe behavior and race condition vulnerabilities", corpus)
	if err != nil {
		t.Fatalf("ExecuteResearch failed: %v", err)
	}

	if len(report.Contradictions) == 0 {
		t.Errorf("Expected contradiction to be detected between 'thread-safe' and 'race condition'")
	} else {
		con := report.Contradictions[0]
		if con.Severity != "High" {
			t.Errorf("Expected High severity contradiction, got %s", con.Severity)
		}
		if con.Resolution == "" {
			t.Error("Expected automated conflict resolution message")
		}
	}
}

func TestResearch_MermaidTreeExport(t *testing.T) {
	cfg := research.DefaultResearchConfig()
	engine := research.NewEngine(cfg)

	doc := research.Document{
		URI:        "file:///pkg/compact/compactor.go",
		Content:    "The compactor deduplicates redundant tool calls using cryptographic SHA-256 hashes.",
		SourceType: research.SourceLocalCode,
	}

	report, err := engine.ExecuteResearch("Analyze context window compaction mechanisms", []research.Document{doc})
	if err != nil {
		t.Fatalf("ExecuteResearch failed: %v", err)
	}

	mermaid := research.ExportMermaidResearchTree(report.Tree)
	if !strings.HasPrefix(mermaid, "```mermaid\nflowchart TD") {
		t.Errorf("Expected mermaid flowchart header, got:\n%s", mermaid)
	}
	if !strings.Contains(mermaid, "node_root") {
		t.Errorf("Expected node_root in mermaid diagram")
	}
}

func TestResearch_ScorecardRendering(t *testing.T) {
	cfg := research.DefaultResearchConfig()
	engine := research.NewEngine(cfg)

	doc := research.Document{
		URI:        "file:///pkg/perf/profiler.go",
		Content:    "High-precision monotonic clock measures P99 latency down to nanosecond resolution.",
		SourceType: research.SourceLocalCode,
	}

	report, err := engine.ExecuteResearch("Profile P99 latency and high-precision timers", []research.Document{doc})
	if err != nil {
		t.Fatalf("ExecuteResearch failed: %v", err)
	}

	card := research.RenderResearchScorecard(report)
	if !strings.Contains(card, "[AgentVCR] RECURSIVE MULTI-AGENT RESEARCH & CITATION VERIFICATION ENGINE") {
		t.Errorf("Expected header banner in scorecard")
	}
	if !strings.Contains(card, "Research Tree Metrics") {
		t.Errorf("Expected metrics section in scorecard")
	}
	if !strings.Contains(card, "Cryptographic Citations") {
		t.Errorf("Expected citations section in scorecard")
	}
}
