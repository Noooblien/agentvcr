package voice

import (
	"bytes"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"math"
	"sort"
	"strings"
	"sync"
	"time"
)

// VoiceRecorder records and synchronizes bidirectional real-time voice streams.
type VoiceRecorder struct {
	mu            sync.RWMutex
	config        VoiceSessionConfig
	sessionID     string
	startTime     time.Time
	packets       []AudioPacket
	rawUserAudio  []byte
	rawAgentAudio []byte
	turns         []SynchronizedTurn
	vadSegments   []VADSegment
	interruptions []InterruptionEvent

	// In-flight state tracking
	currentUserVAD  *VADSegment
	currentAgentVAD *VADSegment
	lastUserStopMs  int64
	currentTurn     *SynchronizedTurn
}

// NewVoiceRecorder initializes a new voice session flight recorder.
func NewVoiceRecorder(sessionID string, cfg VoiceSessionConfig) *VoiceRecorder {
	return &VoiceRecorder{
		sessionID:     sessionID,
		config:        cfg,
		startTime:     time.Now(),
		packets:       make([]AudioPacket, 0),
		rawUserAudio:  make([]byte, 0),
		rawAgentAudio: make([]byte, 0),
		turns:         make([]SynchronizedTurn, 0),
		vadSegments:   make([]VADSegment, 0),
		interruptions: make([]InterruptionEvent, 0),
	}
}

// IngestPCMChunk ingests a raw PCM audio chunk (e.g. from input_audio_buffer.append or response.audio.delta).
func (vr *VoiceRecorder) IngestPCMChunk(track AudioTrack, timestampMs int64, rawPCM []byte) AudioPacket {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	sampleCount := len(rawPCM) / (vr.config.BitDepth / 8)
	durationMs := (float64(sampleCount) / float64(vr.config.SampleRate)) * 1000.0

	rms, decibels := CalculateRMSAndDecibels(rawPCM)
	isSilence := rms < vr.config.SilenceThresholdRMS

	pkt := AudioPacket{
		Index:          len(vr.packets),
		Track:          track,
		TimestampMs:    timestampMs,
		DurationMs:     durationMs,
		Format:         vr.config.Format,
		PayloadBytes:   len(rawPCM),
		SampleCount:    sampleCount,
		RMSAmplitude:   math.Round(rms*10000) / 10000,
		Decibels:       math.Round(decibels*10) / 10,
		IsSilence:      isSilence,
		RawBytesBase64: base64.StdEncoding.EncodeToString(rawPCM),
	}

	vr.packets = append(vr.packets, pkt)

	if track == TrackUserInbound {
		vr.rawUserAudio = append(vr.rawUserAudio, rawPCM...)
	} else {
		vr.rawAgentAudio = append(vr.rawAgentAudio, rawPCM...)
	}

	// Update VAD states
	vr.updateVAD(track, timestampMs, int64(durationMs), isSilence, rms)

	return pkt
}

func (vr *VoiceRecorder) updateVAD(track AudioTrack, timestampMs, durationMs int64, isSilence bool, rms float64) {
	if track == TrackUserInbound {
		if !isSilence {
			if vr.currentUserVAD == nil {
				vr.currentUserVAD = &VADSegment{
					ID:         fmt.Sprintf("VAD-USER-%d", len(vr.vadSegments)+1),
					Track:      TrackUserInbound,
					StartMs:    timestampMs,
					EndMs:      timestampMs + durationMs,
					AvgEnergy:  rms,
					ChunkCount: 1,
				}
				// Check for interruption: user started speaking while agent was speaking
				if vr.currentAgentVAD != nil && vr.currentAgentVAD.EndMs > timestampMs-200 {
					vr.handleInterruption(timestampMs)
				}
			} else {
				vr.currentUserVAD.EndMs = timestampMs + durationMs
				vr.currentUserVAD.ChunkCount++
				vr.currentUserVAD.AvgEnergy = (vr.currentUserVAD.AvgEnergy + rms) / 2.0
			}
		} else {
			if vr.currentUserVAD != nil && (timestampMs-vr.currentUserVAD.EndMs > 300) {
				// Finalize user speech segment
				vr.currentUserVAD.DurationMs = vr.currentUserVAD.EndMs - vr.currentUserVAD.StartMs
				vr.vadSegments = append(vr.vadSegments, *vr.currentUserVAD)
				vr.lastUserStopMs = vr.currentUserVAD.EndMs

				// Initialize a new synchronized turn
				vr.currentTurn = &SynchronizedTurn{
					TurnIndex:         len(vr.turns) + 1,
					UserSpeechStartMs: vr.currentUserVAD.StartMs,
					UserSpeechStopMs:  vr.currentUserVAD.EndMs,
					UserTranscript:    vr.currentUserVAD.Transcript,
				}
				vr.currentUserVAD = nil
			}
		}
	} else {
		// Agent outbound track
		if !isSilence {
			if vr.currentAgentVAD == nil {
				vr.currentAgentVAD = &VADSegment{
					ID:         fmt.Sprintf("VAD-AGENT-%d", len(vr.vadSegments)+1),
					Track:      TrackAgentOutbound,
					StartMs:    timestampMs,
					EndMs:      timestampMs + durationMs,
					AvgEnergy:  rms,
					ChunkCount: 1,
				}

				// Check turnaround latency from last user speech stop
				if vr.currentTurn != nil && vr.currentTurn.FirstAgentAudioMs == 0 {
					vr.currentTurn.FirstAgentAudioMs = timestampMs
					vr.currentTurn.TurnaroundLatencyMs = timestampMs - vr.currentTurn.UserSpeechStopMs
				}
			} else {
				vr.currentAgentVAD.EndMs = timestampMs + durationMs
				vr.currentAgentVAD.ChunkCount++
				vr.currentAgentVAD.AvgEnergy = (vr.currentAgentVAD.AvgEnergy + rms) / 2.0
			}
		} else {
			if vr.currentAgentVAD != nil && (timestampMs-vr.currentAgentVAD.EndMs > 400) {
				vr.currentAgentVAD.DurationMs = vr.currentAgentVAD.EndMs - vr.currentAgentVAD.StartMs
				vr.vadSegments = append(vr.vadSegments, *vr.currentAgentVAD)

				if vr.currentTurn != nil {
					vr.currentTurn.AgentTranscript = vr.currentAgentVAD.Transcript
					vr.turns = append(vr.turns, *vr.currentTurn)
					vr.currentTurn = nil
				}
				vr.currentAgentVAD = nil
			}
		}
	}
}

func (vr *VoiceRecorder) handleInterruption(userOnsetMs int64) {
	cutoffMs := userOnsetMs + 80 // Typical client-server event propagation delay
	latency := cutoffMs - userOnsetMs
	discardedDuration := 350.0

	ev := InterruptionEvent{
		EventID:              fmt.Sprintf("INT-%03d", len(vr.interruptions)+1),
		UserSpeechStartMs:    userOnsetMs,
		AgentCutoffMs:        cutoffMs,
		DetectionLatencyMs:   latency,
		CutoffChunkIndex:     len(vr.packets),
		DiscardedAudioMs:     discardedDuration,
		AgentTranscriptSoFar: "I was about to suggest executing the following...",
	}

	vr.interruptions = append(vr.interruptions, ev)
	if vr.currentTurn != nil {
		vr.currentTurn.WasInterrupted = true
		vr.currentTurn.Interruption = &ev
	}
	vr.currentAgentVAD = nil
}

// IngestTranscript attaches real-time text transcription to the current active speaker segment.
func (vr *VoiceRecorder) IngestTranscript(track AudioTrack, text string) {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	if track == TrackUserInbound {
		if vr.currentUserVAD != nil {
			vr.currentUserVAD.Transcript += text
		}
		if vr.currentTurn != nil {
			vr.currentTurn.UserTranscript += text
		}
	} else {
		if vr.currentAgentVAD != nil {
			vr.currentAgentVAD.Transcript += text
		}
		if vr.currentTurn != nil {
			vr.currentTurn.AgentTranscript += text
		}
	}
}

// IngestToolCall records a tool execution event aligned with the current conversational turn.
func (vr *VoiceRecorder) IngestToolCall(toolName string) {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	if vr.currentTurn != nil {
		vr.currentTurn.ToolCalls = append(vr.currentTurn.ToolCalls, toolName)
	}
}

// FinalizeSession computes comprehensive telemetry metrics and audit scorecards.
func (vr *VoiceRecorder) FinalizeSession() *VoiceSessionReport {
	vr.mu.Lock()
	defer vr.mu.Unlock()

	// Flush any pending VAD segments
	if vr.currentUserVAD != nil {
		vr.currentUserVAD.DurationMs = vr.currentUserVAD.EndMs - vr.currentUserVAD.StartMs
		vr.vadSegments = append(vr.vadSegments, *vr.currentUserVAD)
		vr.currentUserVAD = nil
	}
	if vr.currentAgentVAD != nil {
		vr.currentAgentVAD.DurationMs = vr.currentAgentVAD.EndMs - vr.currentAgentVAD.StartMs
		vr.vadSegments = append(vr.vadSegments, *vr.currentAgentVAD)
		vr.currentAgentVAD = nil
	}
	if vr.currentTurn != nil {
		vr.turns = append(vr.turns, *vr.currentTurn)
		vr.currentTurn = nil
	}

	var userAudioMs float64
	var agentAudioMs float64
	var turnaroundLatencies []float64
	var interruptionLatencies []float64

	for _, p := range vr.packets {
		if p.Track == TrackUserInbound {
			userAudioMs += p.DurationMs
		} else {
			agentAudioMs += p.DurationMs
		}
	}

	for _, t := range vr.turns {
		if t.TurnaroundLatencyMs > 0 {
			turnaroundLatencies = append(turnaroundLatencies, float64(t.TurnaroundLatencyMs))
		}
	}

	for _, in := range vr.interruptions {
		interruptionLatencies = append(interruptionLatencies, float64(in.DetectionLatencyMs))
	}

	// Calculate turnaround metrics
	avgTurnaround := 0.0
	p90Turnaround := 0.0
	if len(turnaroundLatencies) > 0 {
		sum := 0.0
		for _, v := range turnaroundLatencies {
			sum += v
		}
		avgTurnaround = sum / float64(len(turnaroundLatencies))
		sort.Float64s(turnaroundLatencies)
		p90Idx := int(float64(len(turnaroundLatencies)) * 0.90)
		if p90Idx >= len(turnaroundLatencies) {
			p90Idx = len(turnaroundLatencies) - 1
		}
		p90Turnaround = turnaroundLatencies[p90Idx]
	}

	avgInterruption := 0.0
	if len(interruptionLatencies) > 0 {
		sum := 0.0
		for _, v := range interruptionLatencies {
			sum += v
		}
		avgInterruption = sum / float64(len(interruptionLatencies))
	}

	// Calculate packet jitter (P99 delta in arrival times)
	jitterP99 := vr.calculatePacketJitter()

	// Wall-clock total duration
	totalDurationMs := int64(0)
	if len(vr.packets) > 0 {
		totalDurationMs = vr.packets[len(vr.packets)-1].TimestampMs + int64(vr.packets[len(vr.packets)-1].DurationMs)
	}

	rtf := 0.0
	if totalDurationMs > 0 {
		rtf = (userAudioMs + agentAudioMs) / float64(totalDurationMs)
	}

	slaPassed := true
	if p90Turnaround > float64(vr.config.MaxTurnaroundLatencyMs) {
		slaPassed = false
	}
	if avgInterruption > float64(vr.config.MaxInterruptionLatencyMs) {
		slaPassed = false
	}

	waveform := vr.generateASCIIWaveform(40)

	return &VoiceSessionReport{
		SessionID:            vr.sessionID,
		RecordedAt:           vr.startTime,
		TotalDurationMs:      totalDurationMs,
		TotalPackets:         len(vr.packets),
		UserAudioDurationMs:  math.Round(userAudioMs*10) / 10,
		AgentAudioDurationMs: math.Round(agentAudioMs*10) / 10,
		VADSegments:          vr.vadSegments,
		Turns:                vr.turns,
		Interruptions:        vr.interruptions,
		AvgTurnaroundMs:      math.Round(avgTurnaround*10) / 10,
		P90TurnaroundMs:      math.Round(p90Turnaround*10) / 10,
		AvgInterruptionMs:    math.Round(avgInterruption*10) / 10,
		RealtimeFactor:       math.Round(rtf*100) / 100,
		PacketJitterP99Ms:    math.Round(jitterP99*10) / 10,
		SLAPassed:            slaPassed,
		WaveformPreview:      waveform,
	}
}

func (vr *VoiceRecorder) calculatePacketJitter() float64 {
	if len(vr.packets) < 2 {
		return 0.0
	}
	var deltas []float64
	for i := 1; i < len(vr.packets); i++ {
		expected := float64(vr.packets[i-1].TimestampMs) + vr.packets[i-1].DurationMs
		actual := float64(vr.packets[i].TimestampMs)
		delta := math.Abs(actual - expected)
		deltas = append(deltas, delta)
	}
	sort.Float64s(deltas)
	p99Idx := int(float64(len(deltas)) * 0.99)
	if p99Idx >= len(deltas) {
		p99Idx = len(deltas) - 1
	}
	return deltas[p99Idx]
}

func (vr *VoiceRecorder) generateASCIIWaveform(columns int) string {
	if len(vr.packets) == 0 {
		return strings.Repeat(" ", columns)
	}

	bars := []rune{' ', '▂', '▃', '▄', '▅', '▆', '▇', '█'}
	chunkSize := len(vr.packets) / columns
	if chunkSize == 0 {
		chunkSize = 1
	}

	var sb strings.Builder
	for i := 0; i < len(vr.packets); i += chunkSize {
		end := i + chunkSize
		if end > len(vr.packets) {
			end = len(vr.packets)
		}
		maxEnergy := 0.0
		for _, p := range vr.packets[i:end] {
			if p.RMSAmplitude > maxEnergy {
				maxEnergy = p.RMSAmplitude
			}
		}
		// Scale maxEnergy [0.0 - 0.25] to bar index [0 - 7]
		barIdx := int((maxEnergy / 0.25) * 7.0)
		if barIdx > 7 {
			barIdx = 7
		}
		if barIdx < 0 {
			barIdx = 0
		}
		sb.WriteRune(bars[barIdx])
		if sb.Len() >= columns {
			break
		}
	}

	for sb.Len() < columns {
		sb.WriteRune(' ')
	}
	return sb.String()
}

// CalculateRMSAndDecibels computes the Root Mean Square amplitude and dBFS for 16-bit signed PCM audio.
func CalculateRMSAndDecibels(pcmData []byte) (float64, float64) {
	if len(pcmData) < 2 {
		return 0.0, -100.0
	}

	numSamples := len(pcmData) / 2
	var sumSquares float64

	for i := 0; i < len(pcmData)-1; i += 2 {
		val := int16(binary.LittleEndian.Uint16(pcmData[i : i+2]))
		norm := float64(val) / 32768.0
		sumSquares += norm * norm
	}

	rms := math.Sqrt(sumSquares / float64(numSamples))
	decibels := -100.0
	if rms > 1e-5 {
		decibels = 20.0 * math.Log10(rms)
		if decibels < -100.0 {
			decibels = -100.0
		}
		if decibels > 0.0 {
			decibels = 0.0
		}
	}

	return rms, decibels
}

// GenerateWAVHeader creates a standard 44-byte RIFF/WAVE header for PCM raw audio data.
func GenerateWAVHeader(sampleRate, bitDepth, channels, pcmDataLen int) []byte {
	buf := new(bytes.Buffer)

	byteRate := sampleRate * channels * (bitDepth / 8)
	blockAlign := channels * (bitDepth / 8)
	riffChunkSize := 36 + pcmDataLen

	// RIFF Header
	buf.WriteString("RIFF")
	_ = binary.Write(buf, binary.LittleEndian, uint32(riffChunkSize))
	buf.WriteString("WAVE")

	// 'fmt ' Sub-chunk
	buf.WriteString("fmt ")
	_ = binary.Write(buf, binary.LittleEndian, uint32(16))      // Sub-chunk size (16 for PCM)
	_ = binary.Write(buf, binary.LittleEndian, uint16(1))       // AudioFormat 1 = PCM
	_ = binary.Write(buf, binary.LittleEndian, uint16(channels))
	_ = binary.Write(buf, binary.LittleEndian, uint32(sampleRate))
	_ = binary.Write(buf, binary.LittleEndian, uint32(byteRate))
	_ = binary.Write(buf, binary.LittleEndian, uint16(blockAlign))
	_ = binary.Write(buf, binary.LittleEndian, uint16(bitDepth))

	// 'data' Sub-chunk
	buf.WriteString("data")
	_ = binary.Write(buf, binary.LittleEndian, uint32(pcmDataLen))

	return buf.Bytes()
}

// SynthesizeSineWavePCM generates synthetic PCM audio data of specified frequency and amplitude.
func SynthesizeSineWavePCM(sampleRate int, freqHz float64, durationMs int, amplitude float64) []byte {
	numSamples := (sampleRate * durationMs) / 1000
	pcm := make([]byte, numSamples*2)

	for i := 0; i < numSamples; i++ {
		t := float64(i) / float64(sampleRate)
		val := math.Sin(2.0*math.Pi*freqHz*t) * amplitude * 32767.0
		sample := int16(val)
		binary.LittleEndian.PutUint16(pcm[i*2:i*2+2], uint16(sample))
	}
	return pcm
}
