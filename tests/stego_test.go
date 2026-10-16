package tests

import (
	"strings"
	"testing"
	"time"

	"github.com/Noooblien/agentvcr/pkg/stego"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

func TestStego_ZeroWidthUnicodeDecoding(t *testing.T) {
	detector := stego.NewDetector(stego.DefaultStegoConfig())

	// Embed "hi" in binary using ZWSP (0) and ZWNJ (1)
	// 'h' = 01101000, 'i' = 01101001
	zwBits := string([]rune{
		stego.RuneZWSP, stego.RuneZWNJ, stego.RuneZWNJ, stego.RuneZWSP, stego.RuneZWNJ, stego.RuneZWSP, stego.RuneZWSP, stego.RuneZWSP,
		stego.RuneZWSP, stego.RuneZWNJ, stego.RuneZWNJ, stego.RuneZWSP, stego.RuneZWNJ, stego.RuneZWSP, stego.RuneZWSP, stego.RuneZWNJ,
	})

	carrierText := "Hello" + zwBits + " World"

	findings, cleanText := detector.DetectZeroWidth(carrierText, 1, "tool_result")

	if len(findings) == 0 {
		t.Fatalf("Expected zero-width steganography finding, got 0")
	}

	if findings[0].Severity != "CRITICAL" {
		t.Errorf("Expected CRITICAL severity, got %s", findings[0].Severity)
	}

	if findings[0].DecodedSecret != "hi" {
		t.Errorf("Expected decoded secret 'hi', got '%s'", findings[0].DecodedSecret)
	}

	if cleanText != "Hello World" {
		t.Errorf("Expected sanitized text 'Hello World', got '%s'", cleanText)
	}
}

func TestStego_HomoglyphObfuscation(t *testing.T) {
	detector := stego.NewDetector(stego.DefaultStegoConfig())

	// "pаssword" where 'а' is Cyrillic \u0430
	taintedWord := "p\u0430ssword"
	text := "Please enter your " + taintedWord + " to continue."

	findings, normalized := detector.DetectHomoglyphs(text, 2, "llm_response")

	if len(findings) == 0 {
		t.Fatalf("Expected homoglyph finding, got 0")
	}

	if findings[0].ChannelType != stego.ChannelHomoglyph {
		t.Errorf("Expected ChannelHomoglyph, got %s", findings[0].ChannelType)
	}

	if !strings.Contains(normalized, "password") {
		t.Errorf("Expected normalized text to contain 'password', got '%s'", normalized)
	}
}

func TestStego_WhitespaceEncoding(t *testing.T) {
	detector := stego.NewDetector(stego.DefaultStegoConfig())

	// Line with trailing spaces and tabs: 4 spaces = 4 bits
	textWithTrailing := "function deployService() {\n    return true;    \t\t\n}"

	findings, cleaned := detector.DetectWhitespaceEncoding(textWithTrailing, 3, "tool_call")

	if len(findings) == 0 {
		t.Fatalf("Expected whitespace steganography finding, got 0")
	}

	if findings[0].ChannelType != stego.ChannelWhitespaceBinary {
		t.Errorf("Expected ChannelWhitespaceBinary, got %s", findings[0].ChannelType)
	}

	if strings.Contains(cleaned, "    \t\t") {
		t.Errorf("Expected cleaned text to strip trailing whitespace")
	}
}

func TestStego_ShannonEntropyAnomaly(t *testing.T) {
	normalProse := "This is a regular human sentence explaining system architecture and microservices."
	normalEntropy := stego.ComputeShannonEntropy(normalProse)

	if normalEntropy < 3.0 || normalEntropy > 4.5 {
		t.Errorf("Unexpected normal entropy: %.2f bits/char", normalEntropy)
	}

	highEntropyCiphertext := "qX8v!mZ@9#kL2$pW5%jN1&bV7*cM3(dF6)gH4_tY0+rE8~wQ"
	cipherEntropy := stego.ComputeShannonEntropy(highEntropyCiphertext)

	if cipherEntropy <= normalEntropy || cipherEntropy < 4.8 {
		t.Errorf("Expected high entropy for randomized ciphertext, got %.2f", cipherEntropy)
	}
}

func TestStego_FullSessionAnalysisAndScorecard(t *testing.T) {
	detector := stego.NewDetector(stego.DefaultStegoConfig())

	zwPayload := string([]rune{stego.RuneZWSP, stego.RuneZWNJ, stego.RuneZWSP, stego.RuneZWNJ, stego.RuneZWSP, stego.RuneZWSP, stego.RuneZWSP, stego.RuneZWNJ})

	session := &trace.Session{
		ID:        "session-stego-audit",
		AgentName: "covert-agent-01",
		Events: []trace.TraceEvent{
			{
				ID:        "evt-1",
				Type:      trace.EventToolCall,
				Payload:   map[string]interface{}{"query": "benign query" + zwPayload},
				Timestamp: time.Now().UTC(),
			},
			{
				ID:        "evt-2",
				Type:      trace.EventLLMResponse,
				Payload:   map[string]interface{}{"output": "Executing p\u0430ssword verification"},
				Timestamp: time.Now().UTC(),
			},
		},
	}

	report, err := detector.AnalyzeSession(session)
	if err != nil {
		t.Fatalf("AnalyzeSession failed: %v", err)
	}

	if !report.CovertChannelPresent {
		t.Errorf("Expected covert channel to be detected in session")
	}

	if report.CovertSignalsDetected < 2 {
		t.Errorf("Expected at least 2 covert signals detected, got %d", report.CovertSignalsDetected)
	}

	scorecard := stego.RenderStegoScorecard(report)
	if !strings.Contains(scorecard, "COVERT COMMUNICATION CHANNEL & STEGANOGRAPHY AUDIT") {
		t.Errorf("Scorecard header missing")
	}
	if !strings.Contains(scorecard, "COMPROMISED") {
		t.Errorf("Scorecard should state COMPROMISED status")
	}
}
