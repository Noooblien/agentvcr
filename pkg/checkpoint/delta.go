package checkpoint

import (
	"fmt"
	"reflect"
	"strings"
	"time"
)

// DeltaCompressor extracts diffs between a baseline session image and a subsequent turn checkpoint.
type DeltaCompressor struct{}

// NewDeltaCompressor initializes a delta compressor.
func NewDeltaCompressor() *DeltaCompressor {
	return &DeltaCompressor{}
}

// ComputeDelta generates a lightweight delta image representing only what changed since the base checkpoint.
func (dc *DeltaCompressor) ComputeDelta(base *SessionImage, current *SessionImage) *SessionImage {
	if base == nil || current == nil {
		return current
	}

	deltaImage := &SessionImage{
		ImageID:         current.ImageID,
		SessionID:       current.SessionID,
		AgentID:         current.AgentID,
		StepIndex:       current.StepIndex,
		State:           current.State,
		CreatedAt:       current.CreatedAt,
		DeltaBaseID:     base.ImageID,
		PRNGSeed:        current.PRNGSeed,
		VirtualClockNow: current.VirtualClockNow,
		Metadata:        make(map[string]interface{}),
	}

	// 1. Context Frames: Store only appended frames
	if len(current.ContextFrames) > len(base.ContextFrames) {
		deltaImage.ContextFrames = current.ContextFrames[len(base.ContextFrames):]
	} else {
		deltaImage.ContextFrames = []AgentContextFrame{}
	}

	// 2. Working Memory: Store only added or mutated variables
	deltaMemory := make(map[string]interface{})
	for k, v := range current.WorkingMemory {
		baseVal, exists := base.WorkingMemory[k]
		if !exists || !reflect.DeepEqual(baseVal, v) {
			deltaMemory[k] = v
		}
	}
	deltaImage.WorkingMemory = deltaMemory

	// 3. Tool Cursors: Store active tool cursors
	deltaImage.ToolCursors = current.ToolCursors

	return deltaImage
}

// Reconstruct combines a baseline image with a delta image to produce the full restored session state.
func (dc *DeltaCompressor) Reconstruct(base *SessionImage, delta *SessionImage) (*SessionImage, error) {
	if base == nil || delta == nil {
		return nil, fmt.Errorf("cannot reconstruct: base or delta image is nil")
	}

	if delta.DeltaBaseID != "" && delta.DeltaBaseID != base.ImageID {
		return nil, fmt.Errorf("delta base mismatch: expected base %s, got %s", delta.DeltaBaseID, base.ImageID)
	}

	reconstructed := &SessionImage{
		ImageID:         delta.ImageID,
		SessionID:       delta.SessionID,
		AgentID:         delta.AgentID,
		StepIndex:       delta.StepIndex,
		State:           StateRestored,
		CreatedAt:       time.Now(),
		PRNGSeed:        delta.PRNGSeed,
		VirtualClockNow: delta.VirtualClockNow,
		ToolCursors:     delta.ToolCursors,
		Metadata:        make(map[string]interface{}),
	}

	// 1. Rebuild Context Frames (Base + Appended)
	reconstructed.ContextFrames = make([]AgentContextFrame, 0, len(base.ContextFrames)+len(delta.ContextFrames))
	reconstructed.ContextFrames = append(reconstructed.ContextFrames, base.ContextFrames...)
	reconstructed.ContextFrames = append(reconstructed.ContextFrames, delta.ContextFrames...)

	// 2. Rebuild Working Memory (Base overwritten with Delta mutations)
	reconstructed.WorkingMemory = make(map[string]interface{})
	for k, v := range base.WorkingMemory {
		reconstructed.WorkingMemory[k] = v
	}
	for k, v := range delta.WorkingMemory {
		// If key indicates deletion (e.g. nil or __deleted__), remove it
		if v == nil || strings.EqualFold(fmt.Sprintf("%v", v), "__deleted__") {
			delete(reconstructed.WorkingMemory, k)
		} else {
			reconstructed.WorkingMemory[k] = v
		}
	}

	return reconstructed, nil
}
