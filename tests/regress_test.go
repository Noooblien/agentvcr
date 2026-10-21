package tests

import (
	"testing"

	"github.com/Noooblien/agentvcr/pkg/regress"
)

func TestIntegration_Regress_EndToEnd(t *testing.T) {
	suites := regress.GetDemoSuites()
	if len(suites) != 4 {
		t.Fatalf("Expected 4 demo suites, got %d", len(suites))
	}

	eval := regress.NewEvaluator()

	// 1. First scenario should PASS (model upgrade)
	passSuite := suites[0]
	rep1 := eval.Evaluate(passSuite.Spec, passSuite.Candidate, passSuite.GoldenTokens, passSuite.GoldenDurationMs)
	if rep1.Verdict != regress.VerdictPass {
		t.Errorf("Expected scenario 1 to PASS, got %s", rep1.Verdict)
	}
	if len(rep1.Failures) != 0 {
		t.Errorf("Expected 0 failures in scenario 1, got %d", len(rep1.Failures))
	}
	if rep1.TokenDelta >= 0 {
		t.Errorf("Scenario 1 should have achieved token savings, delta: %d", rep1.TokenDelta)
	}

	// 2. Second scenario should FAIL on runaway loop
	loopSuite := suites[1]
	rep2 := eval.Evaluate(loopSuite.Spec, loopSuite.Candidate, loopSuite.GoldenTokens, loopSuite.GoldenDurationMs)
	if rep2.Verdict != regress.VerdictFail {
		t.Errorf("Expected scenario 2 to FAIL, got %s", rep2.Verdict)
	}
	foundLoop := false
	for _, f := range rep2.Failures {
		if f.RuleID == "RUNAWAY_RETRY_LOOP_DETECTED" {
			foundLoop = true
			break
		}
	}
	if !foundLoop {
		t.Errorf("Expected RUNAWAY_RETRY_LOOP_DETECTED failure in scenario 2")
	}

	// 3. Third scenario should FAIL on missing prerequisite
	prereqSuite := suites[2]
	rep3 := eval.Evaluate(prereqSuite.Spec, prereqSuite.Candidate, prereqSuite.GoldenTokens, prereqSuite.GoldenDurationMs)
	if rep3.Verdict != regress.VerdictFail {
		t.Errorf("Expected scenario 3 to FAIL, got %s", rep3.Verdict)
	}
	foundPrereq := false
	for _, f := range rep3.Failures {
		if f.RuleID == "MISSING_PREREQUISITE_TOOLS" {
			foundPrereq = true
			break
		}
	}
	if !foundPrereq {
		t.Errorf("Expected MISSING_PREREQUISITE_TOOLS failure in scenario 3")
	}

	// 4. Fourth scenario should FAIL on forbidden tool
	forbiddenSuite := suites[3]
	rep4 := eval.Evaluate(forbiddenSuite.Spec, forbiddenSuite.Candidate, forbiddenSuite.GoldenTokens, forbiddenSuite.GoldenDurationMs)
	if rep4.Verdict != regress.VerdictFail {
		t.Errorf("Expected scenario 4 to FAIL, got %s", rep4.Verdict)
	}
	foundForbidden := false
	for _, f := range rep4.Failures {
		if f.RuleID == "FORBIDDEN_TOOLS_INVOCATION" {
			foundForbidden = true
			break
		}
	}
	if !foundForbidden {
		t.Errorf("Expected FORBIDDEN_TOOLS_INVOCATION failure in scenario 4")
	}
}

func TestIntegration_Regress_LCSMetricBoundary(t *testing.T) {
	golden := []string{"auth", "get", "post"}
	candReversed := []string{"post", "get", "auth"}

	align := regress.AlignToolSequences(golden, candReversed)
	// In reverse order, LCS length is 1
	if align.LCSLength != 1 {
		t.Errorf("Expected LCS=1 for reversed sequence, got %d", align.LCSLength)
	}
	if align.OrderPreserved {
		t.Errorf("Expected OrderPreserved=false for reversed sequence")
	}
}
