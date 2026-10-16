package vision

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/png"
	"math"
	"math/bits"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Recorder manages frame-accurate visual screen capture, perceptual hashing, and grounding attestations.
type Recorder struct {
	mu          sync.RWMutex
	sessionID   string
	frames      map[int]*VisualFrame // indexed by step index
	frameOrder  []int
}

// NewRecorder initializes a new vision recorder for an execution session.
func NewRecorder(sessionID string) *Recorder {
	return &Recorder{
		sessionID:  sessionID,
		frames:     make(map[int]*VisualFrame),
		frameOrder: make([]int, 0),
	}
}

// ComputePerceptualHash calculates a 64-bit difference hash (dHash) from image bytes.
// dHash tracks horizontal gradients, remaining robust against slight compression or noise.
func ComputePerceptualHash(img image.Image) (string, uint64) {
	if img == nil {
		return "0000000000000000", 0
	}

	// Resize to 9x8 grayscale to compare 8 horizontal pairs across 8 rows = 64 bits
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()

	var grayMatrix [8][9]uint8
	for y := 0; y < 8; y++ {
		origY := bounds.Min.Y + (y * h / 8)
		for x := 0; x < 9; x++ {
			origX := bounds.Min.X + (x * w / 9)
			c := img.At(origX, origY)
			r, g, b, _ := c.RGBA()
			// Standard luminance conversion
			lum := uint8((r*299 + g*587 + b*114) / 1000 >> 8)
			grayMatrix[y][x] = lum
		}
	}

	var hash uint64
	bitIdx := 0
	for y := 0; y < 8; y++ {
		for x := 0; x < 8; x++ {
			if grayMatrix[y][x] > grayMatrix[y][x+1] {
				hash |= (1 << bitIdx)
			}
			bitIdx++
		}
	}

	return fmt.Sprintf("%016x", hash), hash
}

// HammingDistance calculates the bitwise difference between two 64-bit perceptual hashes.
func HammingDistance(h1, h2 string) int {
	v1, err1 := strconv.ParseUint(h1, 16, 64)
	v2, err2 := strconv.ParseUint(h2, 16, 64)
	if err1 != nil || err2 != nil {
		return 64
	}
	return bits.OnesCount64(v1 ^ v2)
}

// GenerateSyntheticFrame synthesizes a valid PNG image with simulated UI elements for offline determinism.
func GenerateSyntheticFrame(width, height int, elements []VisualElement) ([]byte, image.Image) {
	img := image.NewRGBA(image.Rect(0, 0, width, height))

	// Background
	draw.Draw(img, img.Bounds(), &image.Uniform{color.RGBA{240, 242, 245, 255}}, image.Point{}, draw.Src)

	// Draw elements
	for idx, el := range elements {
		b := el.Bounds
		// Assign unique pseudo-colors per tag
		var col color.RGBA
		switch el.Tag {
		case "button":
			col = color.RGBA{59, 130, 246, 255} // Blue
		case "input":
			col = color.RGBA{255, 255, 255, 255} // White box
		case "modal":
			col = color.RGBA{229, 231, 235, 255} // Gray
		case "error":
			col = color.RGBA{239, 68, 68, 255} // Red
		default:
			col = color.RGBA{uint8(60 + (idx*30)%180), uint8(100 + (idx*40)%140), 200, 255}
		}

		elementRect := image.Rect(b.X, b.Y, b.X+b.Width, b.Y+b.Height)
		draw.Draw(img, elementRect, &image.Uniform{col}, image.Point{}, draw.Src)
	}

	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes(), img
}

// RecordFrame stores a visual screen frame for a given trajectory step index.
func (r *Recorder) RecordFrame(stepIndex int, width, height int, data []byte, img image.Image, elements []VisualElement) *VisualFrame {
	r.mu.Lock()
	defer r.mu.Unlock()

	sha := sha256.Sum256(data)
	shaHex := hex.EncodeToString(sha[:])

	pHashStr, _ := ComputePerceptualHash(img)

	frame := &VisualFrame{
		FrameID:        fmt.Sprintf("frame_%s_step%d", r.sessionID, stepIndex),
		SessionID:      r.sessionID,
		StepIndex:      stepIndex,
		Timestamp:      time.Now(),
		Width:          width,
		Height:         height,
		Format:         "png",
		Data:           data,
		SHA256:         shaHex,
		PerceptualHash: pHashStr,
		Elements:       elements,
	}

	if _, exists := r.frames[stepIndex]; !exists {
		r.frameOrder = append(r.frameOrder, stepIndex)
	}
	r.frames[stepIndex] = frame
	return frame
}

// AttestAction coordinates spatial grounding between an action event and the recorded visual state.
func (r *Recorder) AttestAction(stepIndex int, actionType string, targetCoord [2]int, targetTagOrLabel string) *GroundingAttestation {
	r.mu.Lock()
	defer r.mu.Unlock()

	frame, exists := r.frames[stepIndex]
	if !exists {
		return &GroundingAttestation{
			AttestationID:  fmt.Sprintf("attest_%s_%d", r.sessionID, stepIndex),
			StepIndex:      stepIndex,
			ActionType:     actionType,
			TargetCoord:    targetCoord,
			InBounds:       false,
			GroundingScore: 0.0,
			Attested:       false,
			Timestamp:      time.Now(),
		}
	}

	x, y := targetCoord[0], targetCoord[1]
	var hitElement *VisualElement

	// Test bounding boxes
	for i := range frame.Elements {
		el := &frame.Elements[i]
		b := el.Bounds
		if x >= b.X && x <= (b.X+b.Width) && y >= b.Y && y <= (b.Y+b.Height) {
			hitElement = el
			break
		}
	}

	inBounds := false
	groundingScore := 0.0
	hitID := "none"

	if hitElement != nil {
		hitID = hitElement.ID
		// Check label or tag compatibility
		labelMatch := strings.EqualFold(hitElement.Label, targetTagOrLabel) ||
			strings.EqualFold(hitElement.Tag, targetTagOrLabel) ||
			strings.EqualFold(hitElement.ID, targetTagOrLabel)

		if labelMatch || targetTagOrLabel == "" {
			inBounds = true
			groundingScore = 1.0
		} else {
			// Clicked inside an element, but it's the wrong target element (misgrounded click)
			inBounds = false
			groundingScore = 0.3
		}
	} else {
		// Missed all known bounding boxes (clicked blank space)
		inBounds = false
		groundingScore = 0.0
	}

	att := &GroundingAttestation{
		AttestationID:  fmt.Sprintf("attest_%s_%d", r.sessionID, stepIndex),
		StepIndex:      stepIndex,
		ActionType:     actionType,
		TargetElement:  targetTagOrLabel,
		TargetCoord:    targetCoord,
		HitElementID:   hitID,
		InBounds:       inBounds,
		PerceptualHash: frame.PerceptualHash,
		FrameChecksum:  frame.SHA256,
		GroundingScore: groundingScore,
		Attested:       inBounds,
		Timestamp:      time.Now(),
	}

	frame.Attestation = att
	return att
}

// CompareFrames computes perceptual visual drift between two captured frames.
func CompareFrames(f1, f2 *VisualFrame) VisualDiff {
	dist := HammingDistance(f1.PerceptualHash, f2.PerceptualHash)
	similarity := math.Max(0.0, 1.0-(float64(dist)/64.0))

	// Find changed bounding elements
	f1Map := make(map[string]VisualElement)
	for _, el := range f1.Elements {
		f1Map[el.ID] = el
	}

	var newElements []string
	var removedElements []string
	var changedRegions []BoundingBox

	for _, el2 := range f2.Elements {
		if el1, exists := f1Map[el2.ID]; !exists {
			newElements = append(newElements, el2.ID)
			changedRegions = append(changedRegions, el2.Bounds)
		} else if el1.Bounds != el2.Bounds || el1.Label != el2.Label {
			changedRegions = append(changedRegions, el2.Bounds)
		}
	}

	f2Map := make(map[string]bool)
	for _, el := range f2.Elements {
		f2Map[el.ID] = true
	}
	for _, el1 := range f1.Elements {
		if !f2Map[el1.ID] {
			removedElements = append(removedElements, el1.ID)
			changedRegions = append(changedRegions, el1.Bounds)
		}
	}

	return VisualDiff{
		BaseFrameID:     f1.FrameID,
		TargetFrameID:   f2.FrameID,
		HammingDistance: dist,
		SimilarityRatio: similarity,
		ChangedRegions:  changedRegions,
		NewElements:     newElements,
		RemovedElements: removedElements,
		IsVisualDrift:   dist > 12, // More than ~20% perceptual bit flips indicates major visual divergence
	}
}

// AuditSessionVision evaluates multi-modal visual alignment across session events and recorded frames.
func (r *Recorder) AuditSessionVision(session *trace.Session) *VisionAuditReport {
	r.mu.RLock()
	defer r.mu.RUnlock()

	report := &VisionAuditReport{
		SessionID:    session.ID,
		TotalFrames:  len(r.frames),
		Attestations: make([]GroundingAttestation, 0),
		Violations:   make([]string, 0),
	}

	var scoreSum float64

	for _, step := range r.frameOrder {
		f := r.frames[step]
		if f.Attestation != nil {
			report.Attestations = append(report.Attestations, *f.Attestation)
			scoreSum += f.Attestation.GroundingScore
			if f.Attestation.Attested {
				report.AttestedFrames++
			} else {
				report.MisgroundedActions++
				report.Violations = append(report.Violations,
					fmt.Sprintf("Step %d (%s at [%d,%d]): target '%s' failed boundary attestation (hit: '%s')",
						f.StepIndex, f.Attestation.ActionType,
						f.Attestation.TargetCoord[0], f.Attestation.TargetCoord[1],
						f.Attestation.TargetElement, f.Attestation.HitElementID))
			}
		}
	}

	if len(report.Attestations) > 0 {
		report.MeanGroundingScore = scoreSum / float64(len(report.Attestations))
	} else {
		report.MeanGroundingScore = 1.0
	}

	// Check consecutive frame drift
	for i := 0; i < len(r.frameOrder)-1; i++ {
		f1 := r.frames[r.frameOrder[i]]
		f2 := r.frames[r.frameOrder[i+1]]
		diff := CompareFrames(f1, f2)
		if diff.IsVisualDrift {
			report.VisualDriftDetected = true
			report.Violations = append(report.Violations,
				fmt.Sprintf("Uncontrolled visual drift detected between step %d and step %d (Hamming distance: %d/64)",
					f1.StepIndex, f2.StepIndex, diff.HammingDistance))
		}
	}

	report.Passed = report.MisgroundedActions == 0 && !report.VisualDriftDetected
	return report
}

// SimulateTrajectoryVision runs an end-to-end multi-modal visual grounding simulation for an agent trace.
func SimulateTrajectoryVision(session *trace.Session) (*Recorder, *VisionAuditReport) {
	rec := NewRecorder(session.ID)

	standardElements := []VisualElement{
		{ID: "btn_submit", Tag: "button", Label: "Submit Order", Confidence: 0.98, Bounds: BoundingBox{X: 100, Y: 200, Width: 120, Height: 40}},
		{ID: "input_search", Tag: "input", Label: "Search Query", Confidence: 0.95, Bounds: BoundingBox{X: 100, Y: 50, Width: 300, Height: 35}},
		{ID: "btn_cancel", Tag: "button", Label: "Cancel", Confidence: 0.92, Bounds: BoundingBox{X: 250, Y: 200, Width: 80, Height: 40}},
	}

	for _, ev := range session.Events {
		// Synthesize frame for this step
		rawPNG, img := GenerateSyntheticFrame(800, 600, standardElements)
		rec.RecordFrame(ev.Index, 800, 600, rawPNG, img, standardElements)

		// If event is a tool call representing a GUI action
		actionName := "click"
		coord := [2]int{150, 220} // Inside btn_submit
		targetLabel := "btn_submit"

		if ev.Type == trace.EventToolCall {
			if tool, ok := ev.Payload["tool"].(string); ok {
				actionName = tool
			}
			if strings.Contains(strings.ToLower(actionName), "search") {
				coord = [2]int{150, 65} // Inside input_search
				targetLabel = "input_search"
			}
		}

		rec.AttestAction(ev.Index, actionName, coord, targetLabel)
	}

	report := rec.AuditSessionVision(session)
	return rec, report
}

// RenderVisionScorecard generates an ANSI terminal visual grounding scorecard.
func RenderVisionScorecard(report *VisionAuditReport) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString(fmt.Sprintf("\033[1;37m   AGENTVCR MULTI-MODAL VISUAL STATE & SCREEN GROUNDING ATTESTATION\033[0m\n"))
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")

	sb.WriteString(fmt.Sprintf("  \033[1;33mSession ID:\033[0m            %s\n", report.SessionID))
	sb.WriteString(fmt.Sprintf("  \033[1;33mSynchronized Frames:\033[0m   %d frames captured\n", report.TotalFrames))
	sb.WriteString(fmt.Sprintf("  \033[1;33mGrounding Score:\033[0m       \033[1;35m%.1f%%\033[0m mean accuracy across %d actions\n",
		report.MeanGroundingScore*100.0, len(report.Attestations)))

	if report.Passed {
		sb.WriteString("  \033[1;33mAttestation Status:\033[0m    \033[1;32m[PASS] ZERO VISUAL DRIFT & PERFECT SCREEN GROUNDING\033[0m\n")
	} else {
		sb.WriteString(fmt.Sprintf("  \033[1;33mAttestation Status:\033[0m    \033[1;31m[FAIL] %d MISGROUNDED ACTIONS / VISUAL DRIFT DETECTED\033[0m\n",
			report.MisgroundedActions))
	}
	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")

	sb.WriteString("\033[1;37m  FRAME-ACCURATE ACTION GROUNDING ATTESTATIONS:\033[0m\n")
	sb.WriteString("  \033[1;30mSTEP  ACTION       TARGET ELEMENT     COORDINATES   HIT ELEMENT    STATUS    SCORE\033[0m\n")

	for _, a := range report.Attestations {
		statusStr := "\033[1;32mATTESTED\033[0m"
		if !a.Attested {
			statusStr = "\033[1;31mFAILED\033[0m  "
		}
		target := a.TargetElement
		if len(target) > 18 {
			target = target[:15] + "..."
		}
		hit := a.HitElementID
		if len(hit) > 14 {
			hit = hit[:11] + "..."
		}

		sb.WriteString(fmt.Sprintf("  #%-4d %-12s %-18s [%-4d, %-4d] %-14s %s  %4.0f%%\n",
			a.StepIndex, a.ActionType, target, a.TargetCoord[0], a.TargetCoord[1], hit, statusStr, a.GroundingScore*100))
	}

	if len(report.Violations) > 0 {
		sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")
		sb.WriteString("\033[1;31m  GROUNDING VIOLATIONS & VISUAL DRIFT HAZARDS:\033[0m\n")
		for _, v := range report.Violations {
			sb.WriteString(fmt.Sprintf("  \033[1;31m[!] %s\033[0m\n", v))
		}
	}

	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")
	return sb.String()
}
