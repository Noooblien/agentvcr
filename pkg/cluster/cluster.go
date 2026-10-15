package cluster

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Noooblien/agentvcr/pkg/metrics"
	"github.com/Noooblien/agentvcr/pkg/trace"
)

// AgentHeatmap captures aggregate execution statistics for a given agent profile.
type AgentHeatmap struct {
	AgentName        string             `json:"agent_name"`
	TotalSessions    int                `json:"total_sessions"`
	TotalEvents      int                `json:"total_events"`
	TotalTokens      int                `json:"total_tokens"`
	TotalCostUSD     float64            `json:"total_cost_usd"`
	AvgDurationMs    float64            `json:"avg_duration_ms"`
	ToolFrequency    map[string]int     `json:"tool_frequency"`
	ModelBreakdown   map[string]int     `json:"model_breakdown"`
	ErrorCount       int                `json:"error_count"`
	FailureRate      float64            `json:"failure_rate"`
}

// ClusterGroup bundles sessions that share identical tool execution patterns.
type ClusterGroup struct {
	Signature    string   `json:"signature"`     // Sequence of tool dispatches e.g. "search->eval->commit"
	SessionIDs   []string `json:"session_ids"`
	Count        int      `json:"count"`
	AvgTokens    int      `json:"avg_tokens"`
	AvgCostUSD   float64  `json:"avg_cost_usd"`
}

// MultiAgentReport summarizes fleet-wide agent telemetry.
type MultiAgentReport struct {
	TotalFleetSessions int                     `json:"total_fleet_sessions"`
	AgentHeatmaps      map[string]*AgentHeatmap `json:"agent_heatmaps"`
	Clusters           []ClusterGroup          `json:"clusters"`
}

// GenerateFleetReport aggregates across a slice of trace sessions.
func GenerateFleetReport(sessions []*trace.Session) *MultiAgentReport {
	report := &MultiAgentReport{
		TotalFleetSessions: len(sessions),
		AgentHeatmaps:      make(map[string]*AgentHeatmap),
	}

	clusterMap := make(map[string]*struct {
		sessionIDs []string
		tokens     int
		cost       float64
	})

	for _, session := range sessions {
		if session == nil {
			continue
		}

		agent := session.AgentName
		if agent == "" {
			agent = "anonymous-agent"
		}

		heatmap, exists := report.AgentHeatmaps[agent]
		if !exists {
			heatmap = &AgentHeatmap{
				AgentName:      agent,
				ToolFrequency:  make(map[string]int),
				ModelBreakdown: make(map[string]int),
			}
			report.AgentHeatmaps[agent] = heatmap
		}

		sessionMetrics := metrics.ComputeSessionMetrics(session)
		heatmap.TotalSessions++
		heatmap.TotalEvents += sessionMetrics.TotalEvents
		heatmap.TotalTokens += sessionMetrics.TotalTokens
		heatmap.TotalCostUSD += sessionMetrics.EstimatedCostUSD
		heatmap.AvgDurationMs += float64(sessionMetrics.TotalDurationMs)

		var toolSequence []string
		hasError := false

		for _, ev := range session.Events {
			if ev.Type == trace.EventError {
				hasError = true
			}
			if ev.Type == trace.EventToolCall {
				toolName := extractToolName(ev.Payload)
				if toolName != "" {
					heatmap.ToolFrequency[toolName]++
					toolSequence = append(toolSequence, toolName)
				}
			}
			if ev.Type == trace.EventLLMRequest {
				if model, ok := ev.Payload["model"].(string); ok && model != "" {
					heatmap.ModelBreakdown[model]++
				}
			}
		}

		if hasError || session.Status == "failed" {
			heatmap.ErrorCount++
		}

		// Group into execution clusters by tool sequence signature
		sig := strings.Join(toolSequence, " ➔ ")
		if sig == "" {
			sig = "(pure LLM monologue)"
		}

		group, ok := clusterMap[sig]
		if !ok {
			group = &struct {
				sessionIDs []string
				tokens     int
				cost       float64
			}{
				sessionIDs: make([]string, 0),
			}
			clusterMap[sig] = group
		}
		group.sessionIDs = append(group.sessionIDs, session.ID)
		group.tokens += sessionMetrics.TotalTokens
		group.cost += sessionMetrics.EstimatedCostUSD
	}

	// Finalize averages per agent
	for _, heatmap := range report.AgentHeatmaps {
		if heatmap.TotalSessions > 0 {
			heatmap.AvgDurationMs /= float64(heatmap.TotalSessions)
			heatmap.FailureRate = (float64(heatmap.ErrorCount) / float64(heatmap.TotalSessions)) * 100.0
		}
	}

	// Finalize sorted cluster list
	for sig, g := range clusterMap {
		count := len(g.sessionIDs)
		report.Clusters = append(report.Clusters, ClusterGroup{
			Signature:  sig,
			SessionIDs: g.sessionIDs,
			Count:      count,
			AvgTokens:  g.tokens / count,
			AvgCostUSD: g.cost / float64(count),
		})
	}

	sort.Slice(report.Clusters, func(i, j int) bool {
		return report.Clusters[i].Count > report.Clusters[j].Count
	})

	return report
}

// RenderASCIIHeatmap formats aggregate heatmaps into a CLI terminal visualization.
func RenderASCIIHeatmap(report *MultiAgentReport) string {
	var b strings.Builder
	b.WriteString("\n🌐 [AgentVCR] Multi-Agent Fleet Analytics & Heatmap\n")
	b.WriteString(fmt.Sprintf("   Total Sessions Analyzed: %d | Unique Agents: %d\n\n",
		report.TotalFleetSessions, len(report.AgentHeatmaps)))

	for name, h := range report.AgentHeatmaps {
		b.WriteString(fmt.Sprintf("🤖 Agent: %s\n", name))
		b.WriteString(fmt.Sprintf("   Sessions: %d | Tokens: %d | Total Cost: $%.4f USD | Avg Latency: %.1f ms | Fail Rate: %.1f%%\n",
			h.TotalSessions, h.TotalTokens, h.TotalCostUSD, h.AvgDurationMs, h.FailureRate))
		
		b.WriteString("   Top Dispatched Tools:\n")
		for tool, freq := range h.ToolFrequency {
			bar := strings.Repeat("█", min(freq, 20))
			b.WriteString(fmt.Sprintf("     %-20s %3d %s\n", tool, freq, bar))
		}
		b.WriteString("\n")
	}

	b.WriteString("🧬 Trajectory Clusters & Execution Signatures:\n")
	for i, c := range report.Clusters {
		b.WriteString(fmt.Sprintf("   [%d] (%d runs, avg %d tokens, $%.4f): %s\n",
			i+1, c.Count, c.AvgTokens, c.AvgCostUSD, c.Signature))
	}

	return b.String()
}

func extractToolName(payload map[string]interface{}) string {
	if name, ok := payload["tool_name"].(string); ok && name != "" {
		return name
	}
	if name, ok := payload["tool"].(string); ok && name != "" {
		return name
	}
	if method, ok := payload["method"].(string); ok && method != "" {
		return method
	}
	return ""
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
