package forensics

import (
	"math"
	"regexp"
	"strings"
	"unicode"
)

var stopWords = map[string]bool{
	"a": true, "an": true, "the": true, "and": true, "or": true, "but": true,
	"in": true, "on": true, "at": true, "to": true, "for": true, "of": true,
	"with": true, "by": true, "as": true, "is": true, "are": true, "was": true,
	"were": true, "be": true, "been": true, "this": true, "that": true, "it": true,
}

// Vectorizer computes deterministic sparse term-frequency and n-gram vectors.
type Vectorizer struct{}

// NewVectorizer creates a new vectorizer instance.
func NewVectorizer() *Vectorizer {
	return &Vectorizer{}
}

// Vectorize converts a string into a sparse term-frequency map including uni-grams and bi-grams.
func (v *Vectorizer) Vectorize(text string) map[string]float64 {
	tokens := tokenize(text)
	vec := make(map[string]float64)

	// Uni-grams
	for _, tok := range tokens {
		if !stopWords[tok] {
			vec[tok] += 1.0
		}
	}

	// Bi-grams (for semantic phrases like "10 million", "san francisco", "revenue drop")
	for i := 0; i < len(tokens)-1; i++ {
		bigram := tokens[i] + "_" + tokens[i+1]
		vec[bigram] += 1.5
	}

	return vec
}

// CosineSimilarity computes the cosine similarity between two sparse vector representations.
func (v *Vectorizer) CosineSimilarity(vecA, vecB map[string]float64) float64 {
	if len(vecA) == 0 || len(vecB) == 0 {
		return 0.0
	}

	dotProduct := 0.0
	for term, valA := range vecA {
		if valB, ok := vecB[term]; ok {
			dotProduct += valA * valB
		}
	}

	if dotProduct == 0.0 {
		return 0.0
	}

	normA := 0.0
	for _, valA := range vecA {
		normA += valA * valA
	}

	normB := 0.0
	for _, valB := range vecB {
		normB += valB * valB
	}

	if normA == 0.0 || normB == 0.0 {
		return 0.0
	}

	sim := dotProduct / (math.Sqrt(normA) * math.Sqrt(normB))
	if sim > 1.0 {
		return 1.0
	}
	return sim
}

// SegmentSentences splits a paragraph of LLM output into individual claims/assertions.
func SegmentSentences(text string) []string {
	if text == "" {
		return nil
	}

	// Normalize whitespace
	clean := strings.TrimSpace(text)
	// Replace linebreaks with sentence terminators if missing
	clean = strings.ReplaceAll(clean, "\r", "")
	clean = strings.ReplaceAll(clean, "\n\n", ". ")
	clean = strings.ReplaceAll(clean, "\n", ". ")

	re := regexp.MustCompile(`(?m)[.!?]+(\s+|$)`)
	rawSegments := re.Split(clean, -1)

	var sentences []string
	for _, seg := range rawSegments {
		s := strings.TrimSpace(seg)
		s = strings.Trim(s, "*-•# \t\r\n")
		// Filter out very short conversational filler fragments (< 10 chars)
		if len(s) >= 10 {
			sentences = append(sentences, s)
		}
	}

	if len(sentences) == 0 && len(clean) > 0 {
		sentences = append(sentences, clean)
	}

	return sentences
}

// HasContradictionHeuristic checks if a claim contains negation words where the evidence does not, or vice versa.
func HasContradictionHeuristic(claim, evidence string) bool {
	negations := []string{" not ", " never ", " no ", " neither ", " cannot ", " isn't ", " aren't ", " didn't "}
	claimLower := " " + strings.ToLower(claim) + " "
	evidenceLower := " " + strings.ToLower(evidence) + " "

	claimNeg := false
	for _, neg := range negations {
		if strings.Contains(claimLower, neg) {
			claimNeg = true
			break
		}
	}

	evidenceNeg := false
	for _, neg := range negations {
		if strings.Contains(evidenceLower, neg) {
			evidenceNeg = true
			break
		}
	}

	// Conflicting polarity on high-overlap sentences
	return claimNeg != evidenceNeg
}

func tokenize(text string) []string {
	clean := strings.ToLower(text)
	var tokens []string
	var current strings.Builder

	for _, r := range clean {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '$' || r == '%' {
			current.WriteRune(r)
		} else {
			if current.Len() > 0 {
				tokens = append(tokens, current.String())
				current.Reset()
			}
		}
	}

	if current.Len() > 0 {
		tokens = append(tokens, current.String())
	}

	return tokens
}
