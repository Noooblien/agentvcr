package distill

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Engine handles trajectory distillation, SFT extraction, and DPO alignment pairing.
type Engine struct {
	mu     sync.RWMutex
	Config DistillConfig
}

// NewEngine creates an initialized Trajectory Distillation Engine.
func NewEngine(cfg DistillConfig) *Engine {
	if cfg.Format == "" {
		cfg.Format = FormatDPO
	}
	if cfg.MinRewardMargin <= 0 {
		cfg.MinRewardMargin = 0.20
	}
	if cfg.MaxTurnsPerSession <= 0 {
		cfg.MaxTurnsPerSession = 32
	}
	return &Engine{
		Config: cfg,
	}
}

// ExtractSFT extracts clean Supervised Fine-Tuning messages from a recorded trace session.
func (e *Engine) ExtractSFT(session *trace.Session) (*SFTRow, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if session == nil || len(session.Events) == 0 {
		return nil, fmt.Errorf("session contains no trace events")
	}

	messages := make([]ChatMessage, 0, len(session.Events))

	if e.Config.SystemPromptOverride != "" {
		messages = append(messages, ChatMessage{
			Role:    "system",
			Content: e.Config.SystemPromptOverride,
		})
	}

	for _, evt := range session.Events {
		if len(messages) >= e.Config.MaxTurnsPerSession {
			break
		}

		switch evt.Type {
		case trace.EventLLMRequest:
			prompt := extractPayloadString(evt.Payload, "prompt")
			if prompt == "" {
				prompt = extractPayloadString(evt.Payload, "content")
			}
			if prompt != "" {
				role := "user"
				if evt.Index == 0 && strings.Contains(strings.ToLower(prompt), "system:") {
					role = "system"
				}
				messages = append(messages, ChatMessage{
					Role:    role,
					Content: strings.TrimSpace(prompt),
				})
			}

		case trace.EventLLMResponse:
			content := extractPayloadString(evt.Payload, "content")
			if content == "" {
				content = extractPayloadString(evt.Payload, "text")
			}
			if content != "" {
				messages = append(messages, ChatMessage{
					Role:    "assistant",
					Content: strings.TrimSpace(content),
				})
			}

		case trace.EventToolCall:
			toolName := extractPayloadString(evt.Payload, "tool")
			if toolName == "" {
				toolName = extractPayloadString(evt.Payload, "tool_name")
			}
			argsStr := ""
			if args, ok := evt.Payload["args"]; ok {
				b, _ := json.Marshal(args)
				argsStr = string(b)
			} else if cmd := extractPayloadString(evt.Payload, "command"); cmd != "" {
				argsStr = cmd
			}

			messages = append(messages, ChatMessage{
				Role:    "assistant",
				Content: fmt.Sprintf("Action: %s\nArguments: %s", toolName, argsStr),
				Name:    toolName,
				Payload: evt.Payload,
			})

		case trace.EventToolResult:
			toolName := extractPayloadString(evt.Payload, "tool")
			output := extractPayloadString(evt.Payload, "output")
			if output == "" {
				output = extractPayloadString(evt.Payload, "result")
			}

			if e.Config.StripToolNoise && len(output) > 800 {
				output = output[:800] + "... [truncated for distillation economy]"
			}

			messages = append(messages, ChatMessage{
				Role:    "tool",
				Content: strings.TrimSpace(output),
				Name:    toolName,
			})

		case trace.EventError:
			if !e.Config.FilterErrors {
				errMsg := extractPayloadString(evt.Payload, "error")
				messages = append(messages, ChatMessage{
					Role:    "tool",
					Content: fmt.Sprintf("Error: %s", errMsg),
				})
			}
		}
	}

	return &SFTRow{Messages: messages}, nil
}

// PairDPO pairs a chosen (successful/optimal) session with a rejected (suboptimal/failed) session.
func (e *Engine) PairDPO(chosenSession, rejectedSession *trace.Session) (*DPORow, error) {
	e.mu.RLock()
	defer e.mu.RUnlock()

	if chosenSession == nil || rejectedSession == nil {
		return nil, fmt.Errorf("both chosen and rejected sessions are required for DPO pairing")
	}

	chosenSFT, err := e.ExtractSFT(chosenSession)
	if err != nil {
		return nil, fmt.Errorf("failed to extract chosen SFT: %w", err)
	}
	rejectedSFT, err := e.ExtractSFT(rejectedSession)
	if err != nil {
		return nil, fmt.Errorf("failed to extract rejected SFT: %w", err)
	}

	// 1. Extract Common Initial User Prompt
	prompt := "Autonomous Engineering Task"
	for _, m := range chosenSFT.Messages {
		if m.Role == "user" {
			prompt = m.Content
			break
		}
	}

	// 2. Identify Point of Trajectory Divergence
	divergenceStep := 0
	minLen := len(chosenSFT.Messages)
	if len(rejectedSFT.Messages) < minLen {
		minLen = len(rejectedSFT.Messages)
	}

	for i := 0; i < minLen; i++ {
		cMsg := chosenSFT.Messages[i]
		rMsg := rejectedSFT.Messages[i]
		if cMsg.Role != rMsg.Role || cMsg.Content != rMsg.Content {
			divergenceStep = i
			break
		}
	}

	// 3. Compute Objective Reward Margin Delta
	rewardMargin := 0.50 // Base margin for verified divergence

	// Bonus for chosen completion status vs rejected failure
	if chosenSession.Status == "completed" && rejectedSession.Status != "completed" {
		rewardMargin += 0.80
	}

	// Check error counts
	chosenErrors := countErrors(chosenSession)
	rejectedErrors := countErrors(rejectedSession)
	if rejectedErrors > chosenErrors {
		rewardMargin += float64(rejectedErrors-chosenErrors) * 0.40
	}

	// Check token count / duration efficiency
	if len(chosenSession.Events) < len(rejectedSession.Events) {
		rewardMargin += 0.30
	}

	return &DPORow{
		Prompt:       prompt,
		Chosen:       chosenSFT.Messages,
		Rejected:     rejectedSFT.Messages,
		RewardMargin: rewardMargin,
		DivergenceAt: divergenceStep,
	}, nil
}

// ConvertToShareGPT converts an SFT row into standard ShareGPT conversation format.
func ConvertToShareGPT(sft *SFTRow, id string) *ShareGPTRow {
	convs := make([]ShareGPTMessage, 0, len(sft.Messages))
	for _, m := range sft.Messages {
		from := "gpt"
		switch m.Role {
		case "system":
			from = "system"
		case "user":
			from = "human"
		case "assistant":
			from = "gpt"
		case "tool":
			from = "human" // Tool observations are passed back as human turns in ShareGPT
		}
		convs = append(convs, ShareGPTMessage{
			From:  from,
			Value: m.Content,
		})
	}
	return &ShareGPTRow{
		ID:            id,
		Conversations: convs,
	}
}

// ExportJSONL serializes records into newline-delimited JSON bytes.
func ExportJSONL(records interface{}) ([]byte, error) {
	var sb strings.Builder

	switch items := records.(type) {
	case []SFTRow:
		for _, item := range items {
			b, err := json.Marshal(item)
			if err != nil {
				return nil, err
			}
			sb.Write(b)
			sb.WriteByte('\n')
		}
	case []DPORow:
		for _, item := range items {
			b, err := json.Marshal(item)
			if err != nil {
				return nil, err
			}
			sb.Write(b)
			sb.WriteByte('\n')
		}
	case []ShareGPTRow:
		for _, item := range items {
			b, err := json.Marshal(item)
			if err != nil {
				return nil, err
			}
			sb.Write(b)
			sb.WriteByte('\n')
		}
	default:
		return nil, fmt.Errorf("unsupported record type for JSONL export")
	}

	return []byte(sb.String()), nil
}

// DistillSessionPair runs full end-to-end distillation on chosen and rejected session traces.
func (e *Engine) DistillSessionPair(chosenSession, rejectedSession *trace.Session) *DistillationReport {
	e.mu.RLock()
	defer e.mu.RUnlock()

	dpoRow, err := e.PairDPO(chosenSession, rejectedSession)
	if err != nil {
		return &DistillationReport{
			ReportID:    fmt.Sprintf("distill-err-%d", time.Now().Unix()),
			GeneratedAt: time.Now().UTC(),
			Format:      e.Config.Format,
			Passed:      false,
		}
	}

	chosenSFT, _ := e.ExtractSFT(chosenSession)
	rejectedSFT, _ := e.ExtractSFT(rejectedSession)

	rawTokens := estimateSessionTokens(chosenSession) + estimateSessionTokens(rejectedSession)
	distilledTokens := estimateMessagesTokens(chosenSFT.Messages) + estimateMessagesTokens(rejectedSFT.Messages)
	prunedTurns := 0
	if len(chosenSession.Events) > len(chosenSFT.Messages) {
		prunedTurns += len(chosenSession.Events) - len(chosenSFT.Messages)
	}
	if len(rejectedSession.Events) > len(rejectedSFT.Messages) {
		prunedTurns += len(rejectedSession.Events) - len(rejectedSFT.Messages)
	}

	compressionRatio := 0.0
	if rawTokens > 0 {
		compressionRatio = float64(rawTokens-distilledTokens) / float64(rawTokens)
	}

	chosenSummary := "Optimal single-step execution with verified completion"
	if len(chosenSFT.Messages) > 1 {
		chosenSummary = fmt.Sprintf("%s -> %s", chosenSFT.Messages[0].Role, chosenSFT.Messages[len(chosenSFT.Messages)-1].Content)
		if len(chosenSummary) > 80 {
			chosenSummary = chosenSummary[:77] + "..."
		}
	}

	rejectedSummary := "Suboptimal path with redundant actions or execution error"
	if len(rejectedSFT.Messages) > 1 {
		rejectedSummary = fmt.Sprintf("%s -> %s", rejectedSFT.Messages[0].Role, rejectedSFT.Messages[len(rejectedSFT.Messages)-1].Content)
		if len(rejectedSummary) > 80 {
			rejectedSummary = rejectedSummary[:77] + "..."
		}
	}

	return &DistillationReport{
		ReportID:              fmt.Sprintf("distill-report-%s", chosenSession.ID),
		GeneratedAt:           time.Now().UTC(),
		Format:                e.Config.Format,
		TotalSessionsScanned:  2,
		SFTExamplesGenerated:  2,
		DPOPairsGenerated:     1,
		PrunedTurnsCount:      prunedTurns,
		RawTokensTotal:        rawTokens,
		DistilledTokensTotal:  distilledTokens,
		CompressionRatio:      compressionRatio,
		AverageRewardMargin:   dpoRow.RewardMargin,
		Passed:                dpoRow.RewardMargin >= e.Config.MinRewardMargin,
		DPOEntries:            []DPORow{*dpoRow},
		SFTEntries:            []SFTRow{*chosenSFT, *rejectedSFT},
		SamplePrompt:          dpoRow.Prompt,
		SampleChosenSummary:   chosenSummary,
		SampleRejectedSummary: rejectedSummary,
		TraceSession:          chosenSession,
	}
}

// Helpers

func extractPayloadString(p map[string]interface{}, key string) string {
	if p == nil {
		return ""
	}
	if val, ok := p[key]; ok {
		if s, ok := val.(string); ok {
			return s
		}
	}
	return ""
}

func countErrors(session *trace.Session) int {
	errs := 0
	for _, ev := range session.Events {
		if ev.Type == trace.EventError {
			errs++
		}
		if isErr, ok := ev.Payload["is_error"].(bool); ok && isErr {
			errs++
		}
	}
	return errs
}

func estimateSessionTokens(s *trace.Session) int {
	total := 0
	for _, ev := range s.Events {
		for _, v := range ev.Payload {
			total += len(fmt.Sprintf("%v", v)) / 4
		}
	}
	if total == 0 {
		total = len(s.Events) * 80
	}
	return total
}

func estimateMessagesTokens(msgs []ChatMessage) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content) / 4
	}
	return total
}
