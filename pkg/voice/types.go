package voice

import (
	"time"
)

// AudioFormat represents the encoding standard of intercepted voice chunks.
type AudioFormat string

const (
	FormatPCM24kHz16BitMono AudioFormat = "pcm16_24khz_mono" // OpenAI Realtime standard (24,000 samples/sec, 16-bit signed LE)
	FormatPCM16kHz16BitMono AudioFormat = "pcm16_16khz_mono" // Telephony / Whisper standard (16,000 samples/sec)
	FormatOpus              AudioFormat = "opus"
)

// RealtimeEventType specifies standard WebSocket event types for OpenAI Realtime & Gemini Live APIs.
type RealtimeEventType string

const (
	EventInputAudioBufferAppend       RealtimeEventType = "input_audio_buffer.append"
	EventInputAudioBufferCommit       RealtimeEventType = "input_audio_buffer.commit"
	EventInputAudioBufferCleared      RealtimeEventType = "input_audio_buffer.cleared"
	EventInputAudioBufferSpeechStart  RealtimeEventType = "input_audio_buffer.speech_started"
	EventInputAudioBufferSpeechStop   RealtimeEventType = "input_audio_buffer.speech_stopped"
	EventResponseAudioDelta           RealtimeEventType = "response.audio.delta"
	EventResponseAudioTranscriptDelta RealtimeEventType = "response.audio_transcript.delta"
	EventResponseFunctionCallDelta    RealtimeEventType = "response.function_call_arguments.delta"
	EventResponseAudioDone            RealtimeEventType = "response.audio.done"
	EventResponseInterrupted          RealtimeEventType = "response.interrupted"
)

// AudioTrack specifies whether audio originates from client (user mic) or server (agent synthetic voice).
type AudioTrack string

const (
	TrackUserInbound  AudioTrack = "user_inbound"
	TrackAgentOutbound AudioTrack = "agent_outbound"
)

// AudioPacket encapsulates an atomic timestamped audio chunk with acoustic metrics.
type AudioPacket struct {
	Index           int        `json:"index"`
	Track           AudioTrack `json:"track"`
	TimestampMs     int64      `json:"timestamp_ms"`
	DurationMs      float64    `json:"duration_ms"`
	Format          AudioFormat `json:"format"`
	PayloadBytes    int        `json:"payload_bytes"`
	SampleCount     int        `json:"sample_count"`
	RMSAmplitude    float64    `json:"rms_amplitude"` // Root Mean Square energy [0.0 - 1.0]
	Decibels        float64    `json:"decibels"`      // dBFS [-100dB to 0dB]
	IsSilence       bool       `json:"is_silence"`
	RawBytesBase64  string     `json:"raw_bytes_base64,omitempty"`
}

// VADSegment represents a continuous interval of detected human or agent speech.
type VADSegment struct {
	ID          string     `json:"id"`
	Track       AudioTrack `json:"track"`
	StartMs     int64      `json:"start_ms"`
	EndMs       int64      `json:"end_ms"`
	DurationMs  int64      `json:"duration_ms"`
	AvgEnergy   float64    `json:"avg_energy"`
	Transcript  string     `json:"transcript,omitempty"`
	ChunkCount  int        `json:"chunk_count"`
}

// InterruptionEvent records a user speech barge-in that interrupted in-flight agent speech.
type InterruptionEvent struct {
	EventID              string  `json:"event_id"`
	UserSpeechStartMs    int64   `json:"user_speech_start_ms"`
	AgentCutoffMs        int64   `json:"agent_cutoff_ms"`
	DetectionLatencyMs   int64   `json:"detection_latency_ms"` // Time between user onset and agent cutoff
	CutoffChunkIndex     int     `json:"cutoff_chunk_index"`
	DiscardedAudioMs     float64 `json:"discarded_audio_ms"`
	AgentTranscriptSoFar string  `json:"agent_transcript_so_far"`
}

// SynchronizedTurn aligns a user turn with model turnaround response and tool executions.
type SynchronizedTurn struct {
	TurnIndex            int                 `json:"turn_index"`
	UserSpeechStartMs    int64               `json:"user_speech_start_ms"`
	UserSpeechStopMs     int64               `json:"user_speech_stop_ms"`
	UserTranscript       string              `json:"user_transcript"`
	FirstAgentAudioMs    int64               `json:"first_agent_audio_ms"`
	TurnaroundLatencyMs  int64               `json:"turnaround_latency_ms"` // UserSpeechStopMs -> FirstAgentAudioMs
	AgentTranscript      string              `json:"agent_transcript"`
	ToolCalls            []string            `json:"tool_calls,omitempty"`
	WasInterrupted       bool                `json:"was_interrupted"`
	Interruption         *InterruptionEvent  `json:"interruption,omitempty"`
}

// VoiceSessionConfig configures parameters for real-time audio flight recording.
type VoiceSessionConfig struct {
	Format                  AudioFormat `json:"format"`
	SampleRate              int         `json:"sample_rate"`       // e.g. 24000
	BitDepth                int         `json:"bit_depth"`         // e.g. 16
	Channels                int         `json:"channels"`          // 1 (mono)
	SilenceThresholdRMS     float64     `json:"silence_threshold"` // RMS below this is silence (e.g. 0.015)
	MaxTurnaroundLatencyMs  int64       `json:"max_turnaround_latency_ms"` // SLA (e.g. 800ms)
	MaxInterruptionLatencyMs int64      `json:"max_interruption_latency_ms"` // SLA (e.g. 350ms)
}

// DefaultVoiceSessionConfig returns standard production defaults for OpenAI Realtime & Gemini Live APIs.
func DefaultVoiceSessionConfig() VoiceSessionConfig {
	return VoiceSessionConfig{
		Format:                  FormatPCM24kHz16BitMono,
		SampleRate:              24000,
		BitDepth:                16,
		Channels:                1,
		SilenceThresholdRMS:     0.012,
		MaxTurnaroundLatencyMs:  800,
		MaxInterruptionLatencyMs: 350,
	}
}

// VoiceSessionReport encapsulates the audited flight recording telemetry for a real-time voice session.
type VoiceSessionReport struct {
	SessionID             string              `json:"session_id"`
	RecordedAt            time.Time           `json:"recorded_at"`
	TotalDurationMs       int64               `json:"total_duration_ms"`
	TotalPackets          int                 `json:"total_packets"`
	UserAudioDurationMs   float64             `json:"user_audio_duration_ms"`
	AgentAudioDurationMs  float64             `json:"agent_audio_duration_ms"`
	VADSegments           []VADSegment        `json:"vad_segments"`
	Turns                 []SynchronizedTurn  `json:"turns"`
	Interruptions         []InterruptionEvent `json:"interruptions"`
	AvgTurnaroundMs       float64             `json:"avg_turnaround_ms"`
	P90TurnaroundMs       float64             `json:"p90_turnaround_ms"`
	AvgInterruptionMs     float64             `json:"avg_interruption_ms"`
	RealtimeFactor        float64             `json:"realtime_factor"` // (User+Agent audio ms) / Session wall-clock ms
	PacketJitterP99Ms     float64             `json:"packet_jitter_p99_ms"`
	SLAPassed             bool                `json:"sla_passed"`
	WaveformPreview       string              `json:"waveform_preview"`
}
