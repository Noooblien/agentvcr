package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
	"github.com/Noooblien/agentvcr/pkg/vision"
)

func TestVision_PerceptualHashAndHammingDistance(t *testing.T) {
	// Create two identical frames
	elementsA := []vision.VisualElement{
		{ID: "btn_1", Tag: "button", Label: "Login", Bounds: vision.BoundingBox{X: 10, Y: 10, Width: 100, Height: 50}},
	}
	_, imgA := vision.GenerateSyntheticFrame(200, 200, elementsA)

	hashA1, _ := vision.ComputePerceptualHash(imgA)
	hashA2, _ := vision.ComputePerceptualHash(imgA)

	if hashA1 != hashA2 {
		t.Fatalf("dHash should be deterministic, got %s != %s", hashA1, hashA2)
	}

	distSelf := vision.HammingDistance(hashA1, hashA2)
	if distSelf != 0 {
		t.Errorf("identical images must have 0 hamming distance, got %d", distSelf)
	}

	// Create drastically different frame (large colored blocks)
	elementsB := []vision.VisualElement{
		{ID: "modal_1", Tag: "modal", Bounds: vision.BoundingBox{X: 0, Y: 0, Width: 200, Height: 200}},
		{ID: "err_1", Tag: "error", Bounds: vision.BoundingBox{X: 20, Y: 20, Width: 150, Height: 150}},
	}
	_, imgB := vision.GenerateSyntheticFrame(200, 200, elementsB)
	hashB, _ := vision.ComputePerceptualHash(imgB)

	distDiff := vision.HammingDistance(hashA1, hashB)
	if distDiff <= 0 {
		t.Errorf("different images should have non-zero hamming distance, got %d", distDiff)
	}
}

func TestVision_ActionGroundingAttestation_SuccessAndFailure(t *testing.T) {
	rec := vision.NewRecorder("test_session_grounding")

	elements := []vision.VisualElement{
		{ID: "btn_checkout", Tag: "button", Label: "Proceed to Checkout", Bounds: vision.BoundingBox{X: 100, Y: 300, Width: 200, Height: 50}},
		{ID: "input_coupon", Tag: "input", Label: "Coupon Code", Bounds: vision.BoundingBox{X: 100, Y: 200, Width: 200, Height: 40}},
	}
	rawPNG, img := vision.GenerateSyntheticFrame(800, 600, elements)
	rec.RecordFrame(1, 800, 600, rawPNG, img, elements)

	// Case 1: Valid click inside btn_checkout
	att1 := rec.AttestAction(1, "click", [2]int{150, 320}, "btn_checkout")
	if !att1.Attested || !att1.InBounds {
		t.Errorf("expected click at [150,320] to hit btn_checkout, got: %+v", att1)
	}
	if att1.GroundingScore != 1.0 {
		t.Errorf("expected grounding score 1.0, got %f", att1.GroundingScore)
	}

	// Case 2: Misgrounded click (clicked input_coupon when targeting btn_checkout)
	att2 := rec.AttestAction(1, "click", [2]int{150, 220}, "btn_checkout")
	if att2.Attested {
		t.Errorf("misgrounded click should not pass attestation: %+v", att2)
	}
	if att2.HitElementID != "input_coupon" {
		t.Errorf("expected hit on input_coupon, got: %s", att2.HitElementID)
	}

	// Case 3: Whiffed click (clicked blank background [500, 500])
	att3 := rec.AttestAction(1, "click", [2]int{500, 500}, "btn_checkout")
	if att3.Attested || att3.InBounds {
		t.Errorf("blank click should not be in bounds: %+v", att3)
	}
	if att3.HitElementID != "none" {
		t.Errorf("expected hit 'none', got: %s", att3.HitElementID)
	}
}

func TestVision_VisualDriftDetection(t *testing.T) {
	f1 := &vision.VisualFrame{
		FrameID:        "f1",
		PerceptualHash: "0000000000000000",
		Elements: []vision.VisualElement{
			{ID: "w1", Bounds: vision.BoundingBox{X: 0, Y: 0, Width: 100, Height: 100}},
		},
	}
	// Identical
	f2 := &vision.VisualFrame{
		FrameID:        "f2",
		PerceptualHash: "0000000000000000",
		Elements: []vision.VisualElement{
			{ID: "w1", Bounds: vision.BoundingBox{X: 0, Y: 0, Width: 100, Height: 100}},
		},
	}
	diffNoDrift := vision.CompareFrames(f1, f2)
	if diffNoDrift.IsVisualDrift || diffNoDrift.HammingDistance != 0 {
		t.Errorf("expected zero drift, got dist: %d", diffNoDrift.HammingDistance)
	}

	// High bit difference (32 bits flipped)
	f3 := &vision.VisualFrame{
		FrameID:        "f3",
		PerceptualHash: "ffffffff00000000",
		Elements: []vision.VisualElement{
			{ID: "modal_err", Bounds: vision.BoundingBox{X: 50, Y: 50, Width: 300, Height: 300}},
		},
	}
	diffDrift := vision.CompareFrames(f1, f3)
	if !diffDrift.IsVisualDrift {
		t.Errorf("expected visual drift detection with 32 flipped bits")
	}
	if len(diffDrift.NewElements) != 1 || diffDrift.NewElements[0] != "modal_err" {
		t.Errorf("expected new element modal_err, got: %v", diffDrift.NewElements)
	}
}

func TestVision_SimulateTrajectoryVision(t *testing.T) {
	session := &trace.Session{
		ID:        "session_sim_vision",
		AgentName: "vision-agent",
		CreatedAt: time.Now(),
		Events: []trace.TraceEvent{
			{
				Index: 1,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool": "web_search",
				},
			},
			{
				Index: 2,
				Type:  trace.EventToolCall,
				Payload: map[string]interface{}{
					"tool": "click_submit",
				},
			},
		},
	}

	rec, report := vision.SimulateTrajectoryVision(session)

	if report.TotalFrames != 2 {
		t.Fatalf("expected 2 frames, got %d", report.TotalFrames)
	}
	if report.AttestedFrames != 2 {
		t.Errorf("expected 2 attested frames, got %d", report.AttestedFrames)
	}
	if !report.Passed {
		t.Errorf("expected simulation to pass visual attestation: %+v", report)
	}

	scorecard := vision.RenderVisionScorecard(report)
	if !strings.Contains(scorecard, "AGENTVCR MULTI-MODAL VISUAL STATE") {
		t.Errorf("scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "[PASS]") {
		t.Errorf("scorecard should display PASS: %s", scorecard)
	}

	_ = rec
}
