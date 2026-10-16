package causal

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/Noooblien/agentvcr/pkg/trace"
)

// Engine synthesizes causal DAGs, performs backwards root-cause isolation, and generates repair patches.
type Engine struct{}

// NewEngine creates a new Causal DAG and Root Cause Analysis engine.
func NewEngine() *Engine {
	return &Engine{}
}

// SynthesizeDAG constructs a directed acyclic graph representing causal links across trajectory steps.
func (e *Engine) SynthesizeDAG(session *trace.Session) (*CausalDAG, error) {
	if session == nil {
		return nil, fmt.Errorf("session cannot be nil")
	}

	dag := &CausalDAG{
		SessionID: session.ID,
		Nodes:     make([]CausalNode, 0, len(session.Events)),
		Edges:     make([]CausalEdge, 0),
	}

	for _, ev := range session.Events {
		node := e.buildNode(ev)
		dag.Nodes = append(dag.Nodes, node)
	}

	// Build edges based on chronological step progression, data flow, and error triggers
	for i := 0; i < len(dag.Nodes); i++ {
		curr := dag.Nodes[i]

		// 1. Direct operational predecessor edge
		if i > 0 {
			prev := dag.Nodes[i-1]
			dag.Edges = append(dag.Edges, CausalEdge{
				FromID:      prev.ID,
				ToID:        curr.ID,
				Type:        EdgeToolSequence,
				Description: fmt.Sprintf("Step %d preceded step %d", prev.StepIndex, curr.StepIndex),
				Confidence:  1.0,
			})
		}

		// 2. Data flow dependencies: check if outputs from past steps appear in inputs of current step
		for j := 0; j < i; j++ {
			past := dag.Nodes[j]
			overlap := findDataOverlap(past.Outputs, curr.Inputs)
			if len(overlap) > 0 {
				dag.Edges = append(dag.Edges, CausalEdge{
					FromID:      past.ID,
					ToID:        curr.ID,
					Type:        EdgeDataDependency,
					Description: fmt.Sprintf("Data flow on [%s] from step %d to %d", strings.Join(overlap, ", "), past.StepIndex, curr.StepIndex),
					Confidence:  0.90,
				})
			}
		}

		// 3. Error trigger link
		if curr.IsError && i > 0 {
			// Find preceding tool call or LLM request that directly led to this error
			for j := i - 1; j >= 0; j-- {
				past := dag.Nodes[j]
				if past.EventType == trace.EventToolCall || past.EventType == trace.EventLLMRequest {
					dag.Edges = append(dag.Edges, CausalEdge{
						FromID:      past.ID,
						ToID:        curr.ID,
						Type:        EdgeErrorTrigger,
						Description: fmt.Sprintf("Invocation at step %d triggered failure at step %d", past.StepIndex, curr.StepIndex),
						Confidence:  0.95,
					})
					break
				}
			}
		}
	}

	return dag, nil
}

// DiagnoseRootCauses performs backwards graph traversal to locate failure origins and blast radius.
func (e *Engine) DiagnoseRootCauses(session *trace.Session, dag *CausalDAG) (*CausalReport, error) {
	if session == nil || dag == nil {
		return nil, fmt.Errorf("session and dag cannot be nil")
	}

	report := &CausalReport{
		SessionID:          session.ID,
		TotalNodes:         len(dag.Nodes),
		TotalEdges:         len(dag.Edges),
		RootCauses:         make([]RootCauseAnalysis, 0),
		RecommendedPatches: make([]RepairPatch, 0),
		DAG:                dag,
	}

	// Locate error nodes
	var errorNodes []CausalNode
	for _, n := range dag.Nodes {
		if n.IsError {
			errorNodes = append(errorNodes, n)
		}
	}

	if len(errorNodes) == 0 {
		report.HasFailures = false
		return report, nil
	}

	report.HasFailures = true

	for _, errNode := range errorNodes {
		rc := e.traceBackwardsRootCause(errNode, dag)
		report.RootCauses = append(report.RootCauses, rc)

		patch := e.synthesizeRepairPatch(rc, session)
		if patch != nil {
			report.RecommendedPatches = append(report.RecommendedPatches, *patch)
		}
	}

	return report, nil
}

func (e *Engine) traceBackwardsRootCause(errNode CausalNode, dag *CausalDAG) RootCauseAnalysis {
	causalChain := []int{errNode.StepIndex}
	currentID := errNode.ID
	rootNode := errNode

	// Backwards traversal via incoming edges
	for {
		foundPrev := false
		for _, edge := range dag.Edges {
			if edge.ToID == currentID && (edge.Type == EdgeErrorTrigger || edge.Type == EdgeDataDependency) {
				for _, n := range dag.Nodes {
					if n.ID == edge.FromID {
						causalChain = append([]int{n.StepIndex}, causalChain...)
						rootNode = n
						currentID = n.ID
						foundPrev = true
						break
					}
				}
				if foundPrev {
					break
				}
			}
		}
		if !foundPrev {
			break
		}
	}

	// Compute blast radius: all downstream steps after root cause
	var blastRadius []int
	for _, n := range dag.Nodes {
		if n.StepIndex >= rootNode.StepIndex {
			blastRadius = append(blastRadius, n.StepIndex)
		}
	}

	rootCauseType := "unhandled_tool_error"
	desc := fmt.Sprintf("Error at step %d stemmed from failure in step %d", errNode.StepIndex, rootNode.StepIndex)

	if rootNode.EventType == trace.EventToolCall {
		rootCauseType = "bad_tool_invocation"
		desc = fmt.Sprintf("Tool call '%s' at step %d provided invalid arguments leading to error at step %d", rootNode.ToolName, rootNode.StepIndex, errNode.StepIndex)
	} else if rootNode.EventType == trace.EventLLMRequest {
		rootCauseType = "prompt_hallucination"
		desc = fmt.Sprintf("Prompt at step %d induced invalid actions culminating in error at step %d", rootNode.StepIndex, errNode.StepIndex)
	}

	return RootCauseAnalysis{
		SessionID:        dag.SessionID,
		FailureStep:      errNode.StepIndex,
		FailureEventID:   errNode.ID,
		RootCauseStep:    rootNode.StepIndex,
		RootCauseNodeID:  rootNode.ID,
		RootCauseType:    rootCauseType,
		Description:      desc,
		CausalChain:      causalChain,
		BlastRadiusSteps: blastRadius,
	}
}

func (e *Engine) synthesizeRepairPatch(rc RootCauseAnalysis, session *trace.Session) *RepairPatch {
	var targetEvent *trace.TraceEvent
	for _, ev := range session.Events {
		if ev.Index == rc.RootCauseStep {
			targetEvent = &ev
			break
		}
	}

	if targetEvent == nil {
		return nil
	}

	patch := &RepairPatch{
		PatchID:         fmt.Sprintf("patch_step_%d", rc.RootCauseStep),
		TargetStep:      rc.RootCauseStep,
		ModifiedPayload: make(map[string]interface{}),
	}

	for k, v := range targetEvent.Payload {
		patch.ModifiedPayload[k] = v
	}

	switch rc.RootCauseType {
	case "bad_tool_invocation":
		patch.Action = "modify_args"
		patch.Reason = fmt.Sprintf("Sanitize arguments for '%s' to eliminate failure at step %d", targetEvent.Payload["tool_name"], rc.FailureStep)
		patch.ModifiedPayload["sanitized"] = true
		patch.ModifiedPayload["safe_mode"] = true
	case "prompt_hallucination":
		patch.Action = "inject_guardrail"
		patch.Reason = "Inject explicit factual constraint and boundary guardrail into LLM prompt"
		patch.SuggestedPrompt = "Ensure tool calls strictly validate schema before execution."
	default:
		patch.Action = "retry_with_backoff"
		patch.Reason = "Transient failure origin: retry with exponential backoff"
	}

	return patch
}

func (e *Engine) buildNode(ev trace.TraceEvent) CausalNode {
	isErr := ev.Type == trace.EventError
	if !isErr && ev.Payload != nil {
		if _, ok := ev.Payload["error"]; ok {
			isErr = true
		}
		if status, ok := ev.Payload["status"].(string); ok && (status == "error" || status == "failed") {
			isErr = true
		}
	}

	toolName := ""
	for _, k := range []string{"tool_name", "tool", "name"} {
		if v, ok := ev.Payload[k].(string); ok {
			toolName = v
			break
		}
	}

	inputs := extractKeysAndTokens(ev.Payload, []string{"params", "args", "arguments", "input", "prompt", "query", "command"})
	outputs := extractKeysAndTokens(ev.Payload, []string{"result", "response", "output", "data", "content"})

	summary := fmt.Sprintf("[%s] step %d", ev.Type, ev.Index)
	if toolName != "" {
		summary = fmt.Sprintf("call %s() at step %d", toolName, ev.Index)
	}

	return CausalNode{
		ID:        ev.ID,
		StepIndex: ev.Index,
		EventType: ev.Type,
		Summary:   summary,
		ToolName:  toolName,
		Inputs:    inputs,
		Outputs:   outputs,
		IsError:   isErr,
		Payload:   ev.Payload,
	}
}

func findDataOverlap(a, b []string) []string {
	m := make(map[string]bool)
	for _, s := range a {
		m[s] = true
	}
	var overlap []string
	for _, s := range b {
		if m[s] {
			overlap = append(overlap, s)
		}
	}
	return overlap
}

func extractKeysAndTokens(payload map[string]interface{}, candidateKeys []string) []string {
	if payload == nil {
		return nil
	}
	var results []string
	for _, k := range candidateKeys {
		if val, ok := payload[k]; ok {
			str := fmt.Sprintf("%v", val)
			// Extract key words / identifiers > 3 characters
			re := regexp.MustCompile(`[a-zA-Z0-9_\-\.]{4,}`)
			matches := re.FindAllString(str, -1)
			for _, m := range matches {
				results = append(results, strings.ToLower(m))
			}
		}
	}
	return results
}

// RenderCausalScorecard renders an executive terminal view of the causal root-cause analysis.
func RenderCausalScorecard(report *CausalReport) string {
	var b strings.Builder

	b.WriteString("\n🕸️  [AgentVCR] Cross-Trajectory Causal DAG & Automated Root-Cause Engine\n")
	b.WriteString("═══════════════════════════════════════════════════════════════════════════\n")
	b.WriteString(fmt.Sprintf("  Session ID:             %s\n", report.SessionID))
	b.WriteString(fmt.Sprintf("  Total Causal Nodes:     %d\n", report.TotalNodes))
	b.WriteString(fmt.Sprintf("  Causal Dependency Links:%d\n", report.TotalEdges))

	if !report.HasFailures {
		b.WriteString("  Trajectory Health:      ✅ PERFECT (Zero failures detected in causal DAG)\n")
		b.WriteString("═══════════════════════════════════════════════════════════════════════════\n")
		return b.String()
	}

	b.WriteString(fmt.Sprintf("  Failures Diagnosed:     ❌ %d failures detected\n", len(report.RootCauses)))
	b.WriteString("───────────────────────────────────────────────────────────────────────────\n")
	b.WriteString("  Root-Cause Isolations:\n")

	for i, rc := range report.RootCauses {
		b.WriteString(fmt.Sprintf("    💥 [%d] Failure at Step %d: %s\n", i+1, rc.FailureStep, rc.Description))
		b.WriteString(fmt.Sprintf("        Originating Root Step: Step %d (Type: %s)\n", rc.RootCauseStep, rc.RootCauseType))

		chainStr := make([]string, len(rc.CausalChain))
		for idx, step := range rc.CausalChain {
			chainStr[idx] = fmt.Sprintf("Step %d", step)
		}
		b.WriteString(fmt.Sprintf("        Causal Propagation:    %s\n", strings.Join(chainStr, " ➔ ")))
		b.WriteString(fmt.Sprintf("        Blast Radius:          %d steps affected\n", len(rc.BlastRadiusSteps)))
	}

	if len(report.RecommendedPatches) > 0 {
		b.WriteString("───────────────────────────────────────────────────────────────────────────\n")
		b.WriteString("  Automated Repair Patches (for 'agentvcr fork'):\n")
		for i, p := range report.RecommendedPatches {
			b.WriteString(fmt.Sprintf("    🔧 [%d] Target Step: %d (Action: %s)\n", i+1, p.TargetStep, p.Action))
			b.WriteString(fmt.Sprintf("        Rationale: %s\n", p.Reason))
			if p.SuggestedPrompt != "" {
				b.WriteString(fmt.Sprintf("        Suggested Guardrail: \"%s\"\n", p.SuggestedPrompt))
			}
		}
	}
	b.WriteString("═══════════════════════════════════════════════════════════════════════════\n")

	return b.String()
}

// ExportMermaidDiagram generates a Mermaid flowchart visualization of the Causal DAG.
func ExportMermaidDiagram(dag *CausalDAG) string {
	var b strings.Builder
	b.WriteString("```mermaid\nflowchart TD\n")

	for _, n := range dag.Nodes {
		label := fmt.Sprintf("Step %d<br/>%s", n.StepIndex, n.EventType)
		if n.ToolName != "" {
			label = fmt.Sprintf("Step %d: %s()<br/>%s", n.StepIndex, n.ToolName, n.EventType)
		}
		if n.IsError {
			b.WriteString(fmt.Sprintf("    %s[\"%s\"]:::errorNode\n", n.ID, label))
		} else {
			b.WriteString(fmt.Sprintf("    %s[\"%s\"]\n", n.ID, label))
		}
	}

	for _, e := range dag.Edges {
		style := ""
		if e.Type == EdgeErrorTrigger {
			style = "|error|"
		} else if e.Type == EdgeDataDependency {
			style = "|data|"
		}
		b.WriteString(fmt.Sprintf("    %s -->%s %s\n", e.FromID, style, e.ToID))
	}

	b.WriteString("    classDef errorNode fill:#ff4d4d,stroke:#b30000,stroke-width:2px,color:#fff;\n")
	b.WriteString("```\n")
	return b.String()
}
