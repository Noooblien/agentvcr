package tests

import (
	"encoding/binary"
	"strings"
	"testing"

	"github.com/Noooblien/agentvcr/pkg/voice"
)

func TestVoice_PCMChunkIngestionAndRMS(t *testing.T) {
	cfg := voice.DefaultVoiceSessionConfig()
	rec := voice.NewVoiceRecorder("test_voice_1", cfg)

	// Generate 100ms of 440Hz sine wave (audible tone)
	sinePCM := voice.SynthesizeSineWavePCM(cfg.SampleRate, 440.0, 100, 0.5)

	pkt := rec.IngestPCMChunk(voice.TrackUserInbound, 1000, sinePCM)

	if pkt.SampleCount != 2400 { // 24000 samples/sec * 0.1s
		t.Errorf("Expected 2400 samples, got %d", pkt.SampleCount)
	}

	if pkt.DurationMs < 99.0 || pkt.DurationMs > 101.0 {
		t.Errorf("Expected ~100ms duration, got %.2f ms", pkt.DurationMs)
	}

	if pkt.RMSAmplitude <= 0.20 || pkt.RMSAmplitude >= 0.45 {
		t.Errorf("Expected RMS ~0.35 for 0.5 amplitude sine wave, got %.4f", pkt.RMSAmplitude)
	}

	if pkt.IsSilence {
		t.Errorf("Expected 440Hz audible wave to not be marked as silence")
	}

	// Ingest silence
	silencePCM := make([]byte, 2400*2) // 100ms of zeros
	silencePkt := rec.IngestPCMChunk(voice.TrackUserInbound, 1100, silencePCM)

	if !silencePkt.IsSilence {
		t.Errorf("Expected zeroed PCM to be marked as silence")
	}
	if silencePkt.Decibels != -100.0 {
		t.Errorf("Expected -100dB for total silence, got %.1f", silencePkt.Decibels)
	}
}

func TestVoice_VADSegmentation(t *testing.T) {
	cfg := voice.DefaultVoiceSessionConfig()
	rec := voice.NewVoiceRecorder("test_vad", cfg)

	speechChunk := voice.SynthesizeSineWavePCM(cfg.SampleRate, 300.0, 100, 0.4)
	silenceChunk := make([]byte, len(speechChunk))

	// User speaks for 300ms (1000ms to 1300ms)
	rec.IngestPCMChunk(voice.TrackUserInbound, 1000, speechChunk)
	rec.IngestPCMChunk(voice.TrackUserInbound, 1100, speechChunk)
	rec.IngestPCMChunk(voice.TrackUserInbound, 1200, speechChunk)

	// User stops speaking (silence at 1300ms and 1700ms)
	rec.IngestPCMChunk(voice.TrackUserInbound, 1300, silenceChunk)
	rec.IngestPCMChunk(voice.TrackUserInbound, 1700, silenceChunk)

	report := rec.FinalizeSession()

	if len(report.VADSegments) == 0 {
		t.Fatalf("Expected at least 1 VAD segment recorded")
	}

	seg := report.VADSegments[0]
	if seg.Track != voice.TrackUserInbound {
		t.Errorf("Expected TrackUserInbound, got %s", seg.Track)
	}
	if seg.StartMs != 1000 {
		t.Errorf("Expected start 1000ms, got %dms", seg.StartMs)
	}
	if seg.DurationMs < 250 {
		t.Errorf("Expected duration >= 250ms, got %dms", seg.DurationMs)
	}
}

func TestVoice_TurnaroundLatencyAndTurns(t *testing.T) {
	cfg := voice.DefaultVoiceSessionConfig()
	cfg.MaxTurnaroundLatencyMs = 800
	rec := voice.NewVoiceRecorder("test_turns", cfg)

	userSpeech := voice.SynthesizeSineWavePCM(cfg.SampleRate, 400.0, 100, 0.4)
	agentSpeech := voice.SynthesizeSineWavePCM(cfg.SampleRate, 600.0, 100, 0.4)
	silence := make([]byte, len(userSpeech))

	// User speech from 1000 to 1200ms
	rec.IngestPCMChunk(voice.TrackUserInbound, 1000, userSpeech)
	rec.IngestTranscript(voice.TrackUserInbound, "What is the capital of France?")
	rec.IngestPCMChunk(voice.TrackUserInbound, 1100, userSpeech)
	rec.IngestPCMChunk(voice.TrackUserInbound, 1200, silence)
	rec.IngestPCMChunk(voice.TrackUserInbound, 1600, silence) // triggers speech stop at 1200ms

	// Agent response starts at 1950ms (Turnaround = 1950 - 1200 = 750ms)
	rec.IngestPCMChunk(voice.TrackAgentOutbound, 1950, agentSpeech)
	rec.IngestTranscript(voice.TrackAgentOutbound, "The capital of France is Paris.")
	rec.IngestPCMChunk(voice.TrackAgentOutbound, 2050, agentSpeech)
	rec.IngestPCMChunk(voice.TrackAgentOutbound, 2150, silence)
	rec.IngestPCMChunk(voice.TrackAgentOutbound, 2600, silence) // triggers agent speech stop

	report := rec.FinalizeSession()

	if len(report.Turns) == 0 {
		t.Fatalf("Expected 1 synchronized turn recorded")
	}

	turn := report.Turns[0]
	if turn.TurnaroundLatencyMs != 750 {
		t.Errorf("Expected turnaround 750ms, got %dms", turn.TurnaroundLatencyMs)
	}
	if !strings.Contains(turn.UserTranscript, "capital of France") {
		t.Errorf("Expected user transcript in turn, got: %s", turn.UserTranscript)
	}
	if !report.SLAPassed {
		t.Errorf("Expected SLA to pass for 750ms turnaround (< 800ms)")
	}
}

func TestVoice_InterruptionDetection(t *testing.T) {
	cfg := voice.DefaultVoiceSessionConfig()
	rec := voice.NewVoiceRecorder("test_interruption", cfg)

	speech := voice.SynthesizeSineWavePCM(cfg.SampleRate, 500.0, 100, 0.4)

	// Agent is speaking at 2000ms - 2200ms
	rec.IngestPCMChunk(voice.TrackAgentOutbound, 2000, speech)
	rec.IngestPCMChunk(voice.TrackAgentOutbound, 2100, speech)

	// User interrupts / barges in at 2150ms
	rec.IngestPCMChunk(voice.TrackUserInbound, 2150, speech)

	report := rec.FinalizeSession()

	if len(report.Interruptions) == 0 {
		t.Fatalf("Expected interruption to be detected")
	}

	in := report.Interruptions[0]
	if in.UserSpeechStartMs != 2150 {
		t.Errorf("Expected user speech start at 2150ms, got %dms", in.UserSpeechStartMs)
	}
	if in.DetectionLatencyMs <= 0 || in.DetectionLatencyMs > 200 {
		t.Errorf("Expected detection latency between 1ms and 200ms, got %dms", in.DetectionLatencyMs)
	}
}

func TestVoice_WAVHeaderGeneration(t *testing.T) {
	pcmLen := 48000 // 1 sec of 24kHz 16-bit mono
	header := voice.GenerateWAVHeader(24000, 16, 1, pcmLen)

	if len(header) != 44 {
		t.Fatalf("Expected standard 44-byte WAV header, got %d bytes", len(header))
	}

	if string(header[0:4]) != "RIFF" {
		t.Errorf("Expected RIFF magic header")
	}
	if string(header[8:12]) != "WAVE" {
		t.Errorf("Expected WAVE format identifier")
	}
	if string(header[12:16]) != "fmt " {
		t.Errorf("Expected 'fmt ' subchunk")
	}

	sampleRate := binary.LittleEndian.Uint32(header[24:28])
	if sampleRate != 24000 {
		t.Errorf("Expected sample rate 24000, got %d", sampleRate)
	}

	dataSize := binary.LittleEndian.Uint32(header[40:44])
	if dataSize != uint32(pcmLen) {
		t.Errorf("Expected data size %d, got %d", pcmLen, dataSize)
	}
}

func TestVoice_ScorecardRendering(t *testing.T) {
	rep := &voice.VoiceSessionReport{
		SessionID:            "voice_scorecard_test",
		TotalDurationMs:      5400,
		TotalPackets:         54,
		UserAudioDurationMs:  1800.0,
		AgentAudioDurationMs: 2400.0,
		AvgTurnaroundMs:      580.0,
		P90TurnaroundMs:      620.0,
		AvgInterruptionMs:    110.0,
		RealtimeFactor:       0.78,
		PacketJitterP99Ms:    8.5,
		SLAPassed:            true,
		WaveformPreview:      "  ▂▃▅▆▇█▇▅▃▂  ",
		Turns: []voice.SynchronizedTurn{
			{
				TurnIndex:           1,
				TurnaroundLatencyMs: 580,
				UserTranscript:      "Book a table for two tonight.",
				AgentTranscript:     "I have reserved a table at 7 PM for two.",
				ToolCalls:           []string{"restaurant_reservation"},
			},
		},
		Interruptions: []voice.InterruptionEvent{
			{
				EventID:            "INT-001",
				UserSpeechStartMs:  3200,
				AgentCutoffMs:      3310,
				DetectionLatencyMs: 110,
				DiscardedAudioMs:   250,
			},
		},
	}

	card := voice.RenderVoiceScorecard(rep)
	if !strings.Contains(card, "REAL-TIME VOICE API FLIGHT RECORDER & ACOUSTIC TIMELINE") {
		t.Errorf("Expected banner in voice scorecard")
	}
	if !strings.Contains(card, "Acoustic Energy Waveform") {
		t.Errorf("Expected waveform section in scorecard")
	}
	if !strings.Contains(card, "PASS") {
		t.Errorf("Expected PASS SLA verdict badge")
	}
	if !strings.Contains(card, "restaurant_reservation") {
		t.Errorf("Expected tool call to be rendered in turn")
	}
}
