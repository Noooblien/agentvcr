package vision

import (
	"time"
)

// BoundingBox represents rectangular screen coordinates for a visual element or action.
type BoundingBox struct {
	X      int `json:"x"`
	Y      int `json:"y"`
	Width  int `json:"width"`
	Height int `json:"height"`
}

// VisualElement represents a detected or annotated UI element in a frame.
type VisualElement struct {
	ID         string      `json:"id"`
	Tag        string      `json:"tag"`        // e.g. "button", "input", "modal", "window", "link"
	Label      string      `json:"label"`      // OCR or accessibility text label
	Confidence float64     `json:"confidence"` // 0.0 - 1.0 visual recognition confidence
	Bounds     BoundingBox `json:"bounds"`     // Screen coordinates
	Attributes map[string]string `json:"attributes,omitempty"`
}

// GroundingAttestation records cryptographic and spatial alignment between an agent action and visual state.
type GroundingAttestation struct {
	AttestationID  string      `json:"attestation_id"`
	StepIndex      int         `json:"step_index"`
	ActionType     string      `json:"action_type"`      // e.g. "click", "double_click", "type", "drag"
	TargetElement  string      `json:"target_element"`   // Matched element ID or label
	TargetCoord    [2]int      `json:"target_coord"`     // (X, Y) target coordinates
	HitElementID   string      `json:"hit_element_id"`   // Element bounded at coordinates
	InBounds       bool        `json:"in_bounds"`        // Whether click/interaction fell within target element bounds
	PerceptualHash string      `json:"perceptual_hash"`  // 64-bit dHash / pHash of the screen frame
	FrameChecksum  string      `json:"frame_checksum"`   // SHA-256 binary hash of the frame data
	GroundingScore float64     `json:"grounding_score"`  // 0.0 to 1.0 confidence/accuracy score
	Attested       bool        `json:"attested"`         // True if spatial & cryptographic checks passed
	Timestamp      time.Time   `json:"timestamp"`
}

// VisualFrame represents a single frame-accurate visual screen capture synchronized with trajectory steps.
type VisualFrame struct {
	FrameID        string          `json:"frame_id"`
	SessionID      string          `json:"session_id"`
	StepIndex      int             `json:"step_index"`
	Timestamp      time.Time       `json:"timestamp"`
	Width          int             `json:"width"`
	Height         int             `json:"height"`
	Format         string          `json:"format"`          // "png", "jpeg", "rgb_raw"
	Data           []byte          `json:"-"`               // Raw frame image payload
	SHA256         string          `json:"sha256"`          // Binary SHA-256
	PerceptualHash string          `json:"perceptual_hash"` // Perceptual visual hash for similarity diffing
	Elements       []VisualElement `json:"elements"`        // Extracted visual elements
	Attestation    *GroundingAttestation `json:"attestation,omitempty"`
}

// VisualDiff records spatial visual changes and screen deltas between two frames.
type VisualDiff struct {
	BaseFrameID      string        `json:"base_frame_id"`
	TargetFrameID    string        `json:"target_frame_id"`
	HammingDistance  int           `json:"hamming_distance"`  // dHash distance (0 = identical, >10 = significant change)
	SimilarityRatio  float64       `json:"similarity_ratio"` // 1.0 - (dist / 64)
	ChangedRegions   []BoundingBox `json:"changed_regions"`  // Bounding boxes where pixels altered
	NewElements      []string      `json:"new_elements"`
	RemovedElements  []string      `json:"removed_elements"`
	IsVisualDrift    bool          `json:"is_visual_drift"`
}

// VisionAuditReport summarizes multi-modal visual state grounding across an agent trajectory.
type VisionAuditReport struct {
	SessionID            string                  `json:"session_id"`
	TotalFrames          int                     `json:"total_frames"`
	AttestedFrames       int                     `json:"attested_frames"`
	MisgroundedActions   int                     `json:"misgrounded_actions"`
	VisualDriftDetected  bool                    `json:"visual_drift_detected"`
	MeanGroundingScore   float64                 `json:"mean_grounding_score"`
	Attestations         []GroundingAttestation  `json:"attestations"`
	Violations           []string                `json:"violations"`
	Passed               bool                    `json:"passed"`
}
