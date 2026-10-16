package stego

import (
	"encoding/json"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Zero-width Unicode runes
const (
	RuneZWSP = '\u200B' // Zero Width Space (often bit 0)
	RuneZWNJ = '\u200C' // Zero Width Non-Joiner (often bit 1)
	RuneZWJ  = '\u200D' // Zero Width Joiner
	RuneBOM  = '\uFEFF' // Byte Order Mark / Zero Width No-Break Space
	RuneWJ   = '\u2060' // Word Joiner
)

var (
	// Homoglyphs mapping common non-Latin lookalikes to Latin characters
	homoglyphMap = map[rune]rune{
		// Cyrillic
		'\u0430': 'a', // Cyrillic small letter a
		'\u0441': 'c', // Cyrillic small letter es
		'\u0435': 'e', // Cyrillic small letter ie
		'\u043E': 'o', // Cyrillic small letter o
		'\u0440': 'p', // Cyrillic small letter er
		'\u0445': 'x', // Cyrillic small letter ha
		'\u0443': 'y', // Cyrillic small letter u
		'\u0456': 'i', // Cyrillic small letter byelorussian-ukrainian i
		'\u0410': 'A', // Cyrillic capital letter A
		'\u0412': 'B', // Cyrillic capital letter Ve
		'\u0415': 'E', // Cyrillic capital letter Ie
		'\u041A': 'K', // Cyrillic capital letter Ka
		'\u041C': 'M', // Cyrillic capital letter Em
		'\u041D': 'H', // Cyrillic capital letter En
		'\u041E': 'O', // Cyrillic capital letter O
		'\u0420': 'P', // Cyrillic capital letter Er
		'\u0421': 'C', // Cyrillic capital letter Es
		'\u0422': 'T', // Cyrillic capital letter Te
		'\u0425': 'X', // Cyrillic capital letter Ha
		// Greek
		'\u03BF': 'o', // Greek small letter omicron
		'\u03B1': 'a', // Greek small letter alpha
		'\u0391': 'A', // Greek capital letter Alpha
		'\u0392': 'B', // Greek capital letter Beta
		'\u039F': 'O', // Greek capital letter Omicron
	}
)

// Detector analyzes agent communications for steganographic covert channels.
type Detector struct {
	Config StegoConfig
}

// NewDetector creates a new covert communication channel analyzer.
func NewDetector(cfg StegoConfig) *Detector {
	return &Detector{Config: cfg}
}

// ComputeShannonEntropy calculates character-level information entropy in bits per character.
func ComputeShannonEntropy(text string) float64 {
	if len(text) == 0 {
		return 0.0
	}
	freqs := make(map[rune]int)
	runes := []rune(text)
	for _, r := range runes {
		freqs[r]++
	}

	total := float64(len(runes))
	entropy := 0.0
	for _, count := range freqs {
		p := float64(count) / total
		entropy -= p * math.Log2(p)
	}
	return entropy
}

// DetectZeroWidth looks for invisible zero-width unicode runs and attempts binary decoding.
func (d *Detector) DetectZeroWidth(text string, eventIdx int, eventType string) ([]StegoFinding, string) {
	var findings []StegoFinding
	var cleanedRunes []rune
	var zwRun []rune
	firstOffset := -1

	runes := []rune(text)
	for i, r := range runes {
		switch r {
		case RuneZWSP, RuneZWNJ, RuneZWJ, RuneBOM, RuneWJ:
			if firstOffset == -1 {
				firstOffset = i
			}
			zwRun = append(zwRun, r)
		default:
			cleanedRunes = append(cleanedRunes, r)
		}
	}

	if len(zwRun) > 0 {
		// Attempt binary decoding (ZWSP = 0, ZWNJ = 1)
		var bits strings.Builder
		for _, r := range zwRun {
			if r == RuneZWSP {
				bits.WriteByte('0')
			} else if r == RuneZWNJ {
				bits.WriteByte('1')
			}
		}

		decodedText := decodeBinaryString(bits.String())
		evidence := fmt.Sprintf("%d invisible zero-width runes detected (Unicode: U+200B/U+200C/U+200D)", len(zwRun))
		desc := "Covert zero-width Unicode steganography channel detected"
		if decodedText != "" {
			desc = fmt.Sprintf("Extracted hidden binary payload: '%s'", decodedText)
		}

		findings = append(findings, StegoFinding{
			FindingID:     fmt.Sprintf("stego-zw-%d-%d", eventIdx, firstOffset),
			ChannelType:   ChannelZeroWidth,
			Severity:      "CRITICAL",
			Confidence:    0.99,
			Description:   desc,
			DecodedSecret: decodedText,
			RawEvidence:   evidence,
			CharOffset:    firstOffset,
			EventIndex:    eventIdx,
			EventType:     eventType,
			Timestamp:     time.Now().UTC(),
		})
	}

	return findings, string(cleanedRunes)
}

func decodeBinaryString(bits string) string {
	if len(bits) < 8 {
		return ""
	}
	var sb strings.Builder
	for i := 0; i+8 <= len(bits); i += 8 {
		byteStr := bits[i : i+8]
		var val byte
		for b := 0; b < 8; b++ {
			val = (val << 1) | (byteStr[b] - '0')
		}
		if val >= 32 && val <= 126 { // Printable ASCII
			sb.WriteByte(val)
		}
	}
	return sb.String()
}

// DetectHomoglyphs scans for non-Latin script lookalike characters substituted inside Latin words.
func (d *Detector) DetectHomoglyphs(text string, eventIdx int, eventType string) ([]StegoFinding, string) {
	var findings []StegoFinding
	var normalized strings.Builder

	words := strings.Fields(text)
	for _, word := range words {
		hasLatin := false
		hasHomoglyph := false
		var wordNorm strings.Builder
		var taintedChars []string

		for _, r := range word {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				hasLatin = true
				wordNorm.WriteRune(r)
			} else if replacement, ok := homoglyphMap[r]; ok {
				hasHomoglyph = true
				wordNorm.WriteRune(replacement)
				taintedChars = append(taintedChars, fmt.Sprintf("%c (U+%04X -> %c)", r, r, replacement))
			} else {
				wordNorm.WriteRune(r)
			}
		}

		if hasLatin && hasHomoglyph {
			findings = append(findings, StegoFinding{
				FindingID:   fmt.Sprintf("stego-hg-%d-%s", eventIdx, word),
				ChannelType: ChannelHomoglyph,
				Severity:    "HIGH",
				Confidence:  0.95,
				Description: fmt.Sprintf("Mixed-script homoglyph obfuscation in word '%s' (normalized: '%s')", word, wordNorm.String()),
				RawEvidence: strings.Join(taintedChars, ", "),
				EventIndex:  eventIdx,
				EventType:   eventType,
				Timestamp:   time.Now().UTC(),
			})
		}

		if normalized.Len() > 0 {
			normalized.WriteByte(' ')
		}
		normalized.WriteString(wordNorm.String())
	}

	return findings, normalized.String()
}

// DetectWhitespaceEncoding inspects trailing whitespaces on lines for binary encoding.
func (d *Detector) DetectWhitespaceEncoding(text string, eventIdx int, eventType string) ([]StegoFinding, string) {
	var findings []StegoFinding
	lines := strings.Split(text, "\n")
	var cleanedLines []string
	var allBits strings.Builder

	for lineIdx, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		trailing := line[len(trimmed):]
		cleanedLines = append(cleanedLines, trimmed)

		if len(trailing) >= 4 { // Minimum trailing whitespace indicating bit encoding
			for _, r := range trailing {
				if r == ' ' {
					allBits.WriteByte('0')
				} else if r == '\t' {
					allBits.WriteByte('1')
				}
			}

			findings = append(findings, StegoFinding{
				FindingID:   fmt.Sprintf("stego-ws-%d-L%d", eventIdx, lineIdx+1),
				ChannelType: ChannelWhitespaceBinary,
				Severity:    "MEDIUM",
				Confidence:  0.88,
				Description: fmt.Sprintf("Trailing whitespace steganography on line %d (%d bytes)", lineIdx+1, len(trailing)),
				RawEvidence: fmt.Sprintf("Spaces/Tabs count: %d", len(trailing)),
				EventIndex:  eventIdx,
				EventType:   eventType,
				Timestamp:   time.Now().UTC(),
			})
		}
	}

	decoded := decodeBinaryString(allBits.String())
	if decoded != "" && len(findings) > 0 {
		findings[0].DecodedSecret = decoded
		findings[0].Description += fmt.Sprintf(" | Extracted secret: '%s'", decoded)
	}

	return findings, strings.Join(cleanedLines, "\n")
}

// AnalyzeText conducts multi-vector steganographic analysis on a single text string.
func (d *Detector) AnalyzeText(text string, eventIdx int, eventType string) ([]StegoFinding, string) {
	var allFindings []StegoFinding
	currentText := text

	// 1. Zero-width Unicode detection
	if d.Config.CheckZeroWidth {
		zwFindings, cleanZW := d.DetectZeroWidth(currentText, eventIdx, eventType)
		allFindings = append(allFindings, zwFindings...)
		currentText = cleanZW
	}

	// 2. Homoglyph confusable detection
	if d.Config.CheckHomoglyphs {
		hgFindings, cleanHG := d.DetectHomoglyphs(currentText, eventIdx, eventType)
		allFindings = append(allFindings, hgFindings...)
		currentText = cleanHG
	}

	// 3. Whitespace binary detection
	if d.Config.CheckWhitespace {
		wsFindings, cleanWS := d.DetectWhitespaceEncoding(currentText, eventIdx, eventType)
		allFindings = append(allFindings, wsFindings...)
		currentText = cleanWS
	}

	// 4. Shannon Entropy anomaly
	entropy := ComputeShannonEntropy(text)
	if len(text) > 40 && entropy >= d.Config.MinEntropyAnomalyThreshold {
		allFindings = append(allFindings, StegoFinding{
			FindingID:   fmt.Sprintf("stego-ent-%d", eventIdx),
			ChannelType: ChannelEntropyAnomaly,
			Severity:    "MEDIUM",
			Confidence:  0.82,
			Description: fmt.Sprintf("High Shannon entropy anomaly (%.2f bits/char >= threshold %.2f) indicating potential ciphertext or token steganography", entropy, d.Config.MinEntropyAnomalyThreshold),
			RawEvidence: fmt.Sprintf("Text length: %d chars | Entropy: %.3f", len(text), entropy),
			EventIndex:  eventIdx,
			EventType:   eventType,
			Timestamp:   time.Now().UTC(),
		})
	}

	return allFindings, currentText
}

// AnalyzeSession inspects an entire recorded trace session for covert channels.
func (d *Detector) AnalyzeSession(session *trace.Session) (*StegoReport, error) {
	start := time.Now()
	if session == nil {
		return nil, fmt.Errorf("session is nil")
	}

	report := &StegoReport{
		SessionID:           session.ID,
		AgentName:           session.AgentName,
		ChannelDistribution: make(map[CovertChannelType]int),
		Findings:            make([]StegoFinding, 0),
	}

	totalEntropy := 0.0
	textEvents := 0

	for idx, evt := range session.Events {
		var extractedTexts []string

		// Check payload strings
		for _, v := range evt.Payload {
			if strVal, ok := v.(string); ok && len(strVal) > 0 {
				extractedTexts = append(extractedTexts, strVal)
			} else if mapVal, ok := v.(map[string]interface{}); ok {
				b, _ := json.Marshal(mapVal)
				extractedTexts = append(extractedTexts, string(b))
			}
		}

		for _, txt := range extractedTexts {
			textEvents++
			ent := ComputeShannonEntropy(txt)
			totalEntropy += ent

			findings, _ := d.AnalyzeText(txt, idx+1, string(evt.Type))
			for _, f := range findings {
				report.Findings = append(report.Findings, f)
				report.ChannelDistribution[f.ChannelType]++
				if f.Severity == "CRITICAL" {
					report.CriticalBreaches++
				}
				if f.DecodedSecret != "" {
					report.DecodedSecrets = append(report.DecodedSecrets, f.DecodedSecret)
				}
			}
		}
	}

	report.TotalEventsAnalyzed = len(session.Events)
	report.CovertSignalsDetected = len(report.Findings)
	report.CovertChannelPresent = len(report.Findings) > 0
	if textEvents > 0 {
		report.AverageEntropy = totalEntropy / float64(textEvents)
	}
	report.ExecutionDuration = time.Since(start)

	return report, nil
}

// RenderStegoScorecard formats an audit report for terminal inspection.
func RenderStegoScorecard(report *StegoReport) string {
	var sb strings.Builder

	sb.WriteString("\033[1;35m========================================================================================\033[0m\n")
	sb.WriteString("  \033[1;37mAGENTVCR // COVERT COMMUNICATION CHANNEL & STEGANOGRAPHY AUDIT\033[0m\n")
	sb.WriteString("\033[1;35m========================================================================================\033[0m\n")

	sb.WriteString(fmt.Sprintf("  \033[1;33mSession ID:\033[0m           %s | Agent: %s\n", report.SessionID, report.AgentName))
	sb.WriteString(fmt.Sprintf("  \033[1;33mEvents Evaluated:\033[0m     %d events (Scan Time: %v)\n",
		report.TotalEventsAnalyzed, report.ExecutionDuration))
	sb.WriteString(fmt.Sprintf("  \033[1;33mAverage Entropy:\033[0m      %.2f bits/char\n", report.AverageEntropy))

	if !report.CovertChannelPresent {
		sb.WriteString("  \033[1;33mSteganography Status:\033[0m \033[1;32m[CLEAN] ZERO COVERT COMMUNICATION CHANNELS DETECTED\033[0m\n")
	} else {
		sb.WriteString(fmt.Sprintf("  \033[1;33mSteganography Status:\033[0m \033[1;31m[COMPROMISED] %d COVERT SIGNALS DETECTED (%d CRITICAL)\033[0m\n",
			report.CovertSignalsDetected, report.CriticalBreaches))
	}
	sb.WriteString("\033[1;34m----------------------------------------------------------------------------------------\033[0m\n")

	if len(report.Findings) == 0 {
		sb.WriteString("  \033[1;32m✓\033[0m Zero-Width Steganography: Clean; zero hidden binary Unicode encoding.\n")
		sb.WriteString("  \033[1;32m✓\033[0m Homoglyph Confusables: Clean; all characters strictly native to declared script.\n")
		sb.WriteString("  \033[1;32m✓\033[0m Whitespace Signatures: Clean; zero trailing space/tab binary channels.\n")
	} else {
		sb.WriteString(fmt.Sprintf("  \033[1;31mCOVERT CHANNEL FINDINGS & FORENSIC EVIDENCE (%d):\033[0m\n", len(report.Findings)))
		for i, f := range report.Findings {
			sb.WriteString(fmt.Sprintf("  %d. [\033[1;31m%s\033[0m] Event %d (%s) — Severity: %s\n",
				i+1, f.ChannelType, f.EventIndex, f.EventType, f.Severity))
			sb.WriteString(fmt.Sprintf("     \033[1;33mEvidence:\033[0m %s\n", f.Description))
			if f.DecodedSecret != "" {
				sb.WriteString(fmt.Sprintf("     \033[1;32mDECODED SECRET:\033[0m \033[1;37m\"%s\"\033[0m\n", f.DecodedSecret))
			}
		}
	}

	sb.WriteString("\033[1;35m========================================================================================\033[0m\n")
	return sb.String()
}
