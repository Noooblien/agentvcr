package voice

import (
	"fmt"
	"strings"
)

// RenderVoiceScorecard produces a rich ANSI terminal representation of voice flight recording metrics.
func RenderVoiceScorecard(rep *VoiceSessionReport) string {
	var sb strings.Builder

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString("\033[1;36m       🎙️ [AgentVCR] REAL-TIME VOICE API FLIGHT RECORDER & ACOUSTIC TIMELINE            \033[0m\n")
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")

	sb.WriteString(fmt.Sprintf(" \033[1mSession ID:\033[0m          %s\n", rep.SessionID))
	sb.WriteString(fmt.Sprintf(" \033[1mTotal Duration:\033[0m      %.2f s | \033[1mPackets:\033[0m %d | \033[1mRealtime Factor:\033[0m %.2fx\n\n",
		float64(rep.TotalDurationMs)/1000.0, rep.TotalPackets, rep.RealtimeFactor))

	// Waveform Energy Bar
	sb.WriteString("\033[1;33m--- Acoustic Energy Waveform (RMS Decibel Profile) ---\033[0m\n")
	sb.WriteString(fmt.Sprintf(" \033[90mUser/Agent:\033[0m  \033[1;32m[%s]\033[0m\n", rep.WaveformPreview))
	sb.WriteString(fmt.Sprintf("             \033[90mUser Audio: %.1fs | Agent Audio: %.1fs | P99 Jitter: %.1fms\033[0m\n\n",
		rep.UserAudioDurationMs/1000.0, rep.AgentAudioDurationMs/1000.0, rep.PacketJitterP99Ms))

	// Latency & SLA Metrics Table
	sb.WriteString("\033[1;33m--- Conversational Turnaround & Interruption Metrics ---\033[0m\n")
	turnaroundColor := "\033[32m"
	if rep.P90TurnaroundMs > 800 {
		turnaroundColor = "\033[31m"
	}
	sb.WriteString(fmt.Sprintf(" • Turnaround Latency (Avg):      %s%.1f ms\033[0m (User speech stop -> First agent audio)\n",
		turnaroundColor, rep.AvgTurnaroundMs))
	sb.WriteString(fmt.Sprintf(" • Turnaround Latency (P90):      %s%.1f ms\033[0m (SLA Threshold < 800 ms)\n",
		turnaroundColor, rep.P90TurnaroundMs))

	interruptionColor := "\033[32m"
	if rep.AvgInterruptionMs > 350 {
		interruptionColor = "\033[31m"
	}
	sb.WriteString(fmt.Sprintf(" • Interruption Cutoff (Avg):     %s%.1f ms\033[0m (Barge-in onset -> Agent audio cutoff)\n",
		interruptionColor, rep.AvgInterruptionMs))
	sb.WriteString(fmt.Sprintf(" • Total Barge-in Interruptions:  \033[1;35m%d\033[0m detected\n\n", len(rep.Interruptions)))

	// Synchronized Conversation Turns
	if len(rep.Turns) > 0 {
		sb.WriteString("\033[1;33m--- Synchronized Multi-Track Conversation Timeline ---\033[0m\n")
		for _, turn := range rep.Turns {
			interruptedTag := ""
			if turn.WasInterrupted {
				interruptedTag = " \033[1;31m[INTERRUPTED]\033[0m"
			}
			sb.WriteString(fmt.Sprintf(" \033[1mTurn #%d\033[0m (Turnaround: \033[32m%d ms\033[0m)%s:\n",
				turn.TurnIndex, turn.TurnaroundLatencyMs, interruptedTag))

			if turn.UserTranscript != "" {
				sb.WriteString(fmt.Sprintf("   \033[34mUser:\033[0m  \"%s\" \033[90m(from %dms to %dms)\033[0m\n",
					turn.UserTranscript, turn.UserSpeechStartMs, turn.UserSpeechStopMs))
			}
			if len(turn.ToolCalls) > 0 {
				sb.WriteString(fmt.Sprintf("   \033[33mTools:\033[0m %s\n", strings.Join(turn.ToolCalls, ", ")))
			}
			if turn.AgentTranscript != "" {
				sb.WriteString(fmt.Sprintf("   \033[32mAgent:\033[0m \"%s\"\n", turn.AgentTranscript))
			}
		}
		sb.WriteString("\n")
	}

	// Interruption Events Details
	if len(rep.Interruptions) > 0 {
		sb.WriteString("\033[1;31m--- Interruption & Barge-in Forensics ---\033[0m\n")
		for i, in := range rep.Interruptions {
			sb.WriteString(fmt.Sprintf(" • \033[1mEvent #%d (%s)\033[0m: User spoke at \033[1m%d ms\033[0m -> Agent silenced at \033[1m%d ms\033[0m\n",
				i+1, in.EventID, in.UserSpeechStartMs, in.AgentCutoffMs))
			sb.WriteString(fmt.Sprintf("   \033[90mDetection Latency: %d ms | Discarded Synthetic Audio: %.0f ms\033[0m\n",
				in.DetectionLatencyMs, in.DiscardedAudioMs))
		}
		sb.WriteString("\n")
	}

	// SLA Verdict Badge
	sb.WriteString("\033[1;33m--- Real-Time Voice SLA Verdict ---\033[0m\n")
	if rep.SLAPassed {
		sb.WriteString(" \033[1;42;30m PASS \033[0m \033[1;32mTurnaround and interruption latencies conform to Realtime Voice SLA standards.\033[0m\n\n")
	} else {
		sb.WriteString(" \033[1;41;37m SLA BREACH \033[0m \033[1;31mTurnaround (>800ms) or interruption cutoff (>350ms) exceeded latency ceilings.\033[0m\n\n")
	}

	return sb.String()
}
