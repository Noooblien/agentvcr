package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/forensics"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestForensics_FaithfulGrounding(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "rag-faithful-session",
		AgentName: "research-assistant",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventToolResult,
				DurationMs: 40,
				Payload: map[string]interface{}{
					"tool_name": "vector_search",
					"results": []map[string]interface{}{
						{
							"title":   "Project Titan Overview",
							"content": "Project Titan reached 100,000 daily active users in September 2026. The backend infrastructure was migrated to Rust and Go microservices.",
							"url":     "https://internal.wiki/titan",
						},
					},
				},
			},
			{
				Index:      2,
				Timestamp:  now.Add(1 * time.Second),
				Type:       trace.EventLLMResponse,
				DurationMs: 150,
				Payload: map[string]interface{}{
					"content": "According to the internal documentation, Project Titan reached 100,000 daily active users in September 2026. Furthermore, backend infrastructure was migrated to Rust and Go microservices.",
				},
			},
		},
	}

	analyzer := forensics.NewAnalyzer(0.30)
	report, err := analyzer.AuditTrajectory(session)
	if err != nil {
		t.Fatalf("AuditTrajectory failed: %v", err)
	}

	if report.TotalClaimsEvaluated != 2 {
		t.Errorf("expected 2 claims evaluated, got %d", report.TotalClaimsEvaluated)
	}
	if report.FaithfulClaimsCount != 2 {
		t.Errorf("expected 2 faithful claims, got %d", report.FaithfulClaimsCount)
	}
	if report.HallucinationRisk != "LOW" {
		t.Errorf("expected LOW risk, got %s", report.HallucinationRisk)
	}
	if !report.PassedAudit {
		t.Errorf("expected passed audit")
	}

	scorecard := forensics.RenderForensicScorecard(report)
	if !strings.Contains(scorecard, "GROUNDED") {
		t.Errorf("expected scorecard to show GROUNDED status")
	}
}

func TestForensics_HallucinationDetected(t *testing.T) {
	now := time.Now()
	// Trajectory where retrieval returned info about Mars exploration,
	// but agent fabricated medical diagnostics advice about diabetes.
	session := &trace.Session{
		ID:        "rag-hallucination-session",
		AgentName: "advisor-agent",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventToolResult,
				DurationMs: 50,
				Payload: map[string]interface{}{
					"tool_name": "knowledge_retriever",
					"documents": []map[string]interface{}{
						{
							"source":  "space_archive.txt",
							"content": "The rover landed in Jezero crater on Mars, collecting atmospheric readings and mineral soil samples.",
						},
					},
				},
			},
			{
				Index:      2,
				Timestamp:  now.Add(1 * time.Second),
				Type:       trace.EventLLMResponse,
				DurationMs: 200,
				Payload: map[string]interface{}{
					"content": "Patients should immediately administer 50 units of synthetic insulin and avoid all carbohydrates.",
				},
			},
		},
	}

	analyzer := forensics.NewAnalyzer(0.35)
	report, err := analyzer.AuditTrajectory(session)
	if err != nil {
		t.Fatalf("AuditTrajectory failed: %v", err)
	}

	if report.HallucinatedClaimsCount == 0 {
		t.Errorf("expected hallucination to be flagged")
	}
	if report.PassedAudit {
		t.Errorf("expected audit failure due to severe hallucination")
	}
	if report.HallucinationRisk != "CRITICAL" {
		t.Errorf("expected CRITICAL risk, got %s", report.HallucinationRisk)
	}
}

func TestForensics_ContradictionDetected(t *testing.T) {
	now := time.Now()
	session := &trace.Session{
		ID:        "rag-contradiction-session",
		AgentName: "finance-bot",
		CreatedAt: now,
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventToolResult,
				DurationMs: 30,
				Payload: map[string]interface{}{
					"tool_name": "sec_filings",
					"results": []map[string]interface{}{
						{
							"title":   "Q3 2026 Earnings",
							"content": "The company reported operating profit and positive earnings of 50 million dollars.",
						},
					},
				},
			},
			{
				Index:      2,
				Timestamp:  now.Add(1 * time.Second),
				Type:       trace.EventLLMResponse,
				DurationMs: 120,
				Payload: map[string]interface{}{
					"content": "The company reported operating profit was not achieved and resulted in a heavy loss of 50 million dollars.",
				},
			},
		},
	}

	analyzer := forensics.NewAnalyzer(0.35)
	report, err := analyzer.AuditTrajectory(session)
	if err != nil {
		t.Fatalf("AuditTrajectory failed: %v", err)
	}

	if report.ContradictionsCount == 0 {
		t.Errorf("expected contradiction to be detected")
	}
	if report.HallucinationRisk != "CRITICAL" {
		t.Errorf("expected CRITICAL risk for contradiction, got %s", report.HallucinationRisk)
	}
}
