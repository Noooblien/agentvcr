package regress

import (
	"strings"
	"testing"
)

func TestComputeLCS(t *testing.T) {
	seq1 := []string{"a", "b", "c", "d"}
	seq2 := []string{"a", "x", "b", "c", "y", "d"}
	lcs := ComputeLCS(seq1, seq2)
	if lcs != 4 {
		t.Errorf("Expected LCS=4, got %d", lcs)
	}

	disjoint1 := []string{"a", "b"}
	disjoint2 := []string{"c", "d"}
	if ComputeLCS(disjoint1, disjoint2) != 0 {
		t.Errorf("Expected LCS=0 for disjoint sequences")
	}
}

func TestAlignToolSequences(t *testing.T) {
	golden := []string{"connect", "auth", "query", "close"}
	candidate := []string{"connect", "retry", "auth", "query", "close"}

	res := AlignToolSequences(golden, candidate)
	if res.LCSLength != 4 {
		t.Errorf("Expected LCS 4, got %d", res.LCSLength)
	}
	if len(res.ExtraneousTools) != 1 || res.ExtraneousTools[0] != "retry" {
		t.Errorf("Expected extraneous tool 'retry', got %v", res.ExtraneousTools)
	}
	if len(res.MissingTools) != 0 {
		t.Errorf("Expected 0 missing tools, got %v", res.MissingTools)
	}
	if res.SimilarityScore < 0.75 {
		t.Errorf("Expected similarity >= 0.75, got %.2f", res.SimilarityScore)
	}
}

func TestEvaluator_PassingScenario(t *testing.T) {
	suites := GetDemoSuites()
	eval := NewEvaluator()

	// Scenario 1: Model upgrade
	s1 := suites[0]
	rep := eval.Evaluate(s1.Spec, s1.Candidate, s1.GoldenTokens, s1.GoldenDurationMs)

	if rep.Verdict != VerdictPass {
		t.Fatalf("Expected scenario 1 to PASS, got %s (failures: %v)", rep.Verdict, rep.Failures)
	}
	if rep.PassRate != 100.0 {
		t.Errorf("Expected 100%% pass rate, got %.1f%%", rep.PassRate)
	}
	if rep.TokenDelta >= 0 {
		t.Errorf("Expected negative token delta (reduction), got %+d", rep.TokenDelta)
	}
}

func TestEvaluator_RunawayLoop(t *testing.T) {
	suites := GetDemoSuites()
	eval := NewEvaluator()

	// Scenario 2: Runaway loop
	s2 := suites[1]
	rep := eval.Evaluate(s2.Spec, s2.Candidate, s2.GoldenTokens, s2.GoldenDurationMs)

	if rep.Verdict != VerdictFail {
		t.Fatalf("Expected scenario 2 to FAIL, got %s", rep.Verdict)
	}

	hasLoopFailure := false
	hasTokenBloat := false
	for _, f := range rep.Failures {
		if f.RuleID == "RUNAWAY_RETRY_LOOP_DETECTED" {
			hasLoopFailure = true
			if f.Severity != SeverityCritical {
				t.Errorf("Expected loop failure to be CRITICAL, got %s", f.Severity)
			}
		}
		if f.RuleID == "MAX_TOKENS_EXCEEDED" || f.RuleID == "TOKEN_BLOAT_REGRESSION" {
			hasTokenBloat = true
		}
	}

	if !hasLoopFailure {
		t.Errorf("Expected RUNAWAY_RETRY_LOOP_DETECTED failure")
	}
	if !hasTokenBloat {
		t.Errorf("Expected token bloat failure")
	}
}

func TestEvaluator_MissingPrerequisite(t *testing.T) {
	suites := GetDemoSuites()
	eval := NewEvaluator()

	// Scenario 3: Missing auth prerequisite
	s3 := suites[2]
	rep := eval.Evaluate(s3.Spec, s3.Candidate, s3.GoldenTokens, s3.GoldenDurationMs)

	if rep.Verdict != VerdictFail {
		t.Fatalf("Expected scenario 3 to FAIL, got %s", rep.Verdict)
	}

	hasMissingPrereq := false
	for _, f := range rep.Failures {
		if f.RuleID == "MISSING_PREREQUISITE_TOOLS" {
			hasMissingPrereq = true
			if f.Severity != SeverityMajor {
				t.Errorf("Expected missing prereq to be MAJOR, got %s", f.Severity)
			}
		}
	}

	if !hasMissingPrereq {
		t.Errorf("Expected MISSING_PREREQUISITE_TOOLS failure")
	}
}

func TestEvaluator_ForbiddenTool(t *testing.T) {
	suites := GetDemoSuites()
	eval := NewEvaluator()

	// Scenario 4: Forbidden tool
	s4 := suites[3]
	rep := eval.Evaluate(s4.Spec, s4.Candidate, s4.GoldenTokens, s4.GoldenDurationMs)

	if rep.Verdict != VerdictFail {
		t.Fatalf("Expected scenario 4 to FAIL, got %s", rep.Verdict)
	}

	hasForbidden := false
	for _, f := range rep.Failures {
		if f.RuleID == "FORBIDDEN_TOOLS_INVOCATION" {
			hasForbidden = true
			if f.Severity != SeverityCritical {
				t.Errorf("Expected forbidden tool failure to be CRITICAL, got %s", f.Severity)
			}
		}
	}

	if !hasForbidden {
		t.Errorf("Expected FORBIDDEN_TOOLS_INVOCATION failure")
	}
}

func TestScorecardsAndRenderers(t *testing.T) {
	suites := GetDemoSuites()
	eval := NewEvaluator()

	var reports []*RegressionReport
	for _, s := range suites {
		rep := eval.Evaluate(s.Spec, s.Candidate, s.GoldenTokens, s.GoldenDurationMs)
		reports = append(reports, rep)

		scorecard := RenderRegressionScorecard(rep)
		if !strings.Contains(scorecard, "GOLDEN TRAJECTORY REGRESSION") {
			t.Errorf("Scorecard missing header")
		}

		mermaid := RenderMermaidDiff(s.Spec, s.Candidate, rep)
		if !strings.Contains(mermaid, "flowchart TD") {
			t.Errorf("Mermaid missing flowchart TD")
		}
	}

	md := RenderMarkdownReport(reports)
	if !strings.Contains(md, "Summary Matrix") || !strings.Contains(md, "Granular Assertion Failures") {
		t.Errorf("Markdown report missing expected sections")
	}
}
