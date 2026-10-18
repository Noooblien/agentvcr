package watermark

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Zero-width Unicode markers for binary bit encoding:
// 0: Zero Width Space (\u200B)
// 1: Zero Width Non-Joiner (\u200C)
// Delimiter / Header: Zero Width Joiner (\u200D)
const (
	ZWBitZero  = '\u200B'
	ZWBitOne   = '\u200C'
	ZWDelim    = '\u200D'
)

// Engine handles watermarking generation, embedding, extraction, and cryptographic attestation.
type Engine struct {
	mu     sync.RWMutex
	Config WatermarkConfig
}

// NewEngine creates an initialized Watermark Engine.
func NewEngine(cfg WatermarkConfig) *Engine {
	if cfg.Strategy == "" {
		cfg.Strategy = StrategyZeroWidthUnicode
	}
	if cfg.SecretKey == "" {
		cfg.SecretKey = "agentvcr-provenance-default-secret-key"
	}
	if cfg.Redundancy <= 0 {
		cfg.Redundancy = 2
	}
	return &Engine{
		Config: cfg,
	}
}

// GeneratePayload creates a signed watermark payload for an agent session.
func (e *Engine) GeneratePayload(sessionID, agentID, model string) WatermarkPayload {
	now := time.Now().UTC()
	raw := fmt.Sprintf("%s:%s:%s:%s:%d", e.Config.OrgID, sessionID, agentID, model, now.Unix())
	h := sha256.Sum256([]byte(raw))
	payloadHash := hex.EncodeToString(h[:])[:16]

	mac := hmac.New(sha256.New, []byte(e.Config.SecretKey))
	mac.Write([]byte(payloadHash))
	sig := hex.EncodeToString(mac.Sum(nil))[:16]

	return WatermarkPayload{
		SessionID:   sessionID,
		AgentID:     agentID,
		Model:       model,
		OrgID:       e.Config.OrgID,
		Timestamp:   now,
		PayloadHash: payloadHash, // Compact 16-char hash
		Signature:   sig,         // Compact 16-char HMAC
	}
}

// EncodePayloadToBits serializes the WatermarkPayload into a compact binary bit string.
func EncodePayloadToBits(payload WatermarkPayload) (string, error) {
	bytes, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	var sb strings.Builder
	for _, b := range bytes {
		sb.WriteString(fmt.Sprintf("%08b", b))
	}
	return sb.String(), nil
}

// DecodeBitsToPayload deserializes a binary bit string back into a WatermarkPayload.
func DecodeBitsToPayload(bits string) (*WatermarkPayload, error) {
	if len(bits)%8 != 0 {
		return nil, fmt.Errorf("invalid bit length: %d is not a multiple of 8", len(bits))
	}

	bytes := make([]byte, len(bits)/8)
	for i := 0; i < len(bytes); i++ {
		byteBits := bits[i*8 : (i+1)*8]
		var val byte
		for j := 0; j < 8; j++ {
			if byteBits[j] == '1' {
				val |= 1 << (7 - j)
			}
		}
		bytes[i] = val
	}

	var payload WatermarkPayload
	if err := json.Unmarshal(bytes, &payload); err != nil {
		return nil, fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	return &payload, nil
}

// EmbedWatermark embeds the cryptographic watermark into the target text using the configured strategy.
func (e *Engine) EmbedWatermark(text string, payload WatermarkPayload) (string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	bits, err := EncodePayloadToBits(payload)
	if err != nil {
		return text, err
	}

	switch e.Config.Strategy {
	case StrategyZeroWidthUnicode:
		return embedZeroWidth(text, bits), nil
	case StrategyHeaderSignature:
		return embedHeaderSignature(text, payload), nil
	default:
		return embedZeroWidth(text, bits), nil
	}
}

func embedZeroWidth(text string, bits string) string {
	var zw strings.Builder
	zw.WriteRune(ZWDelim) // Start delimiter

	for _, bit := range bits {
		if bit == '0' {
			zw.WriteRune(ZWBitZero)
		} else {
			zw.WriteRune(ZWBitOne)
		}
	}
	zw.WriteRune(ZWDelim) // End delimiter

	// Inject watermark at the first sentence boundary or after first word
	insertIdx := strings.Index(text, ". ")
	if insertIdx != -1 {
		return text[:insertIdx+1] + zw.String() + text[insertIdx+1:]
	}
	spaceIdx := strings.Index(text, " ")
	if spaceIdx != -1 {
		return text[:spaceIdx] + zw.String() + text[spaceIdx:]
	}
	return text + zw.String()
}

func embedHeaderSignature(text string, payload WatermarkPayload) string {
	header := fmt.Sprintf("<!-- AGENTVCR-WATERMARK: org=%s session=%s hash=%s sig=%s -->\n",
		payload.OrgID, payload.SessionID, payload.PayloadHash, payload.Signature)
	return header + text
}

// ExtractWatermark scans text to detect, extract, and cryptographically verify embedded watermarks.
func (e *Engine) ExtractWatermark(text string) ExtractionResult {
	e.mu.RLock()
	defer e.mu.RUnlock()

	// 1. Try Zero-Width extraction
	if strings.ContainsRune(text, ZWDelim) {
		res := extractZeroWidth(text, e.Config.SecretKey)
		if res.Detected {
			return res
		}
	}

	// 2. Try Header Signature extraction
	if strings.Contains(text, "<!-- AGENTVCR-WATERMARK:") {
		res := extractHeaderSignature(text, e.Config.SecretKey)
		if res.Detected {
			return res
		}
	}

	return ExtractionResult{
		Detected:      false,
		Strategy:      e.Config.Strategy,
		BitConfidence: 0.0,
		Tampered:      false,
	}
}

func extractZeroWidth(text string, secretKey string) ExtractionResult {
	firstDelim := strings.IndexRune(text, ZWDelim)
	if firstDelim == -1 {
		return ExtractionResult{Detected: false}
	}

	rest := text[firstDelim+1:]
	secondDelim := strings.IndexRune(rest, ZWDelim)
	if secondDelim == -1 {
		return ExtractionResult{Detected: false}
	}

	encodedBlock := rest[:secondDelim]
	var bitSb strings.Builder

	for _, r := range encodedBlock {
		if r == ZWBitZero {
			bitSb.WriteByte('0')
		} else if r == ZWBitOne {
			bitSb.WriteByte('1')
		}
	}

	bits := bitSb.String()
	payload, err := DecodeBitsToPayload(bits)
	if err != nil {
		return ExtractionResult{
			Detected:      true,
			Strategy:      StrategyZeroWidthUnicode,
			Tampered:      true,
			BitConfidence: 0.35,
			ExtractedBits: len(bits),
		}
	}

	// Verify cryptographic signature
	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(payload.PayloadHash))
	expectedSig := hex.EncodeToString(mac.Sum(nil))[:16]

	tampered := !hmac.Equal([]byte(expectedSig), []byte(payload.Signature))

	confidence := 1.0
	if tampered {
		confidence = 0.50
	}

	return ExtractionResult{
		Detected:         true,
		Strategy:         StrategyZeroWidthUnicode,
		Payload:          payload,
		BitConfidence:    confidence,
		Tampered:         tampered,
		ExtractedBits:    len(bits),
		RawMatchedString: fmt.Sprintf("[%d zero-width bits]", len(bits)),
	}
}

func extractHeaderSignature(text string, secretKey string) ExtractionResult {
	start := strings.Index(text, "<!-- AGENTVCR-WATERMARK:")
	if start == -1 {
		return ExtractionResult{Detected: false}
	}
	end := strings.Index(text[start:], "-->")
	if end == -1 {
		return ExtractionResult{Detected: false}
	}

	headerContent := text[start : start+end+3]
	parts := strings.Fields(headerContent)

	payload := &WatermarkPayload{}
	for _, p := range parts {
		if strings.HasPrefix(p, "org=") {
			payload.OrgID = strings.TrimPrefix(p, "org=")
		} else if strings.HasPrefix(p, "session=") {
			payload.SessionID = strings.TrimPrefix(p, "session=")
		} else if strings.HasPrefix(p, "hash=") {
			payload.PayloadHash = strings.TrimPrefix(p, "hash=")
		} else if strings.HasPrefix(p, "sig=") {
			payload.Signature = strings.TrimPrefix(p, "sig=")
		}
	}

	mac := hmac.New(sha256.New, []byte(secretKey))
	mac.Write([]byte(payload.PayloadHash))
	expectedSig := hex.EncodeToString(mac.Sum(nil))[:16]

	tampered := !hmac.Equal([]byte(expectedSig), []byte(payload.Signature))

	return ExtractionResult{
		Detected:         true,
		Strategy:         StrategyHeaderSignature,
		Payload:          payload,
		BitConfidence:    1.0,
		Tampered:         tampered,
		RawMatchedString: headerContent,
	}
}

// StripWatermarks sanitizes and removes all embedded zero-width watermarks from text for clean replay.
func StripWatermarks(text string) string {
	text = strings.ReplaceAll(text, string(ZWDelim), "")
	text = strings.ReplaceAll(text, string(ZWBitZero), "")
	text = strings.ReplaceAll(text, string(ZWBitOne), "")
	return text
}

// AuditTrajectory scans an entire trace session to verify watermark integrity and provenance.
func (e *Engine) AuditTrajectory(session *trace.Session) *WatermarkAuditReport {
	e.mu.RLock()
	defer e.mu.RUnlock()

	scanned := 0
	extracted := 0
	tamperedCount := 0
	results := make([]ExtractionResult, 0)

	for _, evt := range session.Events {
		if content, ok := evt.Payload["content"].(string); ok && len(content) > 0 {
			scanned++
			res := e.ExtractWatermark(content)
			results = append(results, res)
			if res.Detected {
				extracted++
				if res.Tampered {
					tamperedCount++
				}
			}
		}
	}

	integrityScore := 10.0
	status := "VERIFIED"
	if extracted == 0 {
		status = "NOT_DETECTED"
		integrityScore = 0.0
	} else if tamperedCount > 0 {
		status = "TAMPERED"
		integrityScore = 2.0
	}

	return &WatermarkAuditReport{
		ReportID:              fmt.Sprintf("wm-report-%s", session.ID),
		SessionID:             session.ID,
		Strategy:              e.Config.Strategy,
		TotalTurnsScanned:     scanned,
		WatermarksEmbedded:    scanned, // Expected embedded count
		WatermarksExtracted:   extracted,
		AttestationValid:      extracted > 0 && tamperedCount == 0,
		OverallIntegrityScore: integrityScore,
		Status:                status,
		Results:               results,
		TraceSession:          session,
		Passed:                extracted > 0 && tamperedCount == 0,
	}
}
