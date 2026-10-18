package diff

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// GenerateDemoSessions creates realistic baseline and candidate agent trajectories
// demonstrating semantic matching, parameter drift, and regression detection.
func GenerateDemoSessions(regressed bool) (*trace.Session, *trace.Session) {
	now := time.Now()

	base := &trace.Session{
		ID:        "trace_pr_fixer_baseline",
		AgentName: "code-repair-agent-v1",
		CreatedAt: now.Add(-10 * time.Minute),
		Status:    "completed",
		Events: []trace.TraceEvent{
			{
				Index:      0,
				Type:       trace.EventLLMRequest,
				DurationMs: 120,
				Payload: map[string]interface{}{
					"prompt": "Fix nil pointer dereference in auth handler when token is expired",
				},
			},
			{
				Index:      1,
				Type:       trace.EventLLMResponse,
				DurationMs: 840,
				Payload: map[string]interface{}{
					"content": "I will inspect auth/handler.go and add a nil guard before token validation.",
					"body": map[string]interface{}{
						"usage": map[string]interface{}{
							"prompt_tokens":     float64(350),
							"completion_tokens": float64(120),
							"total_tokens":      float64(470),
						},
					},
				},
			},
			{
				Index:      2,
				Type:       trace.EventToolCall,
				DurationMs: 15,
				Payload: map[string]interface{}{
					"name": "read_file",
					"path": "auth/handler.go",
				},
			},
			{
				Index:      3,
				Type:       trace.EventToolResult,
				DurationMs: 5,
				Payload: map[string]interface{}{
					"output": "func Authenticate(r *Request) error { return r.Token.Validate() }",
				},
			},
			{
				Index:      4,
				Type:       trace.EventToolCall,
				DurationMs: 25,
				Payload: map[string]interface{}{
					"name": "write_file",
					"path": "auth/handler.go",
					"diff": "if r.Token == nil { return ErrMissingToken }",
				},
			},
			{
				Index:      5,
				Type:       trace.EventToolResult,
				DurationMs: 5,
				Payload: map[string]interface{}{
					"output": "File auth/handler.go successfully updated.",
				},
			},
			{
				Index:      6,
				Type:       trace.EventToolCall,
				DurationMs: 350,
				Payload: map[string]interface{}{
					"name": "run_command",
					"cmd":  "go test ./auth/...",
				},
			},
			{
				Index:      7,
				Type:       trace.EventToolResult,
				DurationMs: 10,
				Payload: map[string]interface{}{
					"output": "PASS: TestAuthenticate (0.04s)\nok github.com/repo/auth",
					"status": "success",
				},
			},
		},
	}

	target := &trace.Session{
		ID:        "trace_pr_fixer_candidate",
		AgentName: "code-repair-agent-v2-experimental",
		CreatedAt: now,
		Status:    "completed",
		Events: []trace.TraceEvent{
			{
				Index:      0,
				Type:       trace.EventLLMRequest,
				DurationMs: 110,
				Payload: map[string]interface{}{
					"prompt": "Fix nil pointer dereference in auth handler when token is expired",
				},
			},
			{
				Index:      1,
				Type:       trace.EventLLMResponse,
				DurationMs: 780,
				Payload: map[string]interface{}{
					"content": "Analyzing auth/handler.go to implement safety check and run race detector.",
					"body": map[string]interface{}{
						"usage": map[string]interface{}{
							"prompt_tokens":     float64(340),
							"completion_tokens": float64(115),
							"total_tokens":      float64(455),
						},
					},
				},
			},
			{
				Index:      2,
				Type:       trace.EventToolCall,
				DurationMs: 14,
				Payload: map[string]interface{}{
					"name": "read_file",
					"path": "auth/handler.go",
				},
			},
			{
				Index:      3,
				Type:       trace.EventToolResult,
				DurationMs: 6,
				Payload: map[string]interface{}{
					"output": "func Authenticate(r *Request) error { return r.Token.Validate() }",
				},
			},
			{
				Index:      4,
				Type:       trace.EventToolCall,
				DurationMs: 28,
				Payload: map[string]interface{}{
					"name": "write_file",
					"path": "auth/handler.go",
					"diff": "if r == nil || r.Token == nil { return ErrMissingToken }",
				},
			},
			{
				Index:      5,
				Type:       trace.EventToolResult,
				DurationMs: 4,
				Payload: map[string]interface{}{
					"output": "File auth/handler.go successfully updated.",
				},
			},
		},
	}

	if regressed {
		target.Events = append(target.Events,
			trace.TraceEvent{
				Index:      6,
				Type:       trace.EventToolCall,
				DurationMs: 450,
				Payload: map[string]interface{}{
					"name": "run_command",
					"cmd":  "go test -race ./auth/...",
				},
			},
			trace.TraceEvent{
				Index:      7,
				Type:       trace.EventError,
				DurationMs: 12,
				Payload: map[string]interface{}{
					"error":  "panic: runtime error: index out of range [0] with length 0 in auth_test.go:42",
					"status": "failed",
				},
			},
		)
	} else {
		target.Events = append(target.Events,
			trace.TraceEvent{
				Index:      6,
				Type:       trace.EventToolCall,
				DurationMs: 380,
				Payload: map[string]interface{}{
					"name": "run_command",
					"cmd":  "go test -race ./auth/...",
				},
			},
			trace.TraceEvent{
				Index:      7,
				Type:       trace.EventToolResult,
				DurationMs: 8,
				Payload: map[string]interface{}{
					"output": "PASS: TestAuthenticate (0.05s)\nok github.com/repo/auth (race detector: clean)",
					"status": "success",
				},
			},
		)
	}

	return base, target
}
