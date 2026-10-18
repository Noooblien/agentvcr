package tree

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// NodeStatus defines the lifecycle state of a branch node.
type NodeStatus string

const (
	NodeStatusActive          NodeStatus = "active"
	NodeStatusPruned          NodeStatus = "pruned"
	NodeStatusTerminalSuccess NodeStatus = "terminal_success"
	NodeStatusTerminalFailed  NodeStatus = "terminal_failed"
	NodeStatusCandidate       NodeStatus = "candidate"
)

// PruneReason specifies why a trajectory branch was pruned.
type PruneReason string

const (
	PruneReasonNone           PruneReason = "none"
	PruneReasonBudgetExceeded PruneReason = "budget_exceeded"
	PruneReasonLowConfidence  PruneReason = "low_confidence"
	PruneReasonError          PruneReason = "error"
	PruneReasonHallucination  PruneReason = "hallucination"
	PruneReasonLoopDetected   PruneReason = "loop_detected"
)

// SearchPolicy designates the multi-hypothesis tree exploration strategy.
type SearchPolicy string

const (
	PolicyBeamSearch SearchPolicy = "beam_search"
	PolicyBestFirst  SearchPolicy = "best_first"
	PolicyMCTS       SearchPolicy = "mcts"
	PolicyDFS        SearchPolicy = "dfs"
)

// TreeNode represents a single atomic turn/decision node in the trajectory exploration tree.
type TreeNode struct {
	ID               string                 `json:"id"`
	ParentID         string                 `json:"parent_id,omitempty"`
	Depth            int                    `json:"depth"`
	Role             string                 `json:"role"` // system, user, assistant, tool, evaluator
	Content          string                 `json:"content"`
	Event            trace.TraceEvent       `json:"event"`
	Score            float64                `json:"score"`             // Local heuristic or judge evaluation (0.0 - 10.0)
	CumulativeScore  float64                `json:"cumulative_score"`  // Sum of scores along root-to-node path
	TokenUsage       int                    `json:"token_usage"`       // Tokens consumed at this step
	CumulativeTokens int                    `json:"cumulative_tokens"` // Total tokens along root-to-node path
	Status           NodeStatus             `json:"status"`
	PruneReason      PruneReason            `json:"prune_reason,omitempty"`
	ContentHash      string                 `json:"content_hash"`
	MerkleHash       string                 `json:"merkle_hash"`
	Children         []*TreeNode            `json:"children,omitempty"`
	Metadata         map[string]string      `json:"metadata,omitempty"`
	Visits           int                    `json:"visits,omitempty"` // For MCTS / search policies
	Value            float64                `json:"value,omitempty"`  // Aggregated value for MCTS
	CreatedAt        time.Time              `json:"created_at"`
}

// BranchPath represents an end-to-end trajectory hypothesis through the tree.
type BranchPath struct {
	LeafID       string      `json:"leaf_id"`
	Nodes        []*TreeNode `json:"nodes"`
	TotalScore   float64     `json:"total_score"`
	AverageScore float64     `json:"average_score"`
	TotalTokens  int         `json:"total_tokens"`
	Status       NodeStatus  `json:"status"`
	MerkleChain  []string    `json:"merkle_chain"`
}

// SearchOptions configures tree exploration parameters.
type SearchOptions struct {
	BeamWidth         int          `json:"beam_width"`
	MaxDepth          int          `json:"max_depth"`
	MinScoreThreshold float64      `json:"min_score_threshold"`
	MaxTokenBudget    int          `json:"max_token_budget"`
	Policy            SearchPolicy `json:"policy"`
}

// ExpansionCandidate represents a prospective branch expansion step.
type ExpansionCandidate struct {
	Content    string                 `json:"content"`
	Role       string                 `json:"role"`
	EventType  trace.EventType        `json:"event_type"`
	Payload    map[string]interface{} `json:"payload"`
	Score      float64                `json:"score"`
	TokenUsage int                    `json:"token_usage"`
	Metadata   map[string]string      `json:"metadata,omitempty"`
}

// TrajectoryTree holds the full multi-hypothesis branching DAG with Merkle attestation.
type TrajectoryTree struct {
	ID             string                `json:"id"`
	Root           *TreeNode             `json:"root"`
	NodesByID      map[string]*TreeNode  `json:"nodes_by_id"`
	MaxDepth       int                   `json:"max_depth"`
	BeamWidth      int                   `json:"beam_width"`
	Policy         SearchPolicy          `json:"policy"`
	OptimalPath    *BranchPath           `json:"optimal_path,omitempty"`
	TotalNodes     int                   `json:"total_nodes"`
	TotalPruned    int                   `json:"total_pruned"`
	TotalExpanded  int                   `json:"total_expanded"`
	RootMerkleHash string                `json:"root_merkle_hash"`
	Metadata       map[string]string     `json:"metadata,omitempty"`
}
