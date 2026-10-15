package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Noooblien/agentvcr/pkg/cluster"
	"github.com/Noooblien/agentvcr/pkg/dashboard"
	"github.com/Noooblien/agentvcr/pkg/diff"
	"github.com/Noooblien/agentvcr/pkg/eval"
	"github.com/Noooblien/agentvcr/pkg/fuzz"
	"github.com/Noooblien/agentvcr/pkg/otel"
	"github.com/Noooblien/agentvcr/pkg/proxy"
	"github.com/Noooblien/agentvcr/pkg/replay"
	"github.com/Noooblien/agentvcr/pkg/snapshot"
	"github.com/Noooblien/agentvcr/pkg/storage"
	"github.com/Noooblien/agentvcr/pkg/trace"
	"github.com/Noooblien/agentvcr/pkg/tui"
)

func main() {
	if len(os.Args) < 2 {
		printHelp()
		return
	}

	subcommand := os.Args[1]

	store, err := storage.NewStore("./.vcr_traces")
	if err != nil {
		log.Fatalf("Fatal: %v", err)
	}

	switch subcommand {
	case "record":
		recordCmd := flag.NewFlagSet("record", flag.ExitOnError)
		port := recordCmd.Int("port", 8088, "Local proxy port")
		target := recordCmd.String("target", "https://api.openai.com", "Target upstream API")
		agentName := recordCmd.String("agent", "autonomous-agent", "Name of the agent under recording")
		_ = recordCmd.Parse(os.Args[2:])

		sessionID := fmt.Sprintf("trace_%d", time.Now().Unix())
		srv, err := proxy.NewServer(*target, store, sessionID, *agentName)
		if err != nil {
			log.Fatalf("Failed to initialize proxy server: %v", err)
		}

		fmt.Printf("🔴 [AgentVCR] Recording Flight Recorder Active\n")
		fmt.Printf("    Session ID: %s\n", sessionID)
		fmt.Printf("    Proxy listening on: http://localhost:%d\n", *port)
		fmt.Printf("    Forwarding to:      %s\n", *target)
		fmt.Printf("    Trace directory:    ./.vcr_traces/%s.vcr.json\n\n", sessionID)
		fmt.Println("Point your agent's baseURL to http://localhost:8088/v1 and run your agent.")

		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), srv))

	case "view":
		viewCmd := flag.NewFlagSet("view", flag.ExitOnError)
		sessionID := viewCmd.String("id", "", "Trace Session ID")
		_ = viewCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			sessions, _ := store.ListSessions()
			fmt.Println("Available recorded sessions:")
			for _, s := range sessions {
				fmt.Printf("  - %s\n", s)
			}
			return
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session: %v", err)
		}

		engine := replay.NewEngine(store)
		engine.PrintSummary(session)

	case "fork":
		forkCmd := flag.NewFlagSet("fork", flag.ExitOnError)
		sessionID := forkCmd.String("id", "", "Parent trace session ID")
		step := forkCmd.Int("step", 0, "Step index to branch from")
		_ = forkCmd.Parse(os.Args[2:])

		if *sessionID == "" || *step == 0 {
			log.Fatal("Usage: agentvcr fork -id <session_id> -step <step_number>")
		}

		newID := fmt.Sprintf("%s_fork_step%d_%d", *sessionID, *step, time.Now().Unix())
		engine := replay.NewEngine(store)
		forked, err := engine.ForkSession(*sessionID, *step, newID)
		if err != nil {
			log.Fatalf("Failed to fork: %v", err)
		}

		fmt.Printf("🌱 [AgentVCR] Created execution branch '%s' at step %d\n", forked.ID, *step)
		fmt.Printf("Ready for mutation and targeted replay.\n")

	case "diff":
		diffCmd := flag.NewFlagSet("diff", flag.ExitOnError)
		baseID := diffCmd.String("base", "", "Base trace session ID")
		targetID := diffCmd.String("target", "", "Target/forked trace session ID")
		_ = diffCmd.Parse(os.Args[2:])

		if *baseID == "" || *targetID == "" {
			log.Fatal("Usage: agentvcr diff -base <base_id> -target <target_id>")
		}

		baseSession, err := store.LoadSession(*baseID)
		if err != nil {
			log.Fatalf("Error loading base session: %v", err)
		}
		targetSession, err := store.LoadSession(*targetID)
		if err != nil {
			log.Fatalf("Error loading target session: %v", err)
		}

		res := diff.CompareSessions(baseSession, targetSession)
		fmt.Printf("🔍 [AgentVCR] Trajectory Comparison: %s ➔ %s\n", *baseID, *targetID)
		fmt.Printf("   Event Count:     %d vs %d\n", res.BaseEventCount, res.TargetEventCount)
		fmt.Printf("   Token Delta:     %+d tokens\n", res.TokenDelta)
		fmt.Printf("   Cost Delta:      $%+.4f USD\n", res.CostDeltaUSD)
		fmt.Printf("   Duration Delta:  %+d ms\n", res.DurationDeltaMs)
		if res.DivergenceStep == -1 {
			fmt.Printf("   Status:          Identical execution trajectories.\n")
		} else {
			fmt.Printf("   ⚡ Divergence:   Step %d\n", res.DivergenceStep)
			if res.DivergentDetails != nil {
				fmt.Printf("   Reason:          %s\n", res.DivergentDetails.Reason)
			}
		}

	case "eval":
		evalCmd := flag.NewFlagSet("eval", flag.ExitOnError)
		sessionID := evalCmd.String("id", "", "Trace session ID to evaluate")
		rulesPath := evalCmd.String("rules", "", "Path to JSON file containing assertion rules (optional)")
		maxTokens := evalCmd.Int("max-tokens", 0, "Max allowed token budget")
		maxCost := evalCmd.Float64("max-cost", 0.0, "Max allowed USD cost")
		noErrors := evalCmd.Bool("no-errors", false, "Assert zero tool execution errors")
		_ = evalCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr eval -id <session_id> [-rules suite.json] [-max-tokens 5000] [-max-cost 0.05] [-no-errors]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session: %v", err)
		}

		var rules []eval.AssertionRule
		if *rulesPath != "" {
			data, err := os.ReadFile(*rulesPath)
			if err != nil {
				log.Fatalf("Failed to read rules file %s: %v", *rulesPath, err)
			}
			if err := json.Unmarshal(data, &rules); err != nil {
				log.Fatalf("Invalid rules JSON in %s: %v", *rulesPath, err)
			}
		}

		if *maxTokens > 0 {
			rules = append(rules, eval.AssertionRule{
				Type:      eval.RuleMaxTokens,
				MaxTokens: *maxTokens,
			})
		}
		if *maxCost > 0.0 {
			rules = append(rules, eval.AssertionRule{
				Type:       eval.RuleMaxCost,
				MaxCostUSD: *maxCost,
			})
		}
		if *noErrors {
			rules = append(rules, eval.AssertionRule{
				Type: eval.RuleNoErrors,
			})
		}

		if len(rules) == 0 {
			log.Fatal("No evaluation assertions specified. Provide -rules, -max-tokens, -max-cost, or -no-errors")
		}

		report, err := eval.Evaluate(session, rules)
		if err != nil {
			log.Fatalf("Evaluation failed: %v", err)
		}

		fmt.Printf("🧪 [AgentVCR] Trajectory Assertion Suite for %s\n", *sessionID)
		fmt.Printf("   Assertions Evaluated: %d\n", report.TotalRules)
		fmt.Printf("   Passed: %d, Failed: %d\n\n", report.Passed, report.Failed)

		for i, res := range report.Results {
			icon := "✅"
			if !res.Passed {
				icon = "❌"
			}
			fmt.Printf("   %s [%d] %s: %s\n", icon, i+1, res.RuleType, res.Details)
		}

		if !report.AllPassed {
			os.Exit(1)
		}
		fmt.Printf("\n✨ All assertions PASSED. Trajectory meets SLA.\n")

	case "play":
		playCmd := flag.NewFlagSet("play", flag.ExitOnError)
		sessionID := playCmd.String("id", "", "Trace Session ID to replay")
		delayMs := playCmd.Int("delay", 250, "Playback delay between steps in milliseconds")
		_ = playCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr play -id <session_id> [-delay 250]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session: %v", err)
		}

		player := tui.NewPlayer(os.Stdout)
		player.PlaySession(session, time.Duration(*delayMs)*time.Millisecond)

	case "snapshot":
		snapCmd := flag.NewFlagSet("snapshot", flag.ExitOnError)
		dir := snapCmd.String("dir", ".", "Directory path to snapshot")
		sessionID := snapCmd.String("id", "manual", "Session ID associate with snapshot")
		step := snapCmd.Int("step", 0, "Execution step index")
		restoreTo := snapCmd.String("restore", "", "Target directory path to restore snapshot to")
		_ = snapCmd.Parse(os.Args[2:])

		mgr := snapshot.NewManager()
		snap, err := mgr.CaptureWorkspace(*sessionID, *step, *dir)
		if err != nil {
			log.Fatalf("Failed to capture workspace snapshot: %v", err)
		}

		fmt.Printf("📸 [AgentVCR] Workspace Snapshot Captured\n")
		fmt.Printf("   Snapshot ID: %s\n", snap.ID)
		fmt.Printf("   Root Path:   %s\n", snap.RootPath)
		fmt.Printf("   Files:       %d tracked entries\n", len(snap.Entries))

		if *restoreTo != "" {
			if err := mgr.RestoreWorkspace(snap.ID, *restoreTo); err != nil {
				log.Fatalf("Failed to restore workspace: %v", err)
			}
			fmt.Printf("   Restored To: %s\n", *restoreTo)
		}

	case "cluster":
		sessions, err := store.ListSessions()
		if err != nil {
			log.Fatalf("Failed to list sessions: %v", err)
		}

		var loadedSessions []*trace.Session
		for _, sID := range sessions {
			s, err := store.LoadSession(sID)
			if err == nil {
				loadedSessions = append(loadedSessions, s)
			}
		}

		if len(loadedSessions) == 0 {
			fmt.Println("No recorded trace sessions found in ./.vcr_traces to analyze.")
			return
		}

		report := cluster.GenerateFleetReport(loadedSessions)
		fmt.Print(cluster.RenderASCIIHeatmap(report))

	case "fuzz":
		fuzzCmd := flag.NewFlagSet("fuzz", flag.ExitOnError)
		sessionID := fuzzCmd.String("id", "", "Baseline trace session ID to fuzz")
		strategy := fuzzCmd.String("strategy", "all", "Mutation strategy (prompt_injection, tool_fault, malformed_json, latency_spike, all)")
		count := fuzzCmd.Int("count", 3, "Number of mutated trajectories to generate")
		_ = fuzzCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr fuzz -id <session_id> [-strategy <strategy>] [-count 3]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		harness := fuzz.NewFuzzHarness(0)
		var fuzzedSessions []*trace.Session
		var reports []*fuzz.MutationReport

		if *strategy == "all" {
			fuzzedSessions, reports, err = harness.GenerateSuite(session, *count)
		} else {
			for i := 0; i < *count; i++ {
				s, r, errFuzz := harness.FuzzSession(session, fuzz.MutationStrategy(*strategy))
				if errFuzz == nil {
					fuzzedSessions = append(fuzzedSessions, s)
					reports = append(reports, r)
				}
			}
		}

		if err != nil || len(fuzzedSessions) == 0 {
			log.Fatalf("Fuzzing failed or yielded no mutations: %v", err)
		}

		fmt.Printf("☣️  [AgentVCR] Adversarial Fuzzing Suite Generated for %s\n", *sessionID)
		for i, rep := range reports {
			_ = store.SaveSession(fuzzedSessions[i])
			fmt.Printf("   [%d] Fuzzed Session: %s\n", i+1, rep.FuzzedSessionID)
			fmt.Printf("       Strategy: %s | Step: %d\n", rep.Strategy, rep.MutatedStep)
			fmt.Printf("       Detail:   %s\n\n", rep.Description)
		}
		fmt.Printf("Saved %d mutated trajectories to ./.vcr_traces for replay and SLA verification.\n", len(fuzzedSessions))

	case "serve":
		serveCmd := flag.NewFlagSet("serve", flag.ExitOnError)
		port := serveCmd.Int("port", 9090, "HTTP dashboard port")
		_ = serveCmd.Parse(os.Args[2:])

		srv := dashboard.NewServer(store)
		fmt.Printf("📡 [AgentVCR] Real-Time Telemetry Dashboard running at http://localhost:%d\n", *port)
		fmt.Printf("   Endpoints:\n")
		fmt.Printf("     - UI Dashboard: http://localhost:%d/\n", *port)
		fmt.Printf("     - Sessions API: http://localhost:%d/api/sessions\n", *port)
		fmt.Printf("     - Fleet Metrics: http://localhost:%d/api/metrics\n", *port)
		fmt.Printf("     - Live SSE Stream: http://localhost:%d/api/live/stream\n\n", *port)

		log.Fatal(http.ListenAndServe(fmt.Sprintf(":%d", *port), srv.Handler()))

	case "export":
		exportCmd := flag.NewFlagSet("export", flag.ExitOnError)
		sessionID := exportCmd.String("id", "", "Trace Session ID to export")
		format := exportCmd.String("format", "otlp", "Export format (otlp or spans)")
		outPath := exportCmd.String("out", "", "Output file path (default stdout)")
		serviceName := exportCmd.String("service", "agentvcr", "OTel service name")
		_ = exportCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr export -id <session_id> [-format otlp|spans] [-out trace.json] [-service agentvcr]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		exporter := otel.NewExporter(*serviceName)
		var outputBytes []byte

		if *format == "spans" {
			spans := exporter.ExportToOTelSpans(session)
			outputBytes, err = json.MarshalIndent(spans, "", "  ")
		} else {
			outputBytes, err = exporter.ExportOTLPJSON(session)
		}

		if err != nil {
			log.Fatalf("Export failed: %v", err)
		}

		if *outPath != "" {
			if err := os.WriteFile(*outPath, outputBytes, 0644); err != nil {
				log.Fatalf("Failed to write to %s: %v", *outPath, err)
			}
			fmt.Printf("📦 [AgentVCR] Exported OpenTelemetry trace for %s to %s\n", *sessionID, *outPath)
		} else {
			fmt.Println(string(outputBytes))
		}

	default:
		printHelp()
	}
}

func printHelp() {
	fmt.Println("AgentVCR — Deterministic Flight Recorder & Time-Travel Debugger for AI Agents")
	fmt.Println("\nCommands:")
	fmt.Println("  record    Start transparent reverse-proxy recording session")
	fmt.Println("  view      Inspect recorded agent trajectory and token events")
	fmt.Println("  fork      Branch an execution trace at an exact step for re-testing")
	fmt.Println("  diff      Compare two trajectories to pinpoint execution divergence")
	fmt.Println("  eval      Run deterministic assertion suites and SLAs on trajectories")
	fmt.Println("  play      Interactive visual terminal playback of agent execution")
	fmt.Println("  snapshot  Capture or restore deterministic workspace filesystem state")
	fmt.Println("  cluster   Fleet-wide trajectory pattern clustering & usage heatmaps")
	fmt.Println("  fuzz      Adversarial prompt injection & fault mutation harness")
	fmt.Println("  serve     Host real-time streaming telemetry dashboard & API")
	fmt.Println("  export    Export session traces to OpenTelemetry (OTel / OTLP JSON)")
	fmt.Println("\nExamples:")
	fmt.Println("  agentvcr record --port 8088 --target https://api.openai.com")
	fmt.Println("  agentvcr view -id trace_1760800000")
	fmt.Println("  agentvcr fork -id trace_1760800000 -step 4")
	fmt.Println("  agentvcr diff -base trace_1 -target trace_1_fork")
	fmt.Println("  agentvcr eval -id trace_1 -no-errors -max-tokens 5000")
	fmt.Println("  agentvcr play -id trace_1 -delay 100")
	fmt.Println("  agentvcr snapshot -dir ./workspace -step 3")
	fmt.Println("  agentvcr cluster")
	fmt.Println("  agentvcr fuzz -id trace_1 -strategy prompt_injection -count 3")
	fmt.Println("  agentvcr serve --port 9090")
	fmt.Println("  agentvcr export -id trace_1 -format otlp -out trace_otel.json")
}
