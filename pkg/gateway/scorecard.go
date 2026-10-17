package gateway

import (
	"fmt"
	"strings"
)

// RenderGatewayScorecard formats an ANSI terminal report visualizing gateway resilience and routing metrics.
func RenderGatewayScorecard(report *GatewayReport) string {
	var sb strings.Builder

	sb.WriteString("\033[1;36m========================================================================================\033[0m\n")
	sb.WriteString("  \033[1;37mAGENTVCR // MODEL GATEWAY, CIRCUIT BREAKER & SEMANTIC ROUTER SCORECARD\033[0m\n")
	sb.WriteString("\033[1;36m========================================================================================\033[0m\n\n")

	sb.WriteString(fmt.Sprintf("  \033[1;37mSystem ID:\033[0m        %s\n", report.SystemID))
	sb.WriteString(fmt.Sprintf("  \033[1;37mTimestamp:\033[0m        %s\n", report.Timestamp.Format("2006-01-02 15:04:05 UTC")))

	healthBadge := "\033[1;32m[PASS] ALL UPSTREAM CIRCUITS HEALTHY & ROUTED\033[0m"
	for _, p := range report.ProviderStats {
		if p.CircuitState == CircuitOpen {
			healthBadge = "\033[1;31m[DEGRADED] AT LEAST ONE UPSTREAM CIRCUIT IS TRIPPED OPEN\033[0m"
			break
		}
	}
	sb.WriteString(fmt.Sprintf("  \033[1;37mGateway Health:\033[0m   %s\n\n", healthBadge))

	sb.WriteString("  \033[1;35mGATEWAY TRAFFIC & COST METRICS:\033[0m\n")
	sb.WriteString(fmt.Sprintf("  • Active Providers:   \033[1;33m%d\033[0m\n", report.ActiveProviders))
	sb.WriteString(fmt.Sprintf("  • Total Requests:     \033[1;32m%d\033[0m\n", report.TotalRequests))
	sb.WriteString(fmt.Sprintf("  • Cascade Failovers:  \033[1;35m%d\033[0m\n", report.TotalFailovers))
	sb.WriteString(fmt.Sprintf("  • Net Incurred Cost:  \033[1;37m$%.4f\033[0m\n", report.TotalCostUSD))
	sb.WriteString(fmt.Sprintf("  • Cache Cost Savings: \033[1;32m$%.4f (%.1f%% hit rate)\033[0m\n",
		report.TotalSavingsUSD, report.CacheStats.HitRate*100.0))

	sb.WriteString("\n  \033[1;35mUPSTREAM PROVIDER CIRCUIT STATUS & PERFORMANCE:\033[0m\n")
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")
	sb.WriteString(fmt.Sprintf("  %-20s %-12s %-10s %-10s %-12s %s\n",
		"PROVIDER ID", "CIRCUIT", "TOTAL", "SUCCESS", "AVG LATENCY", "COST USD"))
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")

	for _, p := range report.ProviderStats {
		circuitColor := "\033[32m" // Green
		if p.CircuitState == CircuitOpen {
			circuitColor = "\033[31m" // Red
		} else if p.CircuitState == CircuitHalfOpen {
			circuitColor = "\033[33m" // Yellow
		}

		sb.WriteString(fmt.Sprintf("  %-20s %s%-12s\033[0m %-10d %-10d %-12s $%.4f\n",
			p.ProviderID,
			circuitColor,
			p.CircuitState,
			p.TotalRequests,
			p.SuccessfulRequests,
			fmt.Sprintf("%.1f ms", p.AvgLatencyMs),
			p.TotalCostUSD,
		))
	}
	sb.WriteString("  " + strings.Repeat("─", 84) + "\n")

	sb.WriteString("\n  \033[1;35mSEMANTIC REPLAY CACHE AUDIT:\033[0m\n")
	sb.WriteString(fmt.Sprintf("  • Cached Entries:     %d\n", report.CacheStats.TotalEntries))
	sb.WriteString(fmt.Sprintf("  • Cache Hits:         %d\n", report.CacheStats.Hits))
	sb.WriteString(fmt.Sprintf("  • Cache Misses:       %d\n", report.CacheStats.Misses))
	sb.WriteString(fmt.Sprintf("  • Dollars Saved:      $%.4f\n", report.CacheStats.SavedCostUSD))

	sb.WriteString("\n\033[1;36m========================================================================================\033[0m\n")
	return sb.String()
}

// RenderGatewayMermaid creates a Mermaid diagram representing the dynamic routing and circuit breaker topology.
func RenderGatewayMermaid(report *GatewayReport) string {
	var sb strings.Builder

	sb.WriteString("```mermaid\n")
	sb.WriteString("graph TD\n")
	sb.WriteString("    CLIENT[\"Agent Client SDK<br/>(OpenAI Compatible)\"] --> GW[\"AgentVCR Gateway Engine\"]\n")
	sb.WriteString("    GW --> CACHE{\"Deterministic<br/>Replay Cache\"}\n")
	sb.WriteString("    CACHE -->|\"Cache Hit (0ms, $0)\"| HIT[\"Cached Response\"]\n")
	sb.WriteString("    CACHE -->|\"Cache Miss\"| ROUTER[\"Semantic Task Classifier\"]\n")

	sb.WriteString("    ROUTER --> TIER_FAST[\"Tier: Fast<br/>(gpt-4o-mini / flash)\"]\n")
	sb.WriteString("    ROUTER --> TIER_STD[\"Tier: Standard<br/>(gpt-4o / sonnet)\"]\n")
	sb.WriteString("    ROUTER --> TIER_RSN[\"Tier: Reasoning<br/>(o1 / opus)\"]\n")

	for _, p := range report.ProviderStats {
		stateBadge := string(p.CircuitState)
		sb.WriteString(fmt.Sprintf("    ROUTER -.-> CB_%s[\"Provider: %s<br/>Circuit: %s\"]\n",
			strings.ReplaceAll(p.ProviderID, "-", "_"),
			p.ProviderID,
			stateBadge,
		))
	}

	sb.WriteString("```\n")
	return sb.String()
}
