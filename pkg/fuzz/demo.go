package fuzz

import (
	"time"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// RunFuzzDemo executes a 4-attack-vector adversarial fuzzing campaign on a representative flight trace.
func RunFuzzDemo() *FuzzCampaignReport {
	cfg := DefaultFuzzEngineConfig()
	engine := NewAdversarialEngine(cfg)

	// Representative multi-step agent session
	now := time.Now()
	session := &trace.Session{
		ID:        "trace_agent_autonomous_1760800000",
		AgentName: "Enterprise-SRE-Agent",
		Status:    "completed",
		Events: []trace.TraceEvent{
			{
				Index:      1,
				Timestamp:  now,
				Type:       trace.EventLLMRequest,
				DurationMs: 25,
				Payload: map[string]interface{}{
					"model":  "claude-3-7-sonnet",
					"prompt": "Inspect Kubernetes cluster pods for CrashLoopBackOff and diagnose root cause.",
				},
			},
			{
				Index:      2,
				Timestamp:  now.Add(500 * time.Millisecond),
				Type:       trace.EventLLMResponse,
				DurationMs: 420,
				Payload: map[string]interface{}{
					"content": "Executing kubectl get pods --all-namespaces via MCP tool.",
				},
			},
			{
				Index:      3,
				Timestamp:  now.Add(1 * time.Second),
				Type:       trace.EventToolCall,
				DurationMs: 65,
				Payload: map[string]interface{}{
					"tool_name": "kubernetes_get_pods",
					"params":    map[string]interface{}{"namespace": "production"},
				},
			},
			{
				Index:      4,
				Timestamp:  now.Add(2 * time.Second),
				Type:       trace.EventToolResult,
				DurationMs: 110,
				Payload: map[string]interface{}{
					"status": "success",
					"output": "pod/payment-service-789bf-x7z2k Running 4/4\npod/auth-service-542cf-99wq1 CrashLoopBackOff 0/1",
				},
			},
		},
	}

	return engine.RunCampaign(session)
}
