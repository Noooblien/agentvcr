package tree

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// ComputeContentHash computes a deterministic SHA-256 checksum of a node's local semantic content.
func ComputeContentHash(node *TreeNode) string {
	h := sha256.New()
	h.Write([]byte(node.Role))
	h.Write([]byte(":"))
	h.Write([]byte(node.Content))
	h.Write([]byte(":"))
	h.Write([]byte(string(node.Event.Type)))
	h.Write([]byte(":"))

	if node.Event.Payload != nil {
		payloadBytes, err := json.Marshal(node.Event.Payload)
		if err == nil {
			h.Write(payloadBytes)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}

// ComputeMerkleHash calculates the Merkle tree node hash combining the node's ContentHash
// and the lexicographically sorted Merkle hashes of its children.
func ComputeMerkleHash(contentHash string, childrenHashes []string) string {
	h := sha256.New()
	h.Write([]byte(contentHash))

	if len(childrenHashes) > 0 {
		sortedHashes := make([]string, len(childrenHashes))
		copy(sortedHashes, childrenHashes)
		sort.Strings(sortedHashes)

		h.Write([]byte(":["))
		h.Write([]byte(strings.Join(sortedHashes, ",")))
		h.Write([]byte("]"))
	}

	return hex.EncodeToString(h.Sum(nil))
}

// UpdateMerkleHashes traverses the trajectory tree bottom-up (post-order) to calculate
// ContentHash and MerkleHash for all nodes and updates the tree root Merkle hash.
func (t *TrajectoryTree) UpdateMerkleHashes() string {
	if t.Root == nil {
		t.RootMerkleHash = ""
		return ""
	}

	computeNodeMerkle(t.Root)
	t.RootMerkleHash = t.Root.MerkleHash
	return t.RootMerkleHash
}

func computeNodeMerkle(node *TreeNode) string {
	if node == nil {
		return ""
	}

	node.ContentHash = ComputeContentHash(node)

	childrenHashes := make([]string, 0, len(node.Children))
	for _, child := range node.Children {
		childMerkle := computeNodeMerkle(child)
		childrenHashes = append(childrenHashes, childMerkle)
	}

	node.MerkleHash = ComputeMerkleHash(node.ContentHash, childrenHashes)
	return node.MerkleHash
}

// VerifyMerkleProof checks the cryptographic chain from a leaf node up to the tree root.
func (t *TrajectoryTree) VerifyMerkleProof(leafID string) (bool, []string, error) {
	node, exists := t.NodesByID[leafID]
	if !exists {
		return false, nil, fmt.Errorf("node %s not found in tree", leafID)
	}

	proofChain := []string{node.MerkleHash}
	curr := node

	for curr.ParentID != "" {
		parent, exists := t.NodesByID[curr.ParentID]
		if !exists {
			return false, proofChain, fmt.Errorf("broken hierarchy: parent %s not found", curr.ParentID)
		}

		// Verify parent MerkleHash actually accounts for child's MerkleHash
		childHashes := make([]string, 0, len(parent.Children))
		childFound := false
		for _, ch := range parent.Children {
			childHashes = append(childHashes, ch.MerkleHash)
			if ch.ID == curr.ID {
				childFound = true
			}
		}

		if !childFound {
			return false, proofChain, fmt.Errorf("child %s not listed in parent %s children", curr.ID, parent.ID)
		}

		expectedParentMerkle := ComputeMerkleHash(parent.ContentHash, childHashes)
		if expectedParentMerkle != parent.MerkleHash {
			return false, proofChain, fmt.Errorf("merkle mismatch at parent %s: expected %s, got %s",
				parent.ID, expectedParentMerkle, parent.MerkleHash)
		}

		proofChain = append(proofChain, parent.MerkleHash)
		curr = parent
	}

	// Verify root match
	if curr.ID != t.Root.ID || curr.MerkleHash != t.RootMerkleHash {
		return false, proofChain, fmt.Errorf("root hash mismatch: computed %s, tree root %s", curr.MerkleHash, t.RootMerkleHash)
	}

	return true, proofChain, nil
}

// FindCommonSubtrees identifies subtrees with identical Merkle hashes across two trees.
// This proves cryptographic subtree equivalence and enables zero-redundancy deduplication.
func FindCommonSubtrees(treeA, treeB *TrajectoryTree) []*TreeNode {
	if treeA == nil || treeB == nil {
		return nil
	}

	treeAMerkleMap := make(map[string]*TreeNode)
	for _, node := range treeA.NodesByID {
		if node.MerkleHash != "" {
			treeAMerkleMap[node.MerkleHash] = node
		}
	}

	common := make([]*TreeNode, 0)
	for _, nodeB := range treeB.NodesByID {
		if nodeB.MerkleHash != "" {
			if _, exists := treeAMerkleMap[nodeB.MerkleHash]; exists {
				common = append(common, nodeB)
			}
		}
	}
	return common
}
