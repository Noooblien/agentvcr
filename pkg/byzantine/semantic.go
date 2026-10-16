package byzantine

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"regexp"
	"sort"
	"strings"
)

var (
	wordBoundaryRegex = regexp.MustCompile(`[^a-zA-Z0-9]+`)
	stopWords         = map[string]bool{
		"a": true, "an": true, "the": true, "and": true, "or": true, "in": true,
		"on": true, "at": true, "to": true, "for": true, "with": true, "by": true,
		"is": true, "are": true, "was": true, "were": true, "be": true, "this": true,
		"that": true, "it": true, "from": true, "as": true, "of": true, "please": true,
	}
	synonyms = map[string]string{
		"k8s":   "kubernetes",
		"db":    "database",
		"svc":   "service",
		"cfg":   "config",
		"env":   "environment",
		"msg":   "message",
		"err":   "error",
		"req":   "request",
		"res":   "response",
		"auth":  "authentication",
		"nodes": "node",
		"apps":  "app",
	}
)

// NormalizeTokens tokenizes and filters text into canonical semantic tokens.
func NormalizeTokens(text string) []string {
	clean := wordBoundaryRegex.ReplaceAllString(strings.ToLower(text), " ")
	rawTokens := strings.Fields(clean)

	var filtered []string
	seen := make(map[string]bool)

	for _, t := range rawTokens {
		if len(t) <= 1 || stopWords[t] {
			continue
		}
		if canon, ok := synonyms[t]; ok {
			t = canon
		}
		if !seen[t] {
			seen[t] = true
			filtered = append(filtered, t)
		}
	}
	sort.Strings(filtered)
	return filtered
}

// ComputeJaccardSimilarity computes set intersection over union for two token slices.
func ComputeJaccardSimilarity(tokensA, tokensB []string) float64 {
	if len(tokensA) == 0 && len(tokensB) == 0 {
		return 1.0
	}
	if len(tokensA) == 0 || len(tokensB) == 0 {
		return 0.0
	}

	setA := make(map[string]bool, len(tokensA))
	for _, t := range tokensA {
		setA[t] = true
	}

	intersection := 0
	for _, t := range tokensB {
		if setA[t] {
			intersection++
		}
	}

	union := len(tokensA) + len(tokensB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

// ComputeProposalSimilarity evaluates the composite semantic similarity between two proposals.
func ComputeProposalSimilarity(a, b AgentActionProposal) float64 {
	// Exact intent match shortcut
	if strings.EqualFold(a.ActionIntent, b.ActionIntent) {
		return 1.0
	}

	intentTokensA := NormalizeTokens(a.ActionIntent)
	intentTokensB := NormalizeTokens(b.ActionIntent)
	jaccardIntent := ComputeJaccardSimilarity(intentTokensA, intentTokensB)

	tokensA := a.NormalizedTokens
	if len(tokensA) == 0 {
		tokensA = NormalizeTokens(a.ActionIntent + " " + a.RawPayload)
	}

	tokensB := b.NormalizedTokens
	if len(tokensB) == 0 {
		tokensB = NormalizeTokens(b.ActionIntent + " " + b.RawPayload)
	}

	jaccardPayload := ComputeJaccardSimilarity(tokensA, tokensB)

	// Action verb and target noun heuristic boost
	intentSim := computeStringLevenshteinSimilarity(strings.ToLower(a.ActionIntent), strings.ToLower(b.ActionIntent))

	return (0.45 * jaccardIntent) + (0.35 * jaccardPayload) + (0.20 * intentSim)
}

func computeStringLevenshteinSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}
	if len(s1) == 0 || len(s2) == 0 {
		return 0.0
	}

	r1, r2 := []rune(s1), []rune(s2)
	l1, l2 := len(r1), len(r2)

	dp := make([][]int, l1+1)
	for i := range dp {
		dp[i] = make([]int, l2+1)
		dp[i][0] = i
	}
	for j := 0; j <= l2; j++ {
		dp[0][j] = j
	}

	for i := 1; i <= l1; i++ {
		for j := 1; j <= l2; j++ {
			cost := 0
			if r1[i-1] != r2[j-1] {
				cost = 1
			}
			minVal := dp[i-1][j] + 1
			if dp[i][j-1]+1 < minVal {
				minVal = dp[i][j-1] + 1
			}
			if dp[i-1][j-1]+cost < minVal {
				minVal = dp[i-1][j-1] + cost
			}
			dp[i][j] = minVal
		}
	}

	maxLen := math.Max(float64(l1), float64(l2))
	return 1.0 - (float64(dp[l1][l2]) / maxLen)
}

// GenerateSemanticHash produces a deterministic hash from a canonical intent and normalized tokens.
func GenerateSemanticHash(canonicalIntent string, tokens []string) string {
	sortedTokens := make([]string, len(tokens))
	copy(sortedTokens, tokens)
	sort.Strings(sortedTokens)

	payload := canonicalIntent + ":" + strings.Join(sortedTokens, ",")
	h := sha256.Sum256([]byte(payload))
	return hex.EncodeToString(h[:16]) // 128-bit truncated hex
}

// ClusterProposals groups proposals into semantic equivalence clusters according to a similarity threshold.
func ClusterProposals(proposals []AgentActionProposal, similarityThreshold float64, totalSwarmNodes int) []*SemanticCluster {
	if len(proposals) == 0 {
		return nil
	}

	if similarityThreshold <= 0 {
		similarityThreshold = 0.45 // Tolerant threshold for LLM syntactic divergence
	}

	var clusters []*SemanticCluster

	for _, p := range proposals {
		if len(p.NormalizedTokens) == 0 {
			p.NormalizedTokens = NormalizeTokens(p.ActionIntent + " " + p.RawPayload)
		}
		if p.SemanticHash == "" {
			p.SemanticHash = GenerateSemanticHash(p.ActionIntent, p.NormalizedTokens)
		}

		matched := false
		for _, c := range clusters {
			for _, member := range c.Proposals {
				sim := ComputeProposalSimilarity(p, member)
				if sim >= similarityThreshold {
					c.Proposals = append(c.Proposals, p)
					c.Weight = len(c.Proposals)
					totalSim := 0.0
					for _, m := range c.Proposals {
						totalSim += ComputeProposalSimilarity(m, c.Proposals[0])
					}
					c.AverageSimilarity = totalSim / float64(len(c.Proposals))
					matched = true
					break
				}
			}
			if matched {
				break
			}
		}

		if !matched {
			cluster := &SemanticCluster{
				CanonicalIntent:   p.ActionIntent,
				ClusterHash:       p.SemanticHash,
				Proposals:         []AgentActionProposal{p},
				Weight:            1,
				AverageSimilarity: 1.0,
			}
			clusters = append(clusters, cluster)
		}
	}

	// Sort clusters by weight descending
	sort.Slice(clusters, func(i, j int) bool {
		return clusters[i].Weight > clusters[j].Weight
	})

	// BFT Supermajority: 2f + 1 where f = floor((N - 1) / 3)
	// Quorum is met if Weight >= 2*f + 1
	f := (totalSwarmNodes - 1) / 3
	quorumReq := 2*f + 1
	if quorumReq < 1 {
		quorumReq = 1
	}

	for _, c := range clusters {
		if c.Weight >= quorumReq {
			c.SupermajorityMet = true
		}
	}

	return clusters
}
