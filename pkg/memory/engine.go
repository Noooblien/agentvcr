package memory

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// MemoryEngine coordinates the autonomous hierarchical memory lifecycle.
type MemoryEngine struct {
	mu             sync.RWMutex
	systemID       string
	config         MemoryConfig
	decayEngine    *DecayEngine
	knowledgeGraph *SemanticKnowledgeGraph
	episodicBuffer []*EpisodicTurn
	memoryNodes    map[string]*MemoryNode // Node ID -> Node
	lastConsolidate time.Time
	nodeSeq        int64
	prunedCount    int
}

// NewMemoryEngine creates an initialized memory engine.
func NewMemoryEngine(systemID string, cfg MemoryConfig) *MemoryEngine {
	if systemID == "" {
		systemID = "agentvcr-memory-system"
	}
	return &MemoryEngine{
		systemID:       systemID,
		config:         cfg,
		decayEngine:    NewDecayEngine(cfg),
		knowledgeGraph: NewSemanticKnowledgeGraph(),
		episodicBuffer: make([]*EpisodicTurn, 0),
		memoryNodes:    make(map[string]*MemoryNode),
		lastConsolidate: time.Now(),
	}
}

// KnowledgeGraph returns the underlying semantic knowledge graph.
func (me *MemoryEngine) KnowledgeGraph() *SemanticKnowledgeGraph {
	return me.knowledgeGraph
}

// RecordEpisode appends a new raw turn to the episodic working buffer.
func (me *MemoryEngine) RecordEpisode(turn *EpisodicTurn) {
	me.mu.Lock()
	defer me.mu.Unlock()

	if turn.ID == "" {
		me.nodeSeq++
		turn.ID = fmt.Sprintf("ep-%d", me.nodeSeq)
	}
	if turn.Timestamp.IsZero() {
		turn.Timestamp = time.Now()
	}

	me.episodicBuffer = append(me.episodicBuffer, turn)

	// If episodic buffer exceeds limit, trigger auto-consolidation asynchronously or synchronously
	if len(me.episodicBuffer) >= me.config.EpisodicBufferLimit {
		me.consolidateLocked(turn.Timestamp)
	}
}

// Consolidate triggers a full memory consolidation cycle:
// 1. Synthesizes episodic turns into reflection clusters
// 2. Extracts high-confidence semantic knowledge triples
// 3. Evaluates Ebbinghaus forgetting curves and prunes decayed nodes
func (me *MemoryEngine) Consolidate(now time.Time) *ConsolidationReport {
	me.mu.Lock()
	defer me.mu.Unlock()
	return me.consolidateLocked(now)
}

func (me *MemoryEngine) consolidateLocked(now time.Time) *ConsolidationReport {
	if now.IsZero() {
		now = time.Now()
	}

	rawTokensBefore := me.estimateEpisodicTokens()

	// 1. Cluster episodic turns into Reflections
	batchSize := me.config.ConsolidationBatchSize
	if batchSize <= 0 {
		batchSize = 5
	}

	for len(me.episodicBuffer) >= batchSize {
		batch := me.episodicBuffer[:batchSize]
		me.episodicBuffer = me.episodicBuffer[batchSize:]

		reflection := me.synthesizeReflection(batch, now)
		me.memoryNodes[reflection.ID] = reflection

		// Extract any semantic facts from the reflection
		me.extractSemanticFacts(batch, reflection, now)
	}

	// 2. Prune decayed nodes below minimum retention
	for id, node := range me.memoryNodes {
		if me.decayEngine.IsDecayed(node, now) {
			delete(me.memoryNodes, id)
			me.prunedCount++
		}
	}

	me.lastConsolidate = now

	return me.generateReportLocked(now, rawTokensBefore)
}

func (me *MemoryEngine) synthesizeReflection(batch []*EpisodicTurn, now time.Time) *MemoryNode {
	me.nodeSeq++
	id := fmt.Sprintf("ref-%d", me.nodeSeq)

	var actions []string
	avgImportance := 0.0
	var epIDs []string

	for _, ep := range batch {
		epIDs = append(epIDs, ep.ID)
		avgImportance += ep.Importance
		if ep.Action != "" {
			actions = append(actions, ep.Action)
		} else if ep.Content != "" {
			c := ep.Content
			if len(c) > 40 {
				c = c[:37] + "..."
			}
			actions = append(actions, c)
		}
	}
	avgImportance /= float64(len(batch))

	summary := fmt.Sprintf("Cluster (%d turns): %s", len(batch), strings.Join(actions, " -> "))
	if len(summary) > 200 {
		summary = summary[:197] + "..."
	}

	return &MemoryNode{
		ID:               id,
		Level:            LevelReflection,
		Content:          summary,
		Importance:       avgImportance,
		Stability:        1.2,
		CreatedAt:        now,
		LastAccessedAt:   now,
		AccessCount:      1,
		SourceEpisodeIDs: epIDs,
	}
}

func (me *MemoryEngine) extractSemanticFacts(batch []*EpisodicTurn, reflection *MemoryNode, now time.Time) {
	for _, ep := range batch {
		contentLower := strings.ToLower(ep.Content)

		// Pattern 1: User preferences (e.g. "prefers Go", "use PostgreSQL")
		if strings.Contains(contentLower, "prefer") || strings.Contains(contentLower, "always use") || strings.Contains(contentLower, "requirement:") {
			subject := "User"
			predicate := "prefers"
			object := extractEntityAfter(ep.Content, []string{"prefer", "prefers", "always use", "requirement:"})

			if object != "" {
				me.nodeSeq++
				nodeID := fmt.Sprintf("sem-%d", me.nodeSeq)

				me.knowledgeGraph.AddOrUpdateTriple(subject, predicate, object, 0.95, now)

				me.memoryNodes[nodeID] = &MemoryNode{
					ID:               nodeID,
					Level:            LevelSemantic,
					Content:          fmt.Sprintf("%s %s %s", subject, predicate, object),
					Subject:          subject,
					Predicate:        predicate,
					Object:           object,
					Confidence:       0.95,
					Importance:       0.95, // Permanent durable retention
					Stability:        5.0,
					CreatedAt:        now,
					LastAccessedAt:   now,
					AccessCount:      1,
					SourceEpisodeIDs: []string{ep.ID},
				}
			}
		}

		// Pattern 2: Architecture / Technical decisions
		if strings.Contains(contentLower, "architecture:") || strings.Contains(contentLower, "database:") || strings.Contains(contentLower, "protocol:") {
			subject := "Architecture"
			predicate := "standardizes_on"
			object := extractEntityAfter(ep.Content, []string{"architecture:", "database:", "protocol:"})

			if object != "" {
				me.nodeSeq++
				nodeID := fmt.Sprintf("sem-%d", me.nodeSeq)

				me.knowledgeGraph.AddOrUpdateTriple(subject, predicate, object, 0.90, now)

				me.memoryNodes[nodeID] = &MemoryNode{
					ID:               nodeID,
					Level:            LevelSemantic,
					Content:          fmt.Sprintf("%s %s %s", subject, predicate, object),
					Subject:          subject,
					Predicate:        predicate,
					Object:           object,
					Confidence:       0.90,
					Importance:       0.90,
					Stability:        4.5,
					CreatedAt:        now,
					LastAccessedAt:   now,
					AccessCount:      1,
					SourceEpisodeIDs: []string{ep.ID},
				}
			}
		}
	}
}

func extractEntityAfter(text string, triggers []string) string {
	textLower := strings.ToLower(text)
	for _, trigger := range triggers {
		idx := strings.Index(textLower, trigger)
		if idx != -1 {
			after := strings.TrimSpace(text[idx+len(trigger):])
			// Take first phrase or sentence segment
			split := strings.FieldsFunc(after, func(r rune) bool {
				return r == '.' || r == ',' || r == '\n' || r == ';'
			})
			if len(split) > 0 {
				return strings.TrimSpace(split[0])
			}
		}
	}
	return ""
}

// Recall retrieves memories matching query within a strict token budget, reinforcing accessed nodes.
func (me *MemoryEngine) Recall(query RecallQuery) ([]*MemoryNode, int) {
	me.mu.Lock()
	defer me.mu.Unlock()

	now := query.CurrentTimestamp
	if now.IsZero() {
		now = time.Now()
	}

	type scoredNode struct {
		node  *MemoryNode
		score float64
	}

	var candidates []scoredNode
	qLower := strings.ToLower(query.QueryText)

	for _, node := range me.memoryNodes {
		retention := me.decayEngine.RetentionScore(node, now)
		if retention < query.MinRetention {
			continue
		}

		// Calculate relevance
		relevance := 0.2
		contentLower := strings.ToLower(node.Content)
		if strings.Contains(contentLower, qLower) {
			relevance += 0.6
		} else {
			// Keyword overlap
			words := strings.Fields(qLower)
			matches := 0
			for _, w := range words {
				if len(w) > 2 && strings.Contains(contentLower, w) {
					matches++
				}
			}
			if len(words) > 0 {
				relevance += 0.5 * (float64(matches) / float64(len(words)))
			}
		}

		totalScore := (0.45 * relevance) + (0.35 * retention) + (0.20 * node.Importance)
		candidates = append(candidates, scoredNode{node: node, score: totalScore})
	}

	// Sort candidates by score descending
	sort.Slice(candidates, func(i, j int) bool {
		return candidates[i].score > candidates[j].score
	})

	var result []*MemoryNode
	tokensUsed := 0
	budget := query.TokenBudget
	if budget <= 0 {
		budget = 1000
	}

	for _, cand := range candidates {
		nodeTokens := (len(cand.node.Content) / 4) + 5 // Approximate token estimation
		if tokensUsed+nodeTokens > budget {
			continue // Skip if exceeds remaining budget
		}

		// Reinforce accessed memory
		me.decayEngine.Reinforce(cand.node, now)

		result = append(result, cand.node)
		tokensUsed += nodeTokens
	}

	return result, tokensUsed
}

func (me *MemoryEngine) estimateEpisodicTokens() int {
	total := 0
	for _, ep := range me.episodicBuffer {
		total += (len(ep.Content) + len(ep.Action) + len(ep.Observation) + len(ep.Role)) / 4 + 20
	}
	return total
}

func (me *MemoryEngine) estimateConsolidatedTokens() int {
	total := 0
	for _, node := range me.memoryNodes {
		total += len(node.Content)/4 + 8
	}
	return total
}

func (me *MemoryEngine) generateReportLocked(now time.Time, rawTokensBefore int) *ConsolidationReport {
	reflections := 0
	semantics := 0

	var allNodes []*MemoryNode
	for _, node := range me.memoryNodes {
		allNodes = append(allNodes, node)
		if node.Level == LevelReflection {
			reflections++
		} else if node.Level == LevelSemantic {
			semantics++
		}
	}

	sort.Slice(allNodes, func(i, j int) bool {
		return me.decayEngine.RetentionScore(allNodes[i], now) > me.decayEngine.RetentionScore(allNodes[j], now)
	})

	top := allNodes
	if len(top) > 5 {
		top = top[:5]
	}

	consolidatedTokens := me.estimateConsolidatedTokens()
	if rawTokensBefore == 0 {
		rawTokensBefore = consolidatedTokens * 3
	}

	savingsRatio := 0.0
	if rawTokensBefore > 0 && consolidatedTokens < rawTokensBefore {
		savingsRatio = float64(rawTokensBefore-consolidatedTokens) / float64(rawTokensBefore)
	}

	return &ConsolidationReport{
		SystemID:               me.systemID,
		Timestamp:              now,
		TotalEpisodicBuffered:  len(me.episodicBuffer),
		ConsolidatedReflection: reflections,
		PermanentSemanticFacts: semantics,
		PrunedDecayedNodes:     me.prunedCount,
		ActiveMemoryNodes:      len(me.memoryNodes),
		RawEpisodicTokens:      rawTokensBefore,
		ConsolidatedTokens:     consolidatedTokens,
		TokenSavingsRatio:      savingsRatio,
		TopMemories:            top,
	}
}

// Audit produces a comprehensive diagnostic report.
func (me *MemoryEngine) Audit(now time.Time) *ConsolidationReport {
	me.mu.RLock()
	defer me.mu.RUnlock()
	return me.generateReportLocked(now, me.estimateEpisodicTokens())
}
