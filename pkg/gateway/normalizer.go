package gateway

import (
	"encoding/json"
	"fmt"
	"time"
)

// Normalizer handles wire-level protocol serialization and translation across model providers.
type Normalizer struct{}

// NewNormalizer creates a new protocol translation engine.
func NewNormalizer() *Normalizer {
	return &Normalizer{}
}

// FormatRequest converts a unified GatewayRequest into the specific wire payload of the provider.
func (n *Normalizer) FormatRequest(req *GatewayRequest, provider *ProviderConfig, targetModel string) ([]byte, error) {
	switch provider.Protocol {
	case ProtocolAnthropic:
		return n.formatAnthropic(req, targetModel)
	case ProtocolGemini:
		return n.formatGemini(req, targetModel)
	case ProtocolOpenAI, ProtocolOllama:
		fallthrough
	default:
		return n.formatOpenAI(req, targetModel)
	}
}

func (n *Normalizer) formatOpenAI(req *GatewayRequest, targetModel string) ([]byte, error) {
	payload := map[string]interface{}{
		"model":       targetModel,
		"messages":    req.Messages,
		"temperature": req.Temperature,
	}

	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if len(req.Tools) > 0 {
		payload["tools"] = req.Tools
	}

	return json.Marshal(payload)
}

func (n *Normalizer) formatAnthropic(req *GatewayRequest, targetModel string) ([]byte, error) {
	var systemPrompt string
	anthropicMessages := make([]map[string]interface{}, 0, len(req.Messages))

	for _, msg := range req.Messages {
		if msg.Role == "system" {
			systemPrompt = msg.Content
			continue
		}
		role := msg.Role
		if role == "tool" {
			// Anthropic uses role user with tool_result content block
			anthropicMessages = append(anthropicMessages, map[string]interface{}{
				"role": "user",
				"content": []map[string]interface{}{
					{
						"type":         "tool_result",
						"tool_use_id":  msg.ToolCallID,
						"content":      msg.Content,
					},
				},
			})
			continue
		}

		anthropicMessages = append(anthropicMessages, map[string]interface{}{
			"role":    role,
			"content": msg.Content,
		})
	}

	payload := map[string]interface{}{
		"model":       targetModel,
		"messages":    anthropicMessages,
		"temperature": req.Temperature,
		"max_tokens":  4096,
	}
	if req.MaxTokens > 0 {
		payload["max_tokens"] = req.MaxTokens
	}
	if systemPrompt != "" {
		payload["system"] = systemPrompt
	}

	return json.Marshal(payload)
}

func (n *Normalizer) formatGemini(req *GatewayRequest, _ string) ([]byte, error) {
	contents := make([]map[string]interface{}, 0, len(req.Messages))

	for _, msg := range req.Messages {
		role := "user"
		if msg.Role == "assistant" {
			role = "model"
		}
		contents = append(contents, map[string]interface{}{
			"role": role,
			"parts": []map[string]interface{}{
				{"text": msg.Content},
			},
		})
	}

	payload := map[string]interface{}{
		"contents": contents,
		"generationConfig": map[string]interface{}{
			"temperature": req.Temperature,
		},
	}

	return json.Marshal(payload)
}

// ParseResponse deserializes provider-specific response JSON into a normalized GatewayResponse.
func (n *Normalizer) ParseResponse(raw []byte, provider *ProviderConfig, targetModel string) (*GatewayResponse, error) {
	resp := &GatewayResponse{
		ID:         fmt.Sprintf("gw_%d", time.Now().UnixNano()),
		Model:      targetModel,
		ProviderID: provider.ID,
		Timestamp:  time.Now(),
	}

	switch provider.Protocol {
	case ProtocolAnthropic:
		var anthropicResp struct {
			ID      string `json:"id"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			Usage struct {
				InputTokens  int `json:"input_tokens"`
				OutputTokens int `json:"output_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(raw, &anthropicResp); err != nil {
			return nil, fmt.Errorf("failed to parse anthropic response: %w", err)
		}
		text := ""
		for _, c := range anthropicResp.Content {
			if c.Type == "text" {
				text += c.Text
			}
		}
		resp.Choices = []GatewayChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: text,
				},
				FinishReason: "stop",
			},
		}
		resp.Usage = TokenUsage{
			PromptTokens:     anthropicResp.Usage.InputTokens,
			CompletionTokens: anthropicResp.Usage.OutputTokens,
			TotalTokens:      anthropicResp.Usage.InputTokens + anthropicResp.Usage.OutputTokens,
		}

	case ProtocolGemini:
		var geminiResp struct {
			Candidates []struct {
				Content struct {
					Parts []struct {
						Text string `json:"text"`
					} `json:"parts"`
				} `json:"content"`
				FinishReason string `json:"finishReason"`
			} `json:"candidates"`
			UsageMetadata struct {
				PromptTokenCount     int `json:"promptTokenCount"`
				CandidatesTokenCount int `json:"candidatesTokenCount"`
				TotalTokenCount      int `json:"totalTokenCount"`
			} `json:"usageMetadata"`
		}
		if err := json.Unmarshal(raw, &geminiResp); err != nil {
			return nil, fmt.Errorf("failed to parse gemini response: %w", err)
		}
		text := ""
		if len(geminiResp.Candidates) > 0 && len(geminiResp.Candidates[0].Content.Parts) > 0 {
			text = geminiResp.Candidates[0].Content.Parts[0].Text
		}
		resp.Choices = []GatewayChoice{
			{
				Index: 0,
				Message: ChatMessage{
					Role:    "assistant",
					Content: text,
				},
				FinishReason: "stop",
			},
		}
		resp.Usage = TokenUsage{
			PromptTokens:     geminiResp.UsageMetadata.PromptTokenCount,
			CompletionTokens: geminiResp.UsageMetadata.CandidatesTokenCount,
			TotalTokens:      geminiResp.UsageMetadata.TotalTokenCount,
		}

	case ProtocolOpenAI, ProtocolOllama:
		fallthrough
	default:
		var openAIResp struct {
			ID      string `json:"id"`
			Choices []struct {
				Index   int `json:"index"`
				Message struct {
					Role      string     `json:"role"`
					Content   string     `json:"content"`
					ToolCalls []ToolCall `json:"tool_calls,omitempty"`
				} `json:"message"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
			Usage struct {
				PromptTokens     int `json:"prompt_tokens"`
				CompletionTokens int `json:"completion_tokens"`
				TotalTokens      int `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(raw, &openAIResp); err != nil {
			return nil, fmt.Errorf("failed to parse openai response: %w", err)
		}
		for _, ch := range openAIResp.Choices {
			resp.Choices = append(resp.Choices, GatewayChoice{
				Index: ch.Index,
				Message: ChatMessage{
					Role:      ch.Message.Role,
					Content:   ch.Message.Content,
					ToolCalls: ch.Message.ToolCalls,
				},
				FinishReason: ch.FinishReason,
			})
		}
		resp.Usage = TokenUsage{
			PromptTokens:     openAIResp.Usage.PromptTokens,
			CompletionTokens: openAIResp.Usage.CompletionTokens,
			TotalTokens:      openAIResp.Usage.TotalTokens,
		}
	}

	return resp, nil
}
