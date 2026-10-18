package tests

import (
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/trace"
	"github.com/Noooblien/agentvcr/pkg/tree"
)

func TestTree_InitializationAndAddChild(t *testing.T) {
	tr := tree.NewTrajectoryTree("tree-init-test", "Resolve high memory usage in prod-ingress", map[string]string{
		"env": "production",
	})

	if tr.Root == nil {
		t.Fatalf("Expected non-nil root")
	}
	if tr.Root.Role != "user" || tr.TotalNodes != 1 {
		t.Fatalf("Unexpected root: role=%s, totalNodes=%d", tr.Root.Role, tr.TotalNodes)
	}
	if tr.RootMerkleHash == "" {
		t.Fatalf("Expected root merkle hash to be computed")
	}

	child1, err := tr.AddChild(
		tr.Root.ID,
		"node-d1-a",
		"assistant",
		"Run kubectl top pods to identify top memory consumer",
		trace.EventToolCall,
		map[string]interface{}{"command": "kubectl top pods"},
		9.2,
		45,
		nil,
	)
	if err != nil {
		t.Fatalf("Failed to add child: %v", err)
	}

	if child1.Depth != 1 || child1.ParentID != tr.Root.ID {
		t.Fatalf("Unexpected child state: depth=%d, parent=%s", child1.Depth, child1.ParentID)
	}
	if tr.MaxDepth != 1 || tr.TotalNodes != 2 {
		t.Fatalf("Unexpected tree stats: maxDepth=%d, totalNodes=%d", tr.MaxDepth, tr.TotalNodes)
	}

	// Verify Merkle hash updated
	oldRootHash := tr.RootMerkleHash
	if oldRootHash == "" {
		t.Fatalf("Expected root merkle hash to exist")
	}
}

func TestTree_MerkleProofVerificationAndTamperDetection(t *testing.T) {
	tr := tree.NewTrajectoryTree("tree-merkle-test", "Investigate database latency spike", nil)

	c1, err := tr.AddChild(tr.Root.ID, "node-1", "assistant", "Check slow query log", trace.EventToolCall, nil, 8.5, 30, nil)
	if err != nil {
		t.Fatalf("Error adding c1: %v", err)
	}

	c2, err := tr.AddChild(c1.ID, "node-2", "tool", "Slow query found: SELECT * FROM orders WHERE status = 'pending'", trace.EventToolResult, nil, 9.0, 50, nil)
	if err != nil {
		t.Fatalf("Error adding c2: %v", err)
	}

	// Verify valid Merkle proof
	valid, proof, err := tr.VerifyMerkleProof(c2.ID)
	if err != nil || !valid {
		t.Fatalf("Expected valid Merkle proof, got valid=%v, err=%v", valid, err)
	}
	if len(proof) != 3 { // c2, c1, root
		t.Fatalf("Expected 3 proof elements, got %d", len(proof))
	}

	// Tamper test: Alter node-1 content hash directly
	c1.ContentHash = "tampered-content-hash"
	validTampered, _, _ := tr.VerifyMerkleProof(c2.ID)
	if validTampered {
		t.Fatalf("Expected Merkle proof verification to FAIL after tampering, but got valid=true")
	}
}

func TestTree_SubtreeDeduplication(t *testing.T) {
	// Construct two separate trees sharing an identical execution subtree
	treeA := tree.NewTrajectoryTree("tree-a", "Shared query", nil)
	_, _ = treeA.AddChild(treeA.Root.ID, "node-shared", "assistant", "Identical action", trace.EventToolCall, map[string]interface{}{"k": "v"}, 9.0, 20, nil)

	treeB := tree.NewTrajectoryTree("tree-b", "Shared query", nil)
	_, _ = treeB.AddChild(treeB.Root.ID, "node-shared", "assistant", "Identical action", trace.EventToolCall, map[string]interface{}{"k": "v"}, 9.0, 20, nil)

	common := tree.FindCommonSubtrees(treeA, treeB)
	if len(common) == 0 {
		t.Fatalf("Expected common subtrees with identical Merkle hashes to be identified")
	}
}

func TestTree_BeamSearchAndPruning(t *testing.T) {
	tr := tree.NewTrajectoryTree("tree-beam-test", "Deploy security hotfix to ingress gateway", nil)

	expander := func(n *tree.TreeNode) []tree.ExpansionCandidate {
		if n.Depth == 0 {
			return []tree.ExpansionCandidate{
				{
					Content:    "Path A: Apply canary release with 10% traffic",
					Role:       "assistant",
					EventType:  trace.EventToolCall,
					Score:      9.5,
					TokenUsage: 50,
				},
				{
					Content:    "Path B: Direct in-place upgrade with no health check",
					Role:       "assistant",
					EventType:  trace.EventToolCall,
					Score:      4.2, // Below threshold -> should be pruned
					TokenUsage: 30,
				},
				{
					Content:    "Path C: Dry-run diff and validate security manifest",
					Role:       "assistant",
					EventType:  trace.EventToolCall,
					Score:      8.8,
					TokenUsage: 60,
				},
			}
		}
		if n.Depth == 1 {
			return []tree.ExpansionCandidate{
				{
					Content:    "Canary verified: 0 HTTP 5xx errors, success completed",
					Role:       "tool",
					EventType:  trace.EventToolResult,
					Score:      9.8,
					TokenUsage: 40,
				},
				{
					Content:    "Repetitive poll action",
					Role:       "tool",
					EventType:  trace.EventToolCall,
					Score:      7.0,
					TokenUsage: 35,
				},
			}
		}
		return nil
	}

	options := tree.SearchOptions{
		BeamWidth:         2,
		MaxDepth:          2,
		MinScoreThreshold: 6.0,
		MaxTokenBudget:    1000,
		Policy:            tree.PolicyBeamSearch,
	}

	optimalPath, err := tr.RunBeamSearch(expander, options)
	if err != nil {
		t.Fatalf("Beam search failed: %v", err)
	}

	if optimalPath == nil {
		t.Fatalf("Expected optimal path to be found")
	}
	if tr.TotalPruned < 1 {
		t.Fatalf("Expected at least 1 node to be pruned (Path B score was 4.2 < 6.0), got %d", tr.TotalPruned)
	}

	// Verify optimal path is highest scoring
	if optimalPath.AverageScore < 9.0 {
		t.Fatalf("Expected high average score on winning path, got: %.2f", optimalPath.AverageScore)
	}

	// Verify session export
	session := tr.ExportSession(optimalPath)
	if session == nil {
		t.Fatalf("Failed to export linear trace.Session from optimal path")
	}
	if len(session.Events) != len(optimalPath.Nodes) {
		t.Fatalf("Event count mismatch in exported session: expected %d, got %d", len(optimalPath.Nodes), len(session.Events))
	}
	if session.Status != "completed" {
		t.Fatalf("Expected completed session status, got: %s", session.Status)
	}
}

func TestTree_LoopDetectionPruning(t *testing.T) {
	tr := tree.NewTrajectoryTree("tree-loop-test", "Start search", nil)
	c1, _ := tr.AddChild(tr.Root.ID, "node-1", "tool", "poll_status", trace.EventToolCall, nil, 8.0, 10, nil)
	c2, _ := tr.AddChild(c1.ID, "node-2", "tool", "poll_status", trace.EventToolCall, nil, 8.0, 10, nil)
	c3, _ := tr.AddChild(c2.ID, "node-3", "tool", "poll_status", trace.EventToolCall, nil, 8.0, 10, nil)

	hasLoop := tr.DetectLoops(c3.ID)
	if !hasLoop {
		t.Fatalf("Expected loop detection to return true for 3 consecutive poll_status actions")
	}
}

func TestTree_ScorecardAndMermaid(t *testing.T) {
	tr := tree.NewTrajectoryTree("tree-render-test", "Root prompt", nil)
	c1, _ := tr.AddChild(tr.Root.ID, "c1", "assistant", "Action 1", trace.EventToolCall, nil, 9.0, 40, nil)
	_, _ = tr.AddChild(c1.ID, "c2", "tool", "Result completed", trace.EventToolResult, nil, 9.5, 30, nil)
	tr.OptimalPath = tr.SelectOptimalPath()

	scorecard := tree.RenderTreeScorecard(tr)
	if !strings.Contains(scorecard, "AGENTVCR // MULTI-HYPOTHESIS TRAJECTORY TREE") {
		t.Fatalf("Scorecard missing header: %s", scorecard)
	}
	if !strings.Contains(scorecard, "Root Merkle Hash") {
		t.Fatalf("Scorecard missing Root Merkle Hash: %s", scorecard)
	}

	mermaid := tree.RenderTreeMermaid(tr)
	if !strings.Contains(mermaid, "graph TD") || !strings.Contains(mermaid, "style") {
		t.Fatalf("Mermaid diagram missing expected markup: %s", mermaid)
	}
}
