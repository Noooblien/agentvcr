package research

import (
	"time"
)

// SourceType represents the classification of information origin.
type SourceType string

const (
	SourceLocalCode    SourceType = "local_code"
	SourceDocs         SourceType = "documentation"
	SourceTraceSession SourceType = "trace_session"
	SourceWeb          SourceType = "web_knowledge"
)

// NodeType represents the stage/role of a node in the research tree.
type NodeType string

const (
	NodeRoot             NodeType = "root_objective"
	NodeHypothesis       NodeType = "hypothesis_branch"
	NodeEvidenceGather   NodeType = "evidence_gathering"
	NodeFactVerification NodeType = "fact_verification"
	NodeSynthesis        NodeType = "synthesis"
)

// NodeStatus represents the resolution state of a research node.
type NodeStatus string

const (
	StatusPending      NodeStatus = "pending"
	StatusExploring    NodeStatus = "exploring"
	StatusCorroborated NodeStatus = "corroborated"
	StatusContradicted NodeStatus = "contradicted"
	StatusPruned       NodeStatus = "pruned"
	StatusCompleted    NodeStatus = "completed"
)

// Citation encapsulates a verified cryptographic excerpt from a source document.
type Citation struct {
	ID          string     `json:"id"`
	SourceURI   string     `json:"source_uri"`
	SourceType  SourceType `json:"source_type"`
	Snippet     string     `json:"snippet"`
	LineStart   int        `json:"line_start,omitempty"`
	LineEnd     int        `json:"line_end,omitempty"`
	ContentHash string     `json:"content_hash"`
	Relevance   float64    `json:"relevance"`
	Timestamp   time.Time  `json:"timestamp"`
}

// Claim encapsulates an atomic proposition evaluated during research.
type Claim struct {
	ID              string   `json:"id"`
	Statement       string   `json:"statement"`
	Confidence      float64  `json:"confidence"`
	CitationIDs     []string `json:"citation_ids"`
	GroundingScore  float64  `json:"grounding_score"`
	IsHallucination bool     `json:"is_hallucination"`
	ContradictsWith []string `json:"contradicts_with,omitempty"`
}

// Contradiction records conflicting propositions found across sources.
type Contradiction struct {
	ClaimIDA    string `json:"claim_id_a"`
	ClaimIDB    string `json:"claim_id_b"`
	Description string `json:"description"`
	Resolution  string `json:"resolution"`
	Severity    string `json:"severity"` // "Low", "Medium", "High"
}

// ResearchNode is a vertex in the recursive exploration DAG.
type ResearchNode struct {
	ID             string          `json:"id"`
	ParentID       string          `json:"parent_id,omitempty"`
	ChildrenIDs    []string        `json:"children_ids,omitempty"`
	Type           NodeType        `json:"type"`
	Status         NodeStatus      `json:"status"`
	Topic          string          `json:"topic"`
	Depth          int             `json:"depth"`
	Evidence       []Citation      `json:"evidence,omitempty"`
	ExtractedClaims []Claim         `json:"extracted_claims,omitempty"`
	Summary        string          `json:"summary,omitempty"`
}

// ResearchTree models the full hierarchical goal decomposition.
type ResearchTree struct {
	RootID     string                   `json:"root_id"`
	Nodes      map[string]*ResearchNode `json:"nodes"`
	MaxDepth   int                      `json:"max_depth"`
	TotalNodes int                      `json:"total_nodes"`
}

// ResearchConfig defines execution parameters for recursive deep research.
type ResearchConfig struct {
	MaxDepth                    int     `json:"max_depth"`
	MaxBranchesPerNode          int     `json:"max_branches_per_node"`
	GroundingThreshold          float64 `json:"grounding_threshold"`
	EnableContradictionDetection bool    `json:"enable_contradiction_detection"`
	StrictPruning               bool    `json:"strict_pruning"`
}

// DefaultResearchConfig returns balanced enterprise defaults.
func DefaultResearchConfig() ResearchConfig {
	return ResearchConfig{
		MaxDepth:                    3,
		MaxBranchesPerNode:          4,
		GroundingThreshold:          0.55,
		EnableContradictionDetection: true,
		StrictPruning:               true,
	}
}

// Document represents an input corpus item for analysis.
type Document struct {
	URI        string     `json:"uri"`
	Content    string     `json:"content"`
	SourceType SourceType `json:"source_type"`
}

// ResearchReport aggregates verified findings, citations, and executive brief.
type ResearchReport struct {
	Query             string          `json:"query"`
	GeneratedAt       time.Time       `json:"generated_at"`
	DurationMs        float64         `json:"duration_ms"`
	Tree              *ResearchTree   `json:"tree"`
	ExecutiveSummary  string          `json:"executive_summary"`
	VerifiedClaims    []Claim         `json:"verified_claims"`
	PrunedClaims      []Claim         `json:"pruned_claims"`
	Citations         []Citation      `json:"citations"`
	Contradictions    []Contradiction `json:"contradictions"`
	OverallConfidence float64         `json:"overall_confidence"`
	GroundingRatePct  float64         `json:"grounding_rate_pct"`
	PassedAudit       bool            `json:"passed_audit"`
}
