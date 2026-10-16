package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/Noooblien/agentvcr/pkg/attest"
	"github.com/Noooblien/agentvcr/pkg/bench"
	"github.com/Noooblien/agentvcr/pkg/causal"
	"github.com/Noooblien/agentvcr/pkg/cluster"
	"github.com/Noooblien/agentvcr/pkg/consensus"
	"github.com/Noooblien/agentvcr/pkg/dashboard"
	"github.com/Noooblien/agentvcr/pkg/diff"
	"github.com/Noooblien/agentvcr/pkg/eval"
	"github.com/Noooblien/agentvcr/pkg/forensics"
	"github.com/Noooblien/agentvcr/pkg/fuzz"
	"github.com/Noooblien/agentvcr/pkg/gossip"
	"github.com/Noooblien/agentvcr/pkg/leaderboard"
	"github.com/Noooblien/agentvcr/pkg/lineage"
	"github.com/Noooblien/agentvcr/pkg/otel"
	"github.com/Noooblien/agentvcr/pkg/policy"
	"github.com/Noooblien/agentvcr/pkg/privacy"
	"github.com/Noooblien/agentvcr/pkg/proxy"
	"github.com/Noooblien/agentvcr/pkg/replay"
	"github.com/Noooblien/agentvcr/pkg/sim"
	"github.com/Noooblien/agentvcr/pkg/snapshot"
	"github.com/Noooblien/agentvcr/pkg/speculate"
	"github.com/Noooblien/agentvcr/pkg/storage"
	"github.com/Noooblien/agentvcr/pkg/tenant"
	"github.com/Noooblien/agentvcr/pkg/throttle"
	"github.com/Noooblien/agentvcr/pkg/trace"
	"github.com/Noooblien/agentvcr/pkg/tui"
	"github.com/Noooblien/agentvcr/pkg/vision"
	"github.com/Noooblien/agentvcr/pkg/zkp"
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

	case "sign":
		signCmd := flag.NewFlagSet("sign", flag.ExitOnError)
		sessionID := signCmd.String("id", "", "Trace Session ID to cryptographically sign")
		certPath := signCmd.String("out", "", "Output path for attestation certificate (default: <session_id>.cert.json)")
		_ = signCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr sign -id <session_id> [-out cert.json]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		keypair, err := attest.GenerateKeyPair()
		if err != nil {
			log.Fatalf("Keypair generation failed: %v", err)
		}

		cert, err := attest.SignTrajectory(session, keypair.PrivateKey)
		if err != nil {
			log.Fatalf("Failed to sign trajectory: %v", err)
		}

		certBytes, _ := json.MarshalIndent(cert, "", "  ")
		dest := *certPath
		if dest == "" {
			dest = fmt.Sprintf("./.vcr_traces/%s.cert.json", *sessionID)
		}
		if err := os.WriteFile(dest, certBytes, 0644); err != nil {
			log.Fatalf("Failed to save certificate: %v", err)
		}

		fmt.Printf("🔏 [AgentVCR] Cryptographic Attestation Certificate Generated\n")
		fmt.Printf("   Session:    %s\n", cert.SessionID)
		fmt.Printf("   Root Hash:  %s\n", cert.RootHash)
		fmt.Printf("   Public Key: %s\n", cert.PublicKey)
		fmt.Printf("   Saved To:   %s\n", dest)

	case "verify":
		verifyCmd := flag.NewFlagSet("verify", flag.ExitOnError)
		sessionID := verifyCmd.String("id", "", "Trace Session ID to verify")
		certPath := verifyCmd.String("cert", "", "Path to attestation certificate file")
		_ = verifyCmd.Parse(os.Args[2:])

		if *sessionID == "" || *certPath == "" {
			log.Fatal("Usage: agentvcr verify -id <session_id> -cert <cert.json>")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		certBytes, err := os.ReadFile(*certPath)
		if err != nil {
			log.Fatalf("Failed to read certificate %s: %v", *certPath, err)
		}

		var cert attest.Certificate
		if err := json.Unmarshal(certBytes, &cert); err != nil {
			log.Fatalf("Invalid certificate JSON: %v", err)
		}

		result := attest.VerifyTrajectory(session, &cert)
		fmt.Printf("🛡️  [AgentVCR] Attestation Verification for %s\n", *sessionID)
		fmt.Printf("   Signature Valid:  %t\n", result.SignatureOK)
		fmt.Printf("   Hash Chain Valid: %t\n", result.HashChainOK)
		if result.Valid {
			fmt.Printf("   Status:           ✅ PASS: Trajectory is cryptographically authentic.\n")
		} else {
			fmt.Printf("   Status:           ❌ FAIL: Trajectory has been tampered with!\n")
			fmt.Printf("   Details:          %s\n", result.Details)
			os.Exit(1)
		}

	case "sim":
		simCmd := flag.NewFlagSet("sim", flag.ExitOnError)
		mode := simCmd.String("mode", "run", "Simulation mode: 'run', 'synthesize', or 'list'")
		sessionID := simCmd.String("id", "", "Trace Session ID")
		mocksPath := simCmd.String("mocks", "", "Path to custom mock rules JSON file")
		outPath := simCmd.String("out", "", "Output path for synthesized rules or simulated session ID")
		_ = simCmd.Parse(os.Args[2:])

		if *mode == "list" {
			fmt.Println("🤖 [AgentVCR] Built-in Offline Mock Tool Providers:")
			fmt.Println("  - bash_sandbox:  Deterministic shell sandbox (echo, pwd, whoami, date)")
			fmt.Println("  - memory_kv:     Stateful in-memory key-value store (get, set, delete)")
			fmt.Println("  - mock_search:   Deterministic synthetic web search provider")
			return
		}

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr sim -mode <run|synthesize|list> -id <session_id> [-mocks mocks.json] [-out <dest>]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		if *mode == "synthesize" {
			synth := sim.NewSynthesizer()
			rules, err := synth.SynthesizeFromSession(session)
			if err != nil {
				log.Fatalf("Synthesis failed: %v", err)
			}
			jsonBytes, _ := synth.ExportRulesJSON(rules)
			dest := *outPath
			if dest == "" {
				dest = fmt.Sprintf("./.vcr_traces/%s.mocks.json", *sessionID)
			}
			if err := os.WriteFile(dest, jsonBytes, 0644); err != nil {
				log.Fatalf("Failed to write mock rules to %s: %v", dest, err)
			}
			fmt.Printf("🎭 [AgentVCR] Synthesized %d mock tool rules from %s\n", len(rules), *sessionID)
			fmt.Printf("   Saved To: %s\n", dest)
			return
		}

		// Mode: run
		var customRules []sim.MockRule
		if *mocksPath != "" {
			data, err := os.ReadFile(*mocksPath)
			if err != nil {
				log.Fatalf("Failed to read mocks file %s: %v", *mocksPath, err)
			}
			synth := sim.NewSynthesizer()
			customRules, err = synth.ImportRulesJSON(data)
			if err != nil {
				log.Fatalf("Failed to parse mock rules: %v", err)
			}
		}

		newID := *outPath
		if newID == "" {
			newID = fmt.Sprintf("%s_sim_%d", *sessionID, time.Now().Unix())
		}

		simulator := sim.NewSimulator(customRules)
		simSession, report, err := simulator.SimulateSession(session, newID)
		if err != nil {
			log.Fatalf("Offline simulation failed: %v", err)
		}

		if err := store.SaveSession(simSession); err != nil {
			log.Fatalf("Failed to save simulated session: %v", err)
		}

		fmt.Printf("🧪 [AgentVCR] Offline Trajectory Simulation Complete\n")
		fmt.Printf("   Simulated Session ID: %s\n", simSession.ID)
		fmt.Printf("   Base Session ID:      %s\n", report.BaseSessionID)
		fmt.Printf("   Total Events:         %d\n", report.TotalEvents)
		fmt.Printf("   Tool Calls Simulated: %d\n", report.ToolCallsSimulated)
		fmt.Printf("   Rules Matched:        %d\n", report.RulesMatched)
		fmt.Printf("   Fallbacks Used:       %d\n", report.FallbacksUsed)
		fmt.Printf("   Simulated Latency:    %d ms\n", report.SimulatedLatencyMs)
		fmt.Printf("   Errors Injected:      %d\n", report.ErrorsInjected)
		fmt.Printf("   Trace File:           ./.vcr_traces/%s.vcr.json\n", simSession.ID)

	case "bench":
		benchCmd := flag.NewFlagSet("bench", flag.ExitOnError)
		sessionID := benchCmd.String("id", "", "Trace Session ID to benchmark")
		suitePath := benchCmd.String("suite", "", "Path to benchmark suite JSON file")
		needle := benchCmd.String("needle", "", "Ad-hoc probe: needle text that must be recalled")
		forbidden := benchCmd.String("forbidden", "", "Ad-hoc probe: forbidden regex/pattern that must not appear")
		introStep := benchCmd.Int("intro-step", 1, "Step where needle was introduced")
		probeStep := benchCmd.Int("probe-step", 2, "Step where needle recall is asserted")
		minRecall := benchCmd.Float64("min-recall", 1.0, "Minimum acceptable recall score (0.0 - 1.0)")
		maxViolations := benchCmd.Int("max-violations", 0, "Max allowed negative constraint violations")
		_ = benchCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr bench -id <session_id> [-suite suite.json] [-needle <text>] [-forbidden <pattern>]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		var cfg bench.BenchmarkConfig
		if *suitePath != "" {
			data, err := os.ReadFile(*suitePath)
			if err != nil {
				log.Fatalf("Failed to read benchmark suite file %s: %v", *suitePath, err)
			}
			if err := json.Unmarshal(data, &cfg); err != nil {
				log.Fatalf("Invalid benchmark suite JSON: %v", err)
			}
		} else {
			cfg = bench.BenchmarkConfig{
				SuiteName:               fmt.Sprintf("Ad-hoc Memory Benchmark for %s", *sessionID),
				MinRecallScore:          *minRecall,
				MaxConstraintViolations: *maxViolations,
				MaxContextBloatRatio:    10.0,
			}
			if *needle != "" {
				cfg.Probes = append(cfg.Probes, bench.MemoryProbe{
					ID:            "adhoc-needle-recall",
					Type:          bench.ProbeNeedleRecall,
					Description:   fmt.Sprintf("Assert recall of '%s'", *needle),
					Needle:        *needle,
					IntroduceStep: *introStep,
					ProbeStep:     *probeStep,
				})
			}
			if *forbidden != "" {
				cfg.Probes = append(cfg.Probes, bench.MemoryProbe{
					ID:               "adhoc-safety-constraint",
					Type:             bench.ProbeConstraintPreservation,
					Description:      fmt.Sprintf("Assert pattern '%s' never appears", *forbidden),
					ForbiddenPattern: *forbidden,
					IntroduceStep:    *introStep,
				})
			}
			// Default context efficiency probe if no custom probes given
			if len(cfg.Probes) == 0 {
				cfg.Probes = append(cfg.Probes, bench.MemoryProbe{
					ID:          "context-efficiency-probe",
					Type:        bench.ProbeContextEfficiency,
					Description: "Measure trajectory context bloat and token expansion",
				})
			}
		}

		engine := bench.NewEngine()
		report, err := engine.RunBenchmark(session, &cfg)
		if err != nil {
			log.Fatalf("Benchmark execution failed: %v", err)
		}

		fmt.Print(bench.RenderScorecard(report))
		if !report.PassedSLA {
			os.Exit(1)
		}

	case "consensus":
		consensusCmd := flag.NewFlagSet("consensus", flag.ExitOnError)
		sessionID := consensusCmd.String("id", "", "Single trace session ID or multi-agent coordinator trace")
		sessionsList := consensusCmd.String("sessions", "", "Comma-separated list of swarm session IDs")
		quorum := consensusCmd.Float64("quorum", 0.67, "Supermajority quorum threshold (e.g. 0.67 for 2/3 + 1)")
		strict := consensusCmd.Bool("strict", false, "Exit with non-zero status if Byzantine faults detected or consensus fails")
		_ = consensusCmd.Parse(os.Args[2:])

		if *sessionID == "" && *sessionsList == "" {
			log.Fatal("Usage: agentvcr consensus [-id <session_id> | -sessions s1,s2,...] [-quorum 0.67] [-strict]")
		}

		var targetSessionIDs []string
		if *sessionsList != "" {
			parts := strings.Split(*sessionsList, ",")
			for _, p := range parts {
				trimmed := strings.TrimSpace(p)
				if trimmed != "" {
					targetSessionIDs = append(targetSessionIDs, trimmed)
				}
			}
		} else {
			targetSessionIDs = []string{*sessionID}
		}

		var loadedSessions []*trace.Session
		for _, sID := range targetSessionIDs {
			s, err := store.LoadSession(sID)
			if err != nil {
				log.Fatalf("Error loading session %s: %v", sID, err)
			}
			loadedSessions = append(loadedSessions, s)
		}

		engine := consensus.NewEngine(*quorum)
		report, err := engine.AnalyzeSessions(loadedSessions, *quorum)
		if err != nil {
			log.Fatalf("Consensus analysis failed: %v", err)
		}

		fmt.Print(consensus.RenderConsensusScorecard(report))

		if *strict && (!report.IsByzantineResilient || report.ConsensusRate < 1.0) {
			os.Exit(1)
		}

	case "forensics":
		forensicsCmd := flag.NewFlagSet("forensics", flag.ExitOnError)
		sessionID := forensicsCmd.String("id", "", "Trace Session ID to audit for RAG hallucinations")
		threshold := forensicsCmd.Float64("threshold", 0.35, "Semantic cosine similarity threshold (default 0.35)")
		strict := forensicsCmd.Bool("strict", false, "Exit with non-zero status if ungrounded claims or hallucinations detected")
		_ = forensicsCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr forensics -id <session_id> [-threshold 0.35] [-strict]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		analyzer := forensics.NewAnalyzer(*threshold)
		report, err := analyzer.AuditTrajectory(session)
		if err != nil {
			log.Fatalf("Forensics audit failed: %v", err)
		}

		fmt.Print(forensics.RenderForensicScorecard(report))

		if *strict && !report.PassedAudit {
			os.Exit(1)
		}

	case "causal":
		causalCmd := flag.NewFlagSet("causal", flag.ExitOnError)
		sessionID := causalCmd.String("id", "", "Trace Session ID to construct causal DAG and diagnose root-causes")
		mermaid := causalCmd.Bool("mermaid", false, "Output Mermaid flowchart diagram markup")
		repair := causalCmd.Bool("repair", false, "Generate automated branched repair session for first detected failure")
		_ = causalCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr causal -id <session_id> [-mermaid] [-repair]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		engine := causal.NewEngine()
		dag, err := engine.SynthesizeDAG(session)
		if err != nil {
			log.Fatalf("Failed to synthesize causal DAG: %v", err)
		}

		if *mermaid {
			fmt.Println(causal.ExportMermaidDiagram(dag))
			return
		}

		report, err := engine.DiagnoseRootCauses(session, dag)
		if err != nil {
			log.Fatalf("Failed to diagnose root causes: %v", err)
		}

		fmt.Print(causal.RenderCausalScorecard(report))

		if *repair && len(report.RecommendedPatches) > 0 {
			patch := report.RecommendedPatches[0]
			repairID := fmt.Sprintf("%s_repaired_step%d_%d", *sessionID, patch.TargetStep, time.Now().Unix())
			replayEngine := replay.NewEngine(store)
			forked, err := replayEngine.ForkSession(*sessionID, patch.TargetStep, repairID)
			if err != nil {
				log.Fatalf("Failed to branch repair session: %v", err)
			}
			// Apply repair patch to branched step
			for i := range forked.Events {
				if forked.Events[i].Index == patch.TargetStep {
					forked.Events[i].Payload = patch.ModifiedPayload
					break
				}
			}
			_ = store.SaveSession(forked)
			fmt.Printf("\n✨ [AgentVCR] Auto-healed trajectory branch created: %s\n", repairID)
			fmt.Printf("   Applied Patch: %s (Action: %s)\n", patch.PatchID, patch.Action)
			fmt.Printf("   Ready for execution replay via 'agentvcr play -id %s'\n", repairID)
		}

	case "policy":
		policyCmd := flag.NewFlagSet("policy", flag.ExitOnError)
		sessionID := policyCmd.String("id", "", "Trace Session ID to audit against zero-trust policy")
		policyFile := policyCmd.String("rules", "", "Path to custom policy rules JSON file (optional)")
		blockTool := policyCmd.String("block-tool", "", "Ad-hoc rule: regex pattern of forbidden tools")
		forbidPattern := policyCmd.String("forbid", "", "Ad-hoc rule: forbidden regex pattern (e.g. 'rm -rf')")
		blockHost := policyCmd.String("block-host", "", "Ad-hoc rule: blocked egress destination host (SSRF)")
		maxToolCalls := policyCmd.Int("max-tool-calls", 0, "Max permitted invocations per individual tool")
		redactPII := policyCmd.Bool("redact-pii", true, "Enable automated redaction of API keys and credentials")
		strict := policyCmd.Bool("strict", false, "Exit with non-zero exit code if policy breach is detected")
		_ = policyCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr policy -id <session_id> [-rules policy.json] [-block-tool <regex>] [-forbid <pattern>] [-block-host <host>] [-strict]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		var suite policy.PolicySuite
		if *policyFile != "" {
			data, err := os.ReadFile(*policyFile)
			if err != nil {
				log.Fatalf("Failed to read policy rules %s: %v", *policyFile, err)
			}
			if err := json.Unmarshal(data, &suite); err != nil {
				log.Fatalf("Invalid policy suite JSON: %v", err)
			}
		} else {
			suite = policy.PolicySuite{
				SuiteName:     fmt.Sprintf("Zero-Trust Policy Suite for %s", *sessionID),
				DefaultAction: policy.ActionAllow,
			}
			rule := policy.PolicyRule{
				ID:                "adhoc-governance-rule",
				Action:            policy.ActionDeny,
				ToolPattern:       *blockTool,
				MaxCallsPerTool:   *maxToolCalls,
				RedactPIIPatterns: *redactPII,
			}
			if *forbidPattern != "" {
				rule.ForbiddenRegex = []string{*forbidPattern}
			}
			if *blockHost != "" {
				rule.BlockedHosts = []string{*blockHost}
			}
			suite.Rules = append(suite.Rules, rule)
		}

		enforcer := policy.NewEnforcer(suite)
		report, err := enforcer.AuditSession(session)
		if err != nil {
			log.Fatalf("Policy audit failed: %v", err)
		}

		fmt.Print(policy.RenderPolicyScorecard(report))

		if *strict && !report.ZeroTrustCompliance {
			os.Exit(1)
		}

	case "lineage":
		lineageCmd := flag.NewFlagSet("lineage", flag.ExitOnError)
		sessionID := lineageCmd.String("id", "", "Trace Session ID to extract microsecond state lineage graph")
		key := lineageCmd.String("key", "", "Filter lineage history for a specific state variable/key (optional)")
		step := lineageCmd.Int("step", -1, "Target step to reconstruct time-travel sandbox state snapshot (default: all)")
		strict := lineageCmd.Bool("strict", false, "Exit with non-zero exit code if state divergence hazards are detected")
		_ = lineageCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr lineage -id <session_id> [-key <var>] [-step <n>] [-strict]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		report, err := lineage.GenerateLineageReport(session, *key, *step)
		if err != nil {
			log.Fatalf("Lineage extraction failed: %v", err)
		}

		fmt.Print(lineage.RenderLineageScorecard(report))

		if *strict && !report.EphemeralSandboxValid {
			os.Exit(1)
		}

	case "leaderboard":
		lbCmd := flag.NewFlagSet("leaderboard", flag.ExitOnError)
		sessionsArg := lbCmd.String("sessions", "", "Comma-separated list of trace session IDs to rank on the leaderboard")
		suiteName := lbCmd.String("suite", "Autonomous Engineering Benchmark", "Benchmark suite evaluation title")
		diffMode := lbCmd.Bool("diff", false, "Run head-to-head regression diff between baseline and candidate")
		baselineID := lbCmd.String("baseline", "", "Baseline session ID for regression diffing")
		candidateID := lbCmd.String("candidate", "", "Candidate session ID for regression diffing")
		strict := lbCmd.Bool("strict", false, "Exit with non-zero exit code if regression is detected or gate fails")
		_ = lbCmd.Parse(os.Args[2:])

		if *diffMode || (*baselineID != "" && *candidateID != "") {
			if *baselineID == "" || *candidateID == "" {
				log.Fatal("Usage: agentvcr leaderboard -diff -baseline <id> -candidate <id> [-strict]")
			}
			baseSess, err := store.LoadSession(*baselineID)
			if err != nil {
				log.Fatalf("Error loading baseline session %s: %v", *baselineID, err)
			}
			candSess, err := store.LoadSession(*candidateID)
			if err != nil {
				log.Fatalf("Error loading candidate session %s: %v", *candidateID, err)
			}

			diff := leaderboard.DiffSessions(baseSess, candSess, nil)
			fmt.Print(leaderboard.RenderRegressionDiffCard(diff))

			if *strict && !diff.PassGate {
				os.Exit(1)
			}
			return
		}

		var targetSessions []*trace.Session
		if *sessionsArg != "" {
			ids := strings.Split(*sessionsArg, ",")
			for _, id := range ids {
				trimmed := strings.TrimSpace(id)
				if trimmed == "" {
					continue
				}
				s, err := store.LoadSession(trimmed)
				if err != nil {
					log.Fatalf("Error loading session %s: %v", trimmed, err)
				}
				targetSessions = append(targetSessions, s)
			}
		} else {
			traces, err := store.ListSessions()
			if err != nil {
				log.Fatalf("Error listing sessions: %v", err)
			}
			for _, t := range traces {
				s, err := store.LoadSession(t)
				if err == nil {
					targetSessions = append(targetSessions, s)
				}
			}
		}

		if len(targetSessions) == 0 {
			log.Fatal("No sessions found to benchmark. Record sessions or specify -sessions <id1,id2>.")
		}

		lb := leaderboard.BuildLeaderboard(targetSessions, *suiteName)
		fmt.Print(leaderboard.RenderLeaderboardTable(lb))

	case "throttle":
		throttleCmd := flag.NewFlagSet("throttle", flag.ExitOnError)
		sessionID := throttleCmd.String("id", "", "Trace Session ID to audit for rate limiting and latency SLA compliance")
		maxRPM := throttleCmd.Int("max-rpm", 60, "Maximum requests per minute threshold")
		maxTPM := throttleCmd.Int("max-tpm", 100000, "Maximum tokens per minute threshold")
		totalBudgetMs := throttleCmd.Int64("total-budget", 30000, "Total trajectory wall-clock latency SLA ceiling in ms")
		perStepBudgetMs := throttleCmd.Int64("per-step-budget", 5000, "Per-step execution latency ceiling in ms")
		burst := throttleCmd.Int("burst", 10, "Token bucket burst capacity allowance")
		strict := throttleCmd.Bool("strict", false, "Exit with non-zero exit code if throttle or SLA breach is detected")
		_ = throttleCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr throttle -id <session_id> [-max-rpm <n>] [-max-tpm <n>] [-total-budget <ms>] [-per-step-budget <ms>] [-strict]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		policy := throttle.ThrottlePolicy{
			MaxRPM:               *maxRPM,
			MaxTPM:               *maxTPM,
			TotalLatencyBudgetMs: *totalBudgetMs,
			PerStepBudgetMs:      *perStepBudgetMs,
			BurstCapacity:        *burst,
		}

		report, err := throttle.AuditSession(session, policy)
		if err != nil {
			log.Fatalf("Throttle audit failed: %v", err)
		}

		fmt.Print(throttle.RenderThrottleScorecard(report, policy))

		if *strict && (len(report.Breaches) > 0 || report.LatencySLABreached) {
			os.Exit(1)
		}

	case "zkp":
		zkpCmd := flag.NewFlagSet("zkp", flag.ExitOnError)
		action := zkpCmd.String("action", "prove", "ZKP operation: 'prove' (generate proof) or 'verify' (audit proof)")
		sessionID := zkpCmd.String("id", "", "Trace Session ID to generate zero-knowledge attestation proof for")
		proofFile := zkpCmd.String("proof", "", "Path to ZK proof JSON file (required for 'verify', optional for 'prove' -out)")
		requireZeroPII := zkpCmd.Bool("zero-pii", true, "Prove zero PII or private credentials leaked")
		requireZeroErrors := zkpCmd.Bool("zero-errors", true, "Prove zero unhandled tool errors")
		maxCost := zkpCmd.Float64("max-cost", 0.0, "Prove total USD cost ceiling without revealing token counts")
		maxTokens := zkpCmd.Int("max-tokens", 0, "Prove token consumption ceiling")
		strict := zkpCmd.Bool("strict", false, "Exit with non-zero exit code if proof verification fails")
		_ = zkpCmd.Parse(os.Args[2:])

		switch *action {
		case "prove":
			if *sessionID == "" {
				log.Fatal("Usage: agentvcr zkp -action prove -id <session_id> [-proof <out.json>] [-zero-pii] [-zero-errors]")
			}
			session, err := store.LoadSession(*sessionID)
			if err != nil {
				log.Fatalf("Error loading session %s: %v", *sessionID, err)
			}

			claim := zkp.ProofClaim{
				ClaimID:               fmt.Sprintf("zk-claim-%s", *sessionID),
				Description:           fmt.Sprintf("Zero-Knowledge Attestation for %s", *sessionID),
				RequireZeroPII:        *requireZeroPII,
				RequireZeroToolErrors: *requireZeroErrors,
				MaxCostUSDLimit:       *maxCost,
				MaxTokensLimit:        *maxTokens,
			}

			proof, err := zkp.GenerateProof(session, claim)
			if err != nil {
				log.Fatalf("ZKP generation failed: %v", err)
			}

			outPath := *proofFile
			if outPath == "" {
				outPath = fmt.Sprintf("%s.zkp.json", *sessionID)
			}
			proofBytes, _ := json.MarshalIndent(proof, "", "  ")
			if err := os.WriteFile(outPath, proofBytes, 0644); err != nil {
				log.Fatalf("Failed to save proof file: %v", err)
			}

			verifyRes, err := zkp.VerifyProof(proof)
			if err != nil {
				log.Fatalf("Self-verification failed: %v", err)
			}
			fmt.Print(zkp.RenderZKProofScorecard(proof, verifyRes))
			fmt.Printf("🔒 [AgentVCR] Zero-knowledge proof securely written to: %s\n\n", outPath)

		case "verify":
			if *proofFile == "" {
				log.Fatal("Usage: agentvcr zkp -action verify -proof <proof.json> [-strict]")
			}
			pData, err := os.ReadFile(*proofFile)
			if err != nil {
				log.Fatalf("Failed to read proof file %s: %v", *proofFile, err)
			}
			var proof zkp.ZKAttestationProof
			if err := json.Unmarshal(pData, &proof); err != nil {
				log.Fatalf("Invalid proof JSON: %v", err)
			}

			verifyRes, err := zkp.VerifyProof(&proof)
			if err != nil {
				log.Fatalf("Proof verification failed: %v", err)
			}
			fmt.Print(zkp.RenderZKProofScorecard(&proof, verifyRes))

			if *strict && !verifyRes.Valid {
				os.Exit(1)
			}
		default:
			log.Fatalf("Unknown zkp action '%s'. Use 'prove' or 'verify'.", *action)
		}

	case "speculate":
		specCmd := flag.NewFlagSet("speculate", flag.ExitOnError)
		sessionID := specCmd.String("id", "", "Trace Session ID to evaluate speculative execution branches for")
		minConf := specCmd.Float64("min-conf", 0.75, "Minimum confidence threshold for speculative branch commitment")
		maxDepth := specCmd.Int("max-depth", 3, "Maximum forward speculative execution step depth")
		strict := specCmd.Bool("strict", false, "Exit with non-zero exit code if mispredictions occur")
		_ = specCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr speculate -id <session_id> [-min-conf <float>] [-max-depth <n>] [-strict]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		config := speculate.DefaultSpeculateConfig()
		config.MinConfidenceThreshold = *minConf
		config.MaxSpeculativeDepth = *maxDepth

		engine := speculate.NewEngine(config)

		// Simulate speculative branch evaluation across recorded session events
		for i := 0; i < len(session.Events)-1; i += 2 {
			baseEv := session.Events[i]
			nextEv := session.Events[i+1]

			b, err := engine.CreateBranch(session.ID, baseEv.Index, baseEv.Payload)
			if err == nil {
				savedMs := nextEv.DurationMs
				if savedMs <= 0 {
					savedMs = 250
				}
				_ = engine.AddSpeculativeStep(b.BranchID, speculate.SpeculativeStep{
					StepIndex:        nextEv.Index,
					PredictedType:    string(nextEv.Type),
					PredictedPayload: nextEv.Payload,
					ConfidenceScore:  0.88,
					WallClockSavedMs: savedMs,
					Timestamp:        nextEv.Timestamp,
				})

				_, _ = engine.CommitOrRollback(b.BranchID, []trace.TraceEvent{nextEv})

				ctxHash := speculate.ComputeContextHash(session.ID, baseEv.Index, session.Events[:i+1])
				engine.CachePredictiveBranch(ctxHash, string(nextEv.Type), nextEv.Payload)
			}
		}

		report := engine.GenerateSpeculateReport(*sessionID)
		fmt.Print(speculate.RenderSpeculateScorecard(report))

		if *strict && report.RolledBackBranches > 0 {
			os.Exit(1)
		}

	case "tenant":
		tenantCmd := flag.NewFlagSet("tenant", flag.ExitOnError)
		sessionID := tenantCmd.String("id", "", "Trace Session ID to audit for tenant namespace isolation")
		tenantID := tenantCmd.String("tenant", "tenant-default", "Tenant namespace identifier")
		org := tenantCmd.String("org", "Default Organization", "Organization name")
		blockTool := tenantCmd.String("block-tool", "", "Optional tool to block within this tenant namespace")
		strict := tenantCmd.Bool("strict", false, "Exit with non-zero exit code if namespace or RBAC violations occur")
		_ = tenantCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr tenant -id <session_id> [-tenant <name>] [-org <name>] [-block-tool <tool>] [-strict]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		mgr := tenant.NewManager()
		policy := tenant.NamespacePolicy{
			TenantID:          *tenantID,
			Organization:      *org,
			EnforceEncryption: true,
		}
		if *blockTool != "" {
			policy.BlockedToolList = []string{*blockTool}
		}
		if err := mgr.RegisterTenant(policy); err != nil {
			log.Fatalf("Failed to register tenant policy: %v", err)
		}

		report, err := mgr.AuditSession(session, *tenantID)
		if err != nil {
			log.Fatalf("Tenant audit failed: %v", err)
		}

		fmt.Print(tenant.RenderTenantScorecard(report))

		if *strict && (!report.NamespaceIntegrityOK || report.DeniedRequests > 0) {
			os.Exit(1)
		}

	case "gossip":
		gossipCmd := flag.NewFlagSet("gossip", flag.ExitOnError)
		sessionID := gossipCmd.String("id", "", "Trace Session ID to simulate P2P gossip consensus for")
		peers := gossipCmd.Int("peers", 5, "Number of participating ephemeral peer nodes in the gossip mesh")
		strict := gossipCmd.Bool("strict", false, "Exit with non-zero exit code if Byzantine divergence or quorum failure is detected")
		_ = gossipCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr gossip -id <session_id> [-peers <n>] [-strict]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		report := gossip.SimulateSessionGossip(session, *peers)
		fmt.Print(gossip.RenderGossipScorecard(report))

		if *strict && (!report.QuorumIntegrityOK || report.ByzantinePeers > 0) {
			os.Exit(1)
		}

	case "privacy":
		privCmd := flag.NewFlagSet("privacy", flag.ExitOnError)
		sessionID := privCmd.String("id", "", "Trace Session ID to audit for differential privacy and k-anonymity")
		epsilon := privCmd.Float64("epsilon", 1.0, "Differential privacy loss parameter (epsilon ε)")
		k := privCmd.Int("k", 3, "Minimum group threshold for k-anonymity equivalence classes")
		sanitize := privCmd.Bool("sanitize", false, "Generate and save redacted/sanitized session trajectory")
		strict := privCmd.Bool("strict", false, "Exit with non-zero exit code if privacy budget exhausted or k-anonymity failed")
		_ = privCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr privacy -id <session_id> [-epsilon <float>] [-k <n>] [-sanitize] [-strict]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		// Load available fleet traces to evaluate k-anonymity equivalence classes
		var fleet []*trace.Session
		traceIDs, _ := store.ListSessions()
		for _, tid := range traceIDs {
			if tid != *sessionID {
				if s, err := store.LoadSession(tid); err == nil {
					fleet = append(fleet, s)
				}
			}
		}

		cfg := privacy.DefaultDPConfig()
		cfg.Epsilon = *epsilon
		cfg.KAnonymityThreshold = *k
		engine := privacy.NewPrivacyEngine(cfg)

		report := engine.AuditSessionPrivacy(session, fleet, *k)
		fmt.Print(privacy.RenderPrivacyScorecard(report))

		if *sanitize {
			sanitized, maskRep := engine.SanitizeSession(session)
			if err := store.SaveSession(sanitized); err != nil {
				log.Fatalf("Failed to save sanitized session: %v", err)
			}
			fmt.Printf("🔒 [AgentVCR] Sanitized session saved: %s (Redacted %d sensitive tokens)\n\n",
				sanitized.ID, maskRep.TotalTokensMasked)
		}

		if *strict && !report.OverallCompliance {
			os.Exit(1)
		}

	case "vision":
		visCmd := flag.NewFlagSet("vision", flag.ExitOnError)
		sessionID := visCmd.String("id", "", "Trace Session ID to audit for visual state grounding and screen alignment")
		strict := visCmd.Bool("strict", false, "Exit with non-zero exit code if misgrounded actions or visual drift is detected")
		_ = visCmd.Parse(os.Args[2:])

		if *sessionID == "" {
			log.Fatal("Usage: agentvcr vision -id <session_id> [-strict]")
		}

		session, err := store.LoadSession(*sessionID)
		if err != nil {
			log.Fatalf("Error loading session %s: %v", *sessionID, err)
		}

		_, report := vision.SimulateTrajectoryVision(session)
		fmt.Print(vision.RenderVisionScorecard(report))

		if *strict && (!report.Passed || report.MisgroundedActions > 0 || report.VisualDriftDetected) {
			os.Exit(1)
		}

	default:
		printHelp()
	}
}

func printHelp() {
	fmt.Println("AgentVCR — Deterministic Flight Recorder & Time-Travel Debugger for AI Agents")
	fmt.Println("\nCommands:")
	fmt.Println("  record      Start transparent reverse-proxy recording session")
	fmt.Println("  view        Inspect recorded agent trajectory and token events")
	fmt.Println("  fork        Branch an execution trace at an exact step for re-testing")
	fmt.Println("  diff        Compare two trajectories to pinpoint execution divergence")
	fmt.Println("  eval        Run deterministic assertion suites and SLAs on trajectories")
	fmt.Println("  play        Interactive visual terminal playback of agent execution")
	fmt.Println("  snapshot    Capture or restore deterministic workspace filesystem state")
	fmt.Println("  cluster     Fleet-wide trajectory pattern clustering & usage heatmaps")
	fmt.Println("  fuzz        Adversarial prompt injection & fault mutation harness")
	fmt.Println("  serve       Host real-time streaming telemetry dashboard & API")
	fmt.Println("  export      Export session traces to OpenTelemetry (OTel / OTLP JSON)")
	fmt.Println("  sign        Cryptographically sign trajectory with Ed25519 digital signature")
	fmt.Println("  verify      Validate cryptographic attestation & verify zero tampering")
	fmt.Println("  sim         Offline mock tool synthesizer and deterministic synthetic simulator")
	fmt.Println("  bench       Agentic memory regression tester & long-horizon recall benchmarks")
	fmt.Println("  consensus   Multi-agent consensus & Byzantine fault detection replay harness")
	fmt.Println("  forensics   Semantic vector search & RAG hallucination replay forensics")
	fmt.Println("  causal      Cross-trajectory causal DAG synthesizer & automated root-cause repair engine")
	fmt.Println("  policy      Real-time runtime policy governance & zero-trust sandboxing enforcer")
	fmt.Println("  lineage     Ephemeral execution sandboxing & microsecond state lineage graphs")
	fmt.Println("  leaderboard Automated synthetic agent benchmark leaderboard & regression diffing")
	fmt.Println("  throttle    Dynamic latency budget allocator & distributed token rate-limiter")
	fmt.Println("  zkp         Cross-organization zero-knowledge trajectory attestation & zk-proofs")
	fmt.Println("  speculate   Adaptive speculative execution rollback & predictive branch caching")
	fmt.Println("  tenant      Multi-tenant cryptographic namespace isolation & RBAC policy audit")
	fmt.Println("  gossip      Ephemeral P2P agent gossip consensus & distributed Byzantine quorum ledger")
	fmt.Println("  privacy     Distributed differential privacy token masking & k-anonymity auditing")
	fmt.Println("  vision      Multi-modal frame-accurate visual state recording & screen grounding attestation")
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
	fmt.Println("  agentvcr sign -id trace_1 -out trace_1.cert.json")
	fmt.Println("  agentvcr verify -id trace_1 -cert trace_1.cert.json")
	fmt.Println("  agentvcr sim -mode run -id trace_1 -mocks mocks.json")
	fmt.Println("  agentvcr bench -id trace_1 -needle 'AUTH_KEY_123' -intro-step 1 -probe-step 5")
	fmt.Println("  agentvcr consensus -sessions agent_1,agent_2,agent_3 -quorum 0.67")
	fmt.Println("  agentvcr forensics -id trace_1 -threshold 0.35 -strict")
	fmt.Println("  agentvcr causal -id trace_1 -repair")
	fmt.Println("  agentvcr policy -id trace_1 -forbid 'rm -rf' -block-host 169.254.169.254 -strict")
	fmt.Println("  agentvcr lineage -id trace_1 -key auth.token -step 3")
	fmt.Println("  agentvcr leaderboard -sessions agent_1,agent_2,agent_3")
	fmt.Println("  agentvcr leaderboard -diff -baseline trace_1 -candidate trace_2 -strict")
	fmt.Println("  agentvcr throttle -id trace_1 -max-rpm 120 -total-budget 15000 -strict")
	fmt.Println("  agentvcr zkp -action prove -id trace_1 -proof trace_1.zkp.json -zero-pii")
	fmt.Println("  agentvcr zkp -action verify -proof trace_1.zkp.json -strict")
	fmt.Println("  agentvcr speculate -id trace_1 -min-conf 0.80 -strict")
	fmt.Println("  agentvcr tenant -id trace_1 -tenant enterprise_corp -block-tool shell_exec -strict")
	fmt.Println("  agentvcr gossip -id trace_1 -peers 5 -strict")
	fmt.Println("  agentvcr privacy -id trace_1 -epsilon 1.0 -k 3 -sanitize -strict")
	fmt.Println("  agentvcr vision -id trace_1 -strict")
}
