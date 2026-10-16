package speculate

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Engine manages speculative branching, verification against real trajectory steps, rollback, and branch cache.
type Engine struct {
	mu             sync.RWMutex
	config         SpeculateConfig
	branches       map[string]*SpeculativeBranch
	predictiveCache map[string]*CacheEntry
}

// NewEngine initializes a new adaptive speculative execution engine.
func NewEngine(config SpeculateConfig) *Engine {
	return &Engine{
		config:          config,
		branches:        make(map[string]*SpeculativeBranch),
		predictiveCache: make(map[string]*CacheEntry),
	}
}

// ComputeContextHash produces a deterministic hash from a sequence of preceding trajectory steps.
func ComputeContextHash(sessionID string, baseStep int, priorEvents []trace.TraceEvent) string {
	h := sha256.New()
	h.Write([]byte(sessionID))
	h.Write([]byte(fmt.Sprintf(":%d:", baseStep)))

	// Use up to last 3 events for context window
	start := 0
	if len(priorEvents) > 3 {
		start = len(priorEvents) - 3
	}
	for i := start; i < len(priorEvents); i++ {
		pBytes, _ := json.Marshal(priorEvents[i].Payload)
		h.Write([]byte(fmt.Sprintf("%d:%s:%s", priorEvents[i].Index, priorEvents[i].Type, string(pBytes))))
	}

	return hex.EncodeToString(h.Sum(nil))[:16]
}

// CreateBranch spawns an exploratory speculative branch ahead of step baseStep.
func (e *Engine) CreateBranch(sessionID string, baseStep int, snapshot map[string]interface{}) (*SpeculativeBranch, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	branchID := fmt.Sprintf("branch-%s-step%d-%d", sessionID, baseStep, time.Now().UnixNano()%1000000)
	snapCopy := make(map[string]interface{})
	for k, v := range snapshot {
		snapCopy[k] = v
	}

	b := &SpeculativeBranch{
		BranchID:       branchID,
		SessionID:      sessionID,
		BaseStepIndex:  baseStep,
		Status:         BranchPending,
		CreatedAt:      time.Now(),
		Steps:          make([]SpeculativeStep, 0),
		BranchSnapshot: snapCopy,
	}

	e.branches[branchID] = b
	return b, nil
}

// AddSpeculativeStep records an anticipated step with confidence score and estimated wall-clock latency saving.
func (e *Engine) AddSpeculativeStep(branchID string, step SpeculativeStep) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	b, exists := e.branches[branchID]
	if !exists {
		return fmt.Errorf("branch %s not found", branchID)
	}

	if len(b.Steps) >= e.config.MaxSpeculativeDepth {
		return fmt.Errorf("exceeded max speculative depth (%d)", e.config.MaxSpeculativeDepth)
	}

	if step.Timestamp.IsZero() {
		step.Timestamp = time.Now()
	}

	b.Steps = append(b.Steps, step)
	return nil
}

// CommitOrRollback compares the speculative branch against ground-truth actual events.
// If actual events match predicted types and tool/action signatures, branch is committed.
// Otherwise, instant rollback is triggered and state is reset to baseStep snapshot.
func (e *Engine) CommitOrRollback(branchID string, actualEvents []trace.TraceEvent) (bool, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	b, exists := e.branches[branchID]
	if !exists {
		return false, fmt.Errorf("branch %s not found", branchID)
	}

	b.ResolvedAt = time.Now()

	// Compare step by step
	for i, specStep := range b.Steps {
		if i >= len(actualEvents) {
			// Upstream finished earlier than predicted branch
			b.Status = BranchRolledBack
			b.RollbackReason = fmt.Sprintf("Premature end of trajectory at step %d", specStep.StepIndex)
			b.MispredictedStep = specStep.StepIndex
			return false, nil
		}

		actual := actualEvents[i]
		if string(actual.Type) != specStep.PredictedType {
			b.Status = BranchRolledBack
			b.RollbackReason = fmt.Sprintf("Event type mismatch at step %d: predicted %s, got %s", specStep.StepIndex, specStep.PredictedType, actual.Type)
			b.MispredictedStep = specStep.StepIndex
			return false, nil
		}

		// Tool action signature comparison
		if actual.Type == trace.EventToolCall {
			actualTool, _ := actual.Payload["tool_name"].(string)
			predictedTool, _ := specStep.PredictedPayload["tool_name"].(string)
			if actualTool != "" && predictedTool != "" && actualTool != predictedTool {
				b.Status = BranchRolledBack
				b.RollbackReason = fmt.Sprintf("Tool call mismatch at step %d: predicted '%s', got '%s'", specStep.StepIndex, predictedTool, actualTool)
				b.MispredictedStep = specStep.StepIndex
				return false, nil
			}
		}
	}

	// All speculative steps verified! Commit branch
	b.Status = BranchCommitted
	return true, nil
}

// CachePredictiveBranch caches a verified high-probability branch into warm memory.
func (e *Engine) CachePredictiveBranch(contextHash string, action string, payload map[string]interface{}) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if len(e.predictiveCache) >= e.config.CacheCapacity {
		// Evict oldest or least-hit entry
		var oldestKey string
		oldestTime := time.Now()
		for k, v := range e.predictiveCache {
			if v.LastHitAt.Before(oldestTime) {
				oldestTime = v.LastHitAt
				oldestKey = k
			}
		}
		if oldestKey != "" {
			delete(e.predictiveCache, oldestKey)
		}
	}

	entry, exists := e.predictiveCache[contextHash]
	if exists {
		entry.HitCount++
		entry.LastHitAt = time.Now()
		entry.SuccessRate = (entry.SuccessRate*0.8) + (1.0 * 0.2)
	} else {
		e.predictiveCache[contextHash] = &CacheEntry{
			ContextHash:     contextHash,
			LastHitAt:       time.Now(),
			HitCount:        1,
			PredictedAction: action,
			CachedPayload:   payload,
			SuccessRate:     1.0,
		}
	}
}

// LookupCache queries the predictive branch cache for an anticipated next action.
func (e *Engine) LookupCache(contextHash string) (*CacheEntry, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()

	entry, exists := e.predictiveCache[contextHash]
	if !exists {
		return nil, false
	}
	entry.HitCount++
	entry.LastHitAt = time.Now()
	return entry, true
}

// GenerateSpeculateReport analyzes all active/completed branches for a given session.
func (e *Engine) GenerateSpeculateReport(sessionID string) *SpeculateReport {
	e.mu.RLock()
	defer e.mu.RUnlock()

	report := &SpeculateReport{
		SessionID: sessionID,
		Branches:  make([]*SpeculativeBranch, 0),
	}

	var totalWallClockSaved int64
	var committedSteps int
	var totalPredictedSteps int

	for _, b := range e.branches {
		if b.SessionID != sessionID && sessionID != "" {
			continue
		}
		report.Branches = append(report.Branches, b)
		report.TotalBranches++

		switch b.Status {
		case BranchCommitted:
			report.CommittedBranches++
			for _, st := range b.Steps {
				totalWallClockSaved += st.WallClockSavedMs
				committedSteps++
				totalPredictedSteps++
			}
		case BranchRolledBack:
			report.RolledBackBranches++
			totalPredictedSteps += len(b.Steps)
		}
	}

	for _, ce := range e.predictiveCache {
		report.CachedHits += ce.HitCount
	}

	if report.TotalBranches > 0 {
		report.BranchPredictionAccuracy = math.Round((float64(report.CommittedBranches)/float64(report.TotalBranches))*10000) / 100
	}
	if report.TotalBranches+report.CachedHits > 0 {
		report.CacheHitRatio = math.Round((float64(report.CachedHits)/float64(report.TotalBranches+report.CachedHits))*10000) / 100
	}
	report.NetWallClockSavedMs = totalWallClockSaved

	return report
}

// RenderSpeculateScorecard produces an ANSI-formatted terminal scorecard of speculative execution efficiency.
func RenderSpeculateScorecard(report *SpeculateReport) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString(fmt.Sprintf("\033[1;37m   AGENTVCR ADAPTIVE SPECULATIVE EXECUTION & PREDICTIVE BRANCH REPORT\033[0m\n"))
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")

	sb.WriteString(fmt.Sprintf("  \033[1;33mSession ID:\033[0m          %s\n", report.SessionID))
	sb.WriteString(fmt.Sprintf("  \033[1;33mTotal Branches:\033[0m      %d speculative paths evaluated\n", report.TotalBranches))
	sb.WriteString(fmt.Sprintf("  \033[1;33mCommitted Branches:\033[0m  \033[1;32m%d passed\033[0m | Rolled Back: \033[1;31m%d mispredictions\033[0m\n", report.CommittedBranches, report.RolledBackBranches))
	sb.WriteString(fmt.Sprintf("  \033[1;33mBranch Accuracy:\033[0m     \033[1;32m%.1f%%\033[0m prediction accuracy\n", report.BranchPredictionAccuracy))
	sb.WriteString(fmt.Sprintf("  \033[1;33mWarm Cache Hits:\033[0m     %d hits (Hit Ratio: %.1f%%)\n", report.CachedHits, report.CacheHitRatio))
	sb.WriteString(fmt.Sprintf("  \033[1;33mNet Latency Saved:\033[0m   \033[1;32m%dms wall-clock acceleration\033[0m\n", report.NetWallClockSavedMs))
	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")

	sb.WriteString("\033[1;37m  SPECULATIVE BRANCH AUDIT LOG:\033[0m\n")
	sb.WriteString("  \033[1;30mBRANCH ID               BASE STEP  STEPS  STATUS       SAVED (MS)  OUTCOME / REASON\033[0m\n")

	limit := len(report.Branches)
	if limit > 8 {
		limit = 8
	}
	for i := 0; i < limit; i++ {
		b := report.Branches[i]
		statusLabel := "\033[1;32mCOMMITTED\033[0m"
		reason := "Branch verified"
		if b.Status == BranchRolledBack {
			statusLabel = "\033[1;31mROLLED_BACK\033[0m"
			reason = b.RollbackReason
			if len(reason) > 28 {
				reason = reason[:25] + "..."
			}
		}

		var branchSaved int64
		for _, st := range b.Steps {
			branchSaved += st.WallClockSavedMs
		}

		bID := b.BranchID
		if len(bID) > 23 {
			bID = bID[:20] + "..."
		}

		sb.WriteString(fmt.Sprintf("  %-23s Step %-4d %-6d %-12s %-11d %s\n",
			bID, b.BaseStepIndex, len(b.Steps), statusLabel, branchSaved, reason))
	}

	if len(report.Branches) > limit {
		sb.WriteString(fmt.Sprintf("  ... [%d additional speculative branches hidden]\n", len(report.Branches)-limit))
	}

	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")
	return sb.String()
}
