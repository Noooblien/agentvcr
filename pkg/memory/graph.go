package memory

import (
	"fmt"
	"strings"
	"sync"
	"time"
)

// SemanticTriple represents a structured knowledge fact connecting entities.
type SemanticTriple struct {
	Subject    string    `json:"subject"`
	Predicate  string    `json:"predicate"`
	Object     string    `json:"object"`
	Confidence float64   `json:"confidence"`
	UpdatedAt  time.Time `json:"updated_at"`
	Superseded bool      `json:"superseded"`
}

// SemanticKnowledgeGraph manages entity relations, semantic triples, and contradiction resolution.
type SemanticKnowledgeGraph struct {
	mu      sync.RWMutex
	triples map[string]*SemanticTriple // Key: "subject:predicate"
	nodes   map[string]map[string]bool // Entity -> Set of related entities
}

// NewSemanticKnowledgeGraph initializes a thread-safe semantic knowledge graph.
func NewSemanticKnowledgeGraph() *SemanticKnowledgeGraph {
	return &SemanticKnowledgeGraph{
		triples: make(map[string]*SemanticTriple),
		nodes:   make(map[string]map[string]bool),
	}
}

// AddOrUpdateTriple adds a knowledge triple, resolving contradictions automatically.
// If a triple with the same subject and predicate exists with a different object,
// the newer triple replaces it and marks the fact updated.
func (kg *SemanticKnowledgeGraph) AddOrUpdateTriple(subject, predicate, object string, confidence float64, now time.Time) (bool, string) {
	kg.mu.Lock()
	defer kg.mu.Unlock()

	normSub := strings.TrimSpace(strings.ToLower(subject))
	normPred := strings.TrimSpace(strings.ToLower(predicate))
	normObj := strings.TrimSpace(object)

	key := fmt.Sprintf("%s:%s", normSub, normPred)

	existing, exists := kg.triples[key]
	contradictionResolved := false
	diffMsg := ""

	if exists {
		if !strings.EqualFold(existing.Object, normObj) {
			diffMsg = fmt.Sprintf("Updated [%s %s]: replaced '%s' with '%s'", normSub, normPred, existing.Object, normObj)
			contradictionResolved = true
		}
	}

	kg.triples[key] = &SemanticTriple{
		Subject:    normSub,
		Predicate:  normPred,
		Object:     normObj,
		Confidence: confidence,
		UpdatedAt:  now,
		Superseded: false,
	}

	// Update entity adjacency
	if kg.nodes[normSub] == nil {
		kg.nodes[normSub] = make(map[string]bool)
	}
	kg.nodes[normSub][strings.ToLower(normObj)] = true

	return contradictionResolved, diffMsg
}

// GetTriple looks up a specific relationship.
func (kg *SemanticKnowledgeGraph) GetTriple(subject, predicate string) (*SemanticTriple, bool) {
	kg.mu.RLock()
	defer kg.mu.RUnlock()

	key := fmt.Sprintf("%s:%s", strings.TrimSpace(strings.ToLower(subject)), strings.TrimSpace(strings.ToLower(predicate)))
	t, ok := kg.triples[key]
	return t, ok
}

// ListTriples returns all active semantic triples.
func (kg *SemanticKnowledgeGraph) ListTriples() []*SemanticTriple {
	kg.mu.RLock()
	defer kg.mu.RUnlock()

	res := make([]*SemanticTriple, 0, len(kg.triples))
	for _, t := range kg.triples {
		if !t.Superseded {
			res = append(res, t)
		}
	}
	return res
}

// SearchEntities finds all facts mentioning an entity keyword.
func (kg *SemanticKnowledgeGraph) SearchEntities(query string) []*SemanticTriple {
	kg.mu.RLock()
	defer kg.mu.RUnlock()

	q := strings.ToLower(strings.TrimSpace(query))
	var matches []*SemanticTriple

	for _, t := range kg.triples {
		if strings.Contains(t.Subject, q) || strings.Contains(t.Predicate, q) || strings.Contains(strings.ToLower(t.Object), q) {
			matches = append(matches, t)
		}
	}
	return matches
}

// Count returns total active facts in graph.
func (kg *SemanticKnowledgeGraph) Count() int {
	kg.mu.RLock()
	defer kg.mu.RUnlock()
	return len(kg.triples)
}
