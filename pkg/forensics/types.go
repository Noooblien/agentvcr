package forensics

// ClaimStatus classifies the factual grounding of an agent-generated claim against retrieved context.
type ClaimStatus string

const (
	StatusFaithful     ClaimStatus = "faithful"      // Well-grounded in retrieved documents
	StatusHallucinated ClaimStatus = "hallucinated"  // Made up claim not supported by retrieved context
	StatusUngrounded   ClaimStatus = "ungrounded"    // Low similarity / missing attribution
	StatusContradiction ClaimStatus = "contradiction" // Explicitly conflicts with retrieved source fact
)

// RetrievedDoc represents a context chunk, document, or search result retrieved by a tool.
type RetrievedDoc struct {
	ID        string  `json:"id"`
	Source    string  `json:"source"`
	Content   string  `json:"content"`
	StepIndex int     `json:"step_index"`
	ToolName  string  `json:"tool_name"`
	Relevance float64 `json:"relevance,omitempty"`
}

// ClaimAnalysis details the grounding, similarity, and attribution of an individual generated statement.
type ClaimAnalysis struct {
	ClaimText         string      `json:"claim_text"`
	StepIndex         int         `json:"step_index"`
	GroundingScore    float64     `json:"grounding_score"` // Cosine similarity to closest retrieved evidence (0.0 to 1.0)
	BestMatchingDocID string      `json:"best_matching_doc_id,omitempty"`
	BestEvidence      string      `json:"best_evidence,omitempty"`
	Status            ClaimStatus `json:"status"`
	RiskSeverity      string      `json:"risk_severity"` // "none", "medium", "critical"
	Explanation       string      `json:"explanation"`
}

// ForensicAuditReport encapsulates aggregate RAG faithfulness and hallucination metrics.
type ForensicAuditReport struct {
	SessionID                string          `json:"session_id"`
	TotalRetrievalEvents     int             `json:"total_retrieval_events"`
	TotalDocsRetrieved       int             `json:"total_docs_retrieved"`
	TotalClaimsEvaluated     int             `json:"total_claims_evaluated"`
	FaithfulClaimsCount      int             `json:"faithful_claims_count"`
	HallucinatedClaimsCount  int             `json:"hallucinated_claims_count"`
	ContradictionsCount      int             `json:"contradictions_count"`
	OverallFaithfulnessScore float64         `json:"overall_faithfulness_score"` // 0.0 to 1.0
	HallucinationRisk        string          `json:"hallucination_risk"`         // "LOW", "MODERATE", "CRITICAL"
	PollutionRate            float64         `json:"pollution_rate"`             // Fraction of retrieved docs unused by generation
	PassedAudit              bool            `json:"passed_audit"`
	Claims                   []ClaimAnalysis `json:"claims"`
	RetrievedSources         []RetrievedDoc  `json:"retrieved_sources"`
}
