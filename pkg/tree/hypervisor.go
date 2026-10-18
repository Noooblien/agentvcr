package tree

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// NewTrajectoryTree initializes a multi-hypothesis trajectory tree with a root query/prompt.
func NewTrajectoryTree(id string, rootPrompt string, metadata map[string]string) *TrajectoryTree {
	now := time.Now()
	rootEvent := trace.TraceEvent{
		ID:        fmt.Sprintf("%s-root-evt", id),
		Index:     0,
		Timestamp: now,
		Type:      trace.EventLLMRequest,
		Payload: map[string]interface{}{
			"prompt": rootPrompt,
		},
	}

	rootNode := &TreeNode{
		ID:               fmt.Sprintf("%s-node-root", id),
		ParentID:         "",
		Depth:            0,
		Role:             "user",
		Content:          rootPrompt,
		Event:            rootEvent,
		Score:            10.0,
		CumulativeScore:  10.0,
		TokenUsage:       len(rootPrompt) / 4,
		CumulativeTokens: len(rootPrompt) / 4,
		Status:           NodeStatusActive,
		PruneReason:      PruneReasonNone,
		Children:         make([]*TreeNode, 0),
		Metadata:         metadata,
		CreatedAt:        now,
	}

	t := &TrajectoryTree{
		ID:            id,
		Root:          rootNode,
		NodesByID:     make(map[string]*TreeNode),
		MaxDepth:      0,
		BeamWidth:     3,
		Policy:        PolicyBeamSearch,
		TotalNodes:    1,
		TotalPruned:   0,
		TotalExpanded: 1,
		Metadata:      metadata,
	}

	t.NodesByID[rootNode.ID] = rootNode
	t.UpdateMerkleHashes()
	return t
}

// AddChild creates and links a new child branch to a parent node in the tree.
func (t *TrajectoryTree) AddChild(
	parentID string,
	childID string,
	role string,
	content string,
	eventType trace.EventType,
	payload map[string]interface{},
	score float64,
	tokenUsage int,
	metadata map[string]string,
) (*TreeNode, error) {
	parent, exists := t.NodesByID[parentID]
	if !exists {
		return nil, fmt.Errorf("parent node %s not found in tree", parentID)
	}

	if childID == "" {
		childID = fmt.Sprintf("%s-d%d-b%d", t.ID, parent.Depth+1, len(parent.Children)+1)
	}

	if _, exists := t.NodesByID[childID]; exists {
		return nil, fmt.Errorf("node with ID %s already exists", childID)
	}

	now := time.Now()
	evt := trace.TraceEvent{
		ID:        fmt.Sprintf("evt-%s", childID),
		Index:     parent.Depth + 1,
		Timestamp: now,
		Type:      eventType,
		Payload:   payload,
	}

	child := &TreeNode{
		ID:               childID,
		ParentID:         parent.ID,
		Depth:            parent.Depth + 1,
		Role:             role,
		Content:          content,
		Event:            evt,
		Score:            score,
		CumulativeScore:  parent.CumulativeScore + score,
		TokenUsage:       tokenUsage,
		CumulativeTokens: parent.CumulativeTokens + tokenUsage,
		Status:           NodeStatusActive,
		PruneReason:      PruneReasonNone,
		Children:         make([]*TreeNode, 0),
		Metadata:         metadata,
		CreatedAt:        now,
	}

	parent.Children = append(parent.Children, child)
	t.NodesByID[child.ID] = child
	t.TotalNodes++

	if child.Depth > t.MaxDepth {
		t.MaxDepth = child.Depth
	}

	t.UpdateMerkleHashes()
	return child, nil
}

// PruneBranch marks a node and all of its descendants as pruned with a designated reason.
func (t *TrajectoryTree) PruneBranch(nodeID string, reason PruneReason) error {
	node, exists := t.NodesByID[nodeID]
	if !exists {
		return fmt.Errorf("node %s not found", nodeID)
	}

	var pruneRecursive func(n *TreeNode)
	pruneRecursive = func(n *TreeNode) {
		if n.Status != NodeStatusPruned {
			n.Status = NodeStatusPruned
			n.PruneReason = reason
			t.TotalPruned++
		}
		for _, child := range n.Children {
			pruneRecursive(child)
		}
	}

	pruneRecursive(node)
	t.UpdateMerkleHashes()
	return nil
}

// DetectLoops checks whether an action/content sequence has been cyclically repeated along the ancestor path.
func (t *TrajectoryTree) DetectLoops(nodeID string) bool {
	node, exists := t.NodesByID[nodeID]
	if !exists {
		return false
	}

	occurrences := make(map[string]int)
	curr := node
	for curr != nil {
		normalized := strings.TrimSpace(strings.ToLower(curr.Content))
		if len(normalized) > 0 {
			occurrences[normalized]++
			if occurrences[normalized] >= 3 {
				return true
			}
		}
		if curr.ParentID == "" {
			break
		}
		curr = t.NodesByID[curr.ParentID]
	}
	return false
}

// GetPath constructs the full ancestral branch path from root to the specified leaf node.
func (t *TrajectoryTree) GetPath(leafID string) (*BranchPath, error) {
	leaf, exists := t.NodesByID[leafID]
	if !exists {
		return nil, fmt.Errorf("leaf node %s not found", leafID)
	}

	pathNodes := make([]*TreeNode, 0)
	curr := leaf
	for curr != nil {
		pathNodes = append([]*TreeNode{curr}, pathNodes...)
		if curr.ParentID == "" {
			break
		}
		curr = t.NodesByID[curr.ParentID]
	}

	var totalScore float64
	var totalTokens int
	merkleChain := make([]string, len(pathNodes))

	for i, n := range pathNodes {
		totalScore += n.Score
		totalTokens += n.TokenUsage
		merkleChain[i] = n.MerkleHash
	}

	avgScore := 0.0
	if len(pathNodes) > 0 {
		avgScore = totalScore / float64(len(pathNodes))
	}

	return &BranchPath{
		LeafID:       leaf.ID,
		Nodes:        pathNodes,
		TotalScore:   totalScore,
		AverageScore: avgScore,
		TotalTokens:  totalTokens,
		Status:       leaf.Status,
		MerkleChain:  merkleChain,
	}, nil
}

// RunBeamSearch executes multi-path beam search over prospective agent trajectories.
func (t *TrajectoryTree) RunBeamSearch(
	expander func(node *TreeNode) []ExpansionCandidate,
	options SearchOptions,
) (*BranchPath, error) {
	t.Policy = PolicyBeamSearch
	t.BeamWidth = options.BeamWidth
	frontier := []*TreeNode{t.Root}

	for depth := 1; depth <= options.MaxDepth; depth++ {
		if len(frontier) == 0 {
			break
		}

		candidatesAtLevel := make([]*TreeNode, 0)

		for _, parent := range frontier {
			if parent.Status == NodeStatusPruned || parent.Status == NodeStatusTerminalSuccess {
				continue
			}

			expansions := expander(parent)
			t.TotalExpanded += len(expansions)

			for i, exp := range expansions {
				childID := fmt.Sprintf("%s-d%d-b%d", t.ID, depth, len(parent.Children)+i+1)
				child, err := t.AddChild(
					parent.ID,
					childID,
					exp.Role,
					exp.Content,
					exp.EventType,
					exp.Payload,
					exp.Score,
					exp.TokenUsage,
					exp.Metadata,
				)
				if err != nil {
					continue
				}

				// Check Pruning Criteria
				if exp.Score < options.MinScoreThreshold {
					_ = t.PruneBranch(child.ID, PruneReasonLowConfidence)
					continue
				}

				if options.MaxTokenBudget > 0 && child.CumulativeTokens > options.MaxTokenBudget {
					_ = t.PruneBranch(child.ID, PruneReasonBudgetExceeded)
					continue
				}

				if t.DetectLoops(child.ID) {
					_ = t.PruneBranch(child.ID, PruneReasonLoopDetected)
					continue
				}

				// Check terminal conditions
				if strings.Contains(strings.ToLower(child.Content), "completed") ||
					strings.Contains(strings.ToLower(child.Content), "success") {
					child.Status = NodeStatusTerminalSuccess
				}

				candidatesAtLevel = append(candidatesAtLevel, child)
			}
		}

		if len(candidatesAtLevel) == 0 {
			break
		}

		// Sort candidates at this depth descending by score (or normalized cumulative score)
		sort.Slice(candidatesAtLevel, func(i, j int) bool {
			// Higher local score prioritized, broken by higher cumulative score
			if candidatesAtLevel[i].Score == candidatesAtLevel[j].Score {
				return candidatesAtLevel[i].CumulativeScore > candidatesAtLevel[j].CumulativeScore
			}
			return candidatesAtLevel[i].Score > candidatesAtLevel[j].Score
		})

		// Keep top BeamWidth candidates, prune the rest
		survivors := make([]*TreeNode, 0, options.BeamWidth)
		for idx, cand := range candidatesAtLevel {
			if idx < options.BeamWidth {
				survivors = append(survivors, cand)
			} else {
				_ = t.PruneBranch(cand.ID, PruneReasonLowConfidence)
			}
		}

		frontier = survivors
	}

	t.UpdateMerkleHashes()
	optimal := t.SelectOptimalPath()
	t.OptimalPath = optimal
	return optimal, nil
}

// SelectOptimalPath discovers the highest scoring active or terminal_success branch path in the tree.
func (t *TrajectoryTree) SelectOptimalPath() *BranchPath {
	var bestLeaf *TreeNode
	var bestScore float64 = -1.0

	for _, node := range t.NodesByID {
		// Only consider leaves or non-pruned terminal/active nodes
		if node.Status == NodeStatusPruned {
			continue
		}

		isLeaf := len(node.Children) == 0
		if isLeaf {
			// Prefer terminal success, then highest cumulative score
			score := node.CumulativeScore
			if node.Status == NodeStatusTerminalSuccess {
				score += 10.0 // Bonus for reaching explicit completion
			}

			if score > bestScore {
				bestScore = score
				bestLeaf = node
			}
		}
	}

	if bestLeaf == nil {
		bestLeaf = t.Root
	}

	path, _ := t.GetPath(bestLeaf.ID)
	return path
}

// ExportSession extracts a verified linear trace.Session from a branch path.
func (t *TrajectoryTree) ExportSession(path *BranchPath) *trace.Session {
	if path == nil || len(path.Nodes) == 0 {
		return nil
	}

	now := time.Now()
	events := make([]trace.TraceEvent, len(path.Nodes))
	var prevChecksum string

	for i, node := range path.Nodes {
		evt := node.Event
		evt.Index = i
		evt.Timestamp = now.Add(time.Duration(i) * time.Second)

		payloadBytes := []byte(fmt.Sprintf("%v", evt.Payload))
		evt.Checksum = trace.ComputeChecksum(prevChecksum, i, evt.Type, payloadBytes)
		prevChecksum = evt.Checksum
		events[i] = evt
	}

	sessionStatus := "completed"
	if path.Status == NodeStatusPruned || path.Status == NodeStatusTerminalFailed {
		sessionStatus = "failed"
	}

	return &trace.Session{
		ID:        fmt.Sprintf("tree-export-%s", t.ID),
		AgentName: fmt.Sprintf("hypervisor-tree-%s", t.Policy),
		CreatedAt: path.Nodes[0].CreatedAt,
		UpdatedAt: now,
		Metadata: map[string]string{
			"tree_id":           t.ID,
			"leaf_id":           path.LeafID,
			"search_policy":     string(t.Policy),
			"root_merkle_hash":  t.RootMerkleHash,
			"total_score":       fmt.Sprintf("%.2f", path.TotalScore),
			"cumulative_tokens": fmt.Sprintf("%d", path.TotalTokens),
		},
		Events: events,
		Status: sessionStatus,
	}
}
