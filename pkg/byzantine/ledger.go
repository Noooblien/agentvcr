package byzantine

import (
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"sync"
	"time"
)

// Ledger manages the append-only cryptographically verifiable blockchain of consensus blocks.
type Ledger struct {
	mu           sync.RWMutex
	Blocks       []*LedgerBlock
	GenesisHash  string
	CurrentState string
	NodeKeys     map[string]ed25519.PublicKey // agentID -> Ed25519 Public Key
	QuorumSize   int
}

// NewLedger initializes a fresh distributed consensus ledger with a deterministic genesis block.
func NewLedger(nodeKeys map[string]ed25519.PublicKey, quorumSize int) *Ledger {
	genesisTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	genesis := &LedgerBlock{
		Index:           0,
		PrevHash:        "0000000000000000000000000000000000000000000000000000000000000000",
		Round:           0,
		View:            0,
		LeaderID:        "GENESIS",
		CanonicalIntent: "GENESIS_ROOT",
		SemanticHash:    "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855",
		StateRoot:       "genesis_state_root",
		Timestamp:       genesisTime,
	}
	genesis.BlockHash = ComputeBlockHash(genesis)

	return &Ledger{
		Blocks:       []*LedgerBlock{genesis},
		GenesisHash:  genesis.BlockHash,
		CurrentState: genesis.StateRoot,
		NodeKeys:     nodeKeys,
		QuorumSize:   quorumSize,
	}
}

// ComputeBlockHash computes a deterministic SHA-256 hash across block header attributes.
func ComputeBlockHash(b *LedgerBlock) string {
	payload := fmt.Sprintf("%d:%s:%d:%d:%s:%s:%s:%s:%d",
		b.Index,
		b.PrevHash,
		b.Round,
		b.View,
		b.LeaderID,
		b.CanonicalIntent,
		b.SemanticHash,
		b.StateRoot,
		b.Timestamp.UnixNano(),
	)
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:])
}

// AppendBlock appends a cryptographically attested block to the ledger after validating invariants.
func (l *Ledger) AppendBlock(b *LedgerBlock) error {
	l.mu.Lock()
	defer l.mu.Unlock()

	last := l.Blocks[len(l.Blocks)-1]

	if b.Index != last.Index+1 {
		return fmt.Errorf("block index discontinuity: expected %d, got %d", last.Index+1, b.Index)
	}

	if b.PrevHash != last.BlockHash {
		return fmt.Errorf("invalid prev_hash: expected %s, got %s", last.BlockHash, b.PrevHash)
	}

	calculatedHash := ComputeBlockHash(b)
	if b.BlockHash != "" && b.BlockHash != calculatedHash {
		return fmt.Errorf("block hash mismatch: expected %s, got %s", calculatedHash, b.BlockHash)
	}
	b.BlockHash = calculatedHash

	// Verify Commit Certificate
	if b.CommitCert == nil {
		return fmt.Errorf("block %d missing commit certificate", b.Index)
	}

	if len(b.CommitCert.Signatures) < l.QuorumSize {
		return fmt.Errorf("insufficient commit certificate signatures: have %d, required %d",
			len(b.CommitCert.Signatures), l.QuorumSize)
	}

	// Verify cryptographic signatures on the commit certificate
	certPayload := fmt.Sprintf("%d:%d:%d:%s:%s",
		b.Index, b.Round, b.View, b.SemanticHash, b.StateRoot)

	validSignatures := 0
	for agentID, sigHex := range b.CommitCert.Signatures {
		pubKey, exists := l.NodeKeys[agentID]
		if !exists {
			continue
		}
		sigBytes, err := hex.DecodeString(sigHex)
		if err != nil {
			continue
		}
		if ed25519.Verify(pubKey, []byte(certPayload), sigBytes) {
			validSignatures++
		}
	}

	if validSignatures < l.QuorumSize {
		return fmt.Errorf("cryptographically valid signatures (%d) fell below required quorum (%d)",
			validSignatures, l.QuorumSize)
	}

	l.Blocks = append(l.Blocks, b)
	l.CurrentState = b.StateRoot
	return nil
}

// VerifyLedger audits the entire block chain for hash continuity and certificate validity.
func (l *Ledger) VerifyLedger() (bool, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()

	if len(l.Blocks) == 0 {
		return false, fmt.Errorf("ledger contains no blocks")
	}

	if l.Blocks[0].BlockHash != l.GenesisHash {
		return false, fmt.Errorf("genesis block hash corrupted")
	}

	for i := 1; i < len(l.Blocks); i++ {
		prev := l.Blocks[i-1]
		curr := l.Blocks[i]

		if curr.Index != prev.Index+1 {
			return false, fmt.Errorf("index gap between block %d and %d", prev.Index, curr.Index)
		}
		if curr.PrevHash != prev.BlockHash {
			return false, fmt.Errorf("broken chain link at block %d", curr.Index)
		}
		if ComputeBlockHash(curr) != curr.BlockHash {
			return false, fmt.Errorf("tampered block hash at block %d", curr.Index)
		}
		if curr.CommitCert == nil || len(curr.CommitCert.Signatures) < l.QuorumSize {
			return false, fmt.Errorf("invalid commit certificate at block %d", curr.Index)
		}
	}

	return true, nil
}

// RollbackTo discards any blocks after targetIndex, effectively restoring state to a safe checkpoint.
func (l *Ledger) RollbackTo(targetIndex int) (*LedgerBlock, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	if targetIndex < 0 || targetIndex >= len(l.Blocks) {
		return nil, fmt.Errorf("invalid rollback target index: %d (current height: %d)", targetIndex, len(l.Blocks)-1)
	}

	l.Blocks = l.Blocks[:targetIndex+1]
	checkpoint := l.Blocks[targetIndex]
	l.CurrentState = checkpoint.StateRoot
	return checkpoint, nil
}

// Height returns the current block height of the ledger.
func (l *Ledger) Height() int {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return len(l.Blocks) - 1
}

// LatestBlock returns the latest committed block in the ledger.
func (l *Ledger) LatestBlock() *LedgerBlock {
	l.mu.RLock()
	defer l.mu.RUnlock()
	return l.Blocks[len(l.Blocks)-1]
}

// GetBlock retrieves a block by its height index.
func (l *Ledger) GetBlock(index int) (*LedgerBlock, error) {
	l.mu.RLock()
	defer l.mu.RUnlock()
	if index < 0 || index >= len(l.Blocks) {
		return nil, fmt.Errorf("block index %d out of bounds", index)
	}
	return l.Blocks[index], nil
}
