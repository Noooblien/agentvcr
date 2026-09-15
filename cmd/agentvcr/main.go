package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Noooblien/agentvcr/pkg/proxy"
	"github.com/Noooblien/agentvcr/pkg/replay"
	"github.com/Noooblien/agentvcr/pkg/storage"
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

	default:
		printHelp()
	}
}

func printHelp() {
	fmt.Println("AgentVCR — Deterministic Flight Recorder & Time-Travel Debugger for AI Agents")
	fmt.Println("\nCommands:")
	fmt.Println("  record  Start transparent reverse-proxy recording session")
	fmt.Println("  view    Inspect recorded agent trajectory and token events")
	fmt.Println("  fork    Branch an execution trace at an exact step for re-testing")
	fmt.Println("\nExamples:")
	fmt.Println("  agentvcr record --port 8088 --target https://api.openai.com")
	fmt.Println("  agentvcr view -id trace_1760800000")
	fmt.Println("  agentvcr fork -id trace_1760800000 -step 4")
}
