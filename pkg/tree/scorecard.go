package tree

import (
	"fmt"
	"strings"
)

// ANSI terminal color escape codes
const (
	ColorReset   = "\033[0m"
	ColorBold    = "\033[1m"
	ColorDim     = "\033[2m"
	ColorRed     = "\033[31m"
	ColorGreen   = "\033[32m"
	ColorYellow  = "\033[33m"
	ColorBlue    = "\033[34m"
	ColorMagenta = "\033[35m"
	ColorCyan    = "\033[36m"
	ColorWhite   = "\033[37m"
)

// RenderTreeScorecard formats an ANSI terminal scorecard summarizing tree exploration metrics.
func RenderTreeScorecard(t *TrajectoryTree) string {
	var sb strings.Builder

	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s  AGENTVCR // MULTI-HYPOTHESIS TRAJECTORY TREE & MERKLE SEARCH HYPERVISOR%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n\n", ColorBold, ColorCyan, ColorReset))

	sb.WriteString(fmt.Sprintf("  %sTree ID:%s          %s\n", ColorBold, ColorReset, t.ID))
	sb.WriteString(fmt.Sprintf("  %sSearch Policy:%s    %s (Beam Width: %d, Max Depth: %d)\n", ColorBold, ColorReset, t.Policy, t.BeamWidth, t.MaxDepth))

	merkleDisplay := t.RootMerkleHash
	if len(merkleDisplay) > 24 {
		merkleDisplay = merkleDisplay[:12] + "..." + merkleDisplay[len(merkleDisplay)-12:]
	}
	sb.WriteString(fmt.Sprintf("  %sRoot Merkle Hash:%s %s%s%s\n", ColorBold, ColorReset, ColorCyan, merkleDisplay, ColorReset))

	pruneRatio := 0.0
	if t.TotalNodes > 0 {
		pruneRatio = (float64(t.TotalPruned) / float64(t.TotalNodes)) * 100.0
	}
	sb.WriteString(fmt.Sprintf("  %sSearch Scale:%s     %d nodes total | %d expanded | %s%d pruned (%.1f%%)%s\n",
		ColorBold, ColorReset, t.TotalNodes, t.TotalExpanded, ColorYellow, t.TotalPruned, pruneRatio, ColorReset))

	if t.OptimalPath != nil {
		sb.WriteString(fmt.Sprintf("  %sOptimal Branch:%s   %s%s[%s]%s (Score: %.2f/10.0, Tokens: %d, Nodes: %d)\n\n",
			ColorBold, ColorReset, ColorBold, ColorGreen, t.OptimalPath.LeafID, ColorReset,
			t.OptimalPath.AverageScore, t.OptimalPath.TotalTokens, len(t.OptimalPath.Nodes)))
	} else {
		sb.WriteString("\n")
	}

	// Hierarchy Breakdown Table
	sb.WriteString(fmt.Sprintf("  %s%sEXPLORATION NODES & CRYPTOGRAPHIC PROVENANCE:%s\n", ColorBold, ColorWhite, ColorReset))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %-20s %-5s %-10s %-8s %-8s %-18s %s\n", "NODE ID", "DEPTH", "ROLE", "SCORE", "TOKENS", "STATUS", "MERKLE"))
	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")

	// Collect nodes in breadth-first or depth-first order
	var printNode func(n *TreeNode)
	printNode = func(n *TreeNode) {
		statusColor := ColorGreen
		statusText := string(n.Status)
		if n.Status == NodeStatusPruned {
			statusColor = ColorRed
			if n.PruneReason != PruneReasonNone {
				statusText = fmt.Sprintf("pruned:%s", n.PruneReason)
			}
		} else if n.Status == NodeStatusTerminalSuccess {
			statusColor = ColorBold + ColorGreen
			statusText = "SUCCESS"
		}

		shortHash := n.MerkleHash
		if len(shortHash) > 10 {
			shortHash = shortHash[:8] + ".."
		}

		nodePrefix := ""
		if t.OptimalPath != nil {
			for _, optNode := range t.OptimalPath.Nodes {
				if optNode.ID == n.ID {
					nodePrefix = "★ "
					break
				}
			}
		}

		idDisplay := fmt.Sprintf("%s%s", nodePrefix, n.ID)
		if len(idDisplay) > 20 {
			idDisplay = idDisplay[:17] + "..."
		}

		sb.WriteString(fmt.Sprintf("  %-20s %-5d %-10s %-8.1f %-8d %s%-18s%s %s\n",
			idDisplay, n.Depth, n.Role, n.Score, n.TokenUsage, statusColor, statusText, ColorReset, shortHash))

		for _, child := range n.Children {
			printNode(child)
		}
	}

	if t.Root != nil {
		printNode(t.Root)
	}

	sb.WriteString("  ────────────────────────────────────────────────────────────────────────────────────────\n")
	sb.WriteString(fmt.Sprintf("  %s★ = Selected Optimal Winning Path Node%s\n", ColorGreen, ColorReset))
	sb.WriteString(fmt.Sprintf("%s%s========================================================================================%s\n", ColorBold, ColorCyan, ColorReset))
	return sb.String()
}

// RenderTreeMermaid generates an interactive Mermaid flowchart visualizing the trajectory tree,
// highlighting the optimal trajectory branch and pruned paths.
func RenderTreeMermaid(t *TrajectoryTree) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")

	optimalNodeIDs := make(map[string]bool)
	if t.OptimalPath != nil {
		for _, n := range t.OptimalPath.Nodes {
			optimalNodeIDs[n.ID] = true
		}
	}

	var walkTree func(n *TreeNode)
	walkTree = func(n *TreeNode) {
		label := fmt.Sprintf("%s<br/>Role: %s | Score: %.1f<br/>Tokens: %d", n.ID, n.Role, n.Score, n.TokenUsage)
		if n.Status == NodeStatusPruned {
			label = fmt.Sprintf("%s<br/>PRUNED: %s", n.ID, n.PruneReason)
		}

		safeID := strings.ReplaceAll(strings.ReplaceAll(n.ID, "-", "_"), ".", "_")
		sb.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", safeID, label))

		for _, ch := range n.Children {
			safeChildID := strings.ReplaceAll(strings.ReplaceAll(ch.ID, "-", "_"), ".", "_")
			edgeLabel := fmt.Sprintf("+%.1f", ch.Score)
			if ch.Status == NodeStatusPruned {
				sb.WriteString(fmt.Sprintf("    %s -.->|%s| %s\n", safeID, edgeLabel, safeChildID))
			} else {
				sb.WriteString(fmt.Sprintf("    %s -->|%s| %s\n", safeID, edgeLabel, safeChildID))
			}
			walkTree(ch)
		}
	}

	if t.Root != nil {
		walkTree(t.Root)
	}

	sb.WriteString("\n    %% Styling\n")
	// Style Root
	if t.Root != nil {
		rootSafeID := strings.ReplaceAll(strings.ReplaceAll(t.Root.ID, "-", "_"), ".", "_")
		sb.WriteString(fmt.Sprintf("    style %s fill:#f1eefe,stroke:#6f42c1,stroke-width:2px\n", rootSafeID))
	}

	for _, n := range t.NodesByID {
		safeID := strings.ReplaceAll(strings.ReplaceAll(n.ID, "-", "_"), ".", "_")
		if n.ID == t.Root.ID {
			continue
		}
		if optimalNodeIDs[n.ID] {
			sb.WriteString(fmt.Sprintf("    style %s fill:#e6f4ea,stroke:#28a745,stroke-width:3px\n", safeID))
		} else if n.Status == NodeStatusPruned {
			sb.WriteString(fmt.Sprintf("    style %s fill:#fbeae8,stroke:#dc3545,stroke-dasharray: 5 5,stroke-width:1px\n", safeID))
		}
	}

	sb.WriteString("```\n")
	return sb.String()
}
