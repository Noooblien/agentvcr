# Architecture Blueprint: AgentVCR

AgentVCR is a deterministic time-travel debugger and flight-recorder designed specifically for autonomous AI agents, multi-agent swarms, and Model Context Protocol (MCP) tool pipelines.

---

## 1. High-Level System Architecture

```
                  ┌────────────────────────────────────────────────────────┐
                  │                   Agent Execution                      │
                  │         (OpenBot / Claude Code / CrewAI / AutoGen)     │
                  └──────────────────────────┬─────────────────────────────┘
                                             │ HTTP / SSE / JSON-RPC
                                             ▼
                  ┌────────────────────────────────────────────────────────┐
                  │                 AgentVCR Reverse Proxy                 │
                  │             (pkg/proxy, pkg/policy, pkg/taint)         │
                  └──────┬───────────────────┬────────────────────┬────────┘
                         │                   │                    │
        [Record Mode]    │                   │   [Replay Mode]    │   [Telemetry]
                         ▼                   ▼                    ▼
             ┌─────────────────────┐  ┌─────────────┐   ┌───────────────────┐
             │ Trajectory Storage  │  │ Mock Server │   │ OpenTelemetry /   │
             │ (zstd / json / sql) │  │  (pkg/sim)  │   │ Jaeger Exporter   │
             └──────────┬──────────┘  └──────┬──────┘   └───────────────────┘
                        │                    │
                        ▼                    ▼
             ┌────────────────────────────────────────────────────────┐
             │               Interactive Time-Travel Player           │
             │         (pkg/tui, pkg/replay, pkg/diff, pkg/lineage)   │
             └────────────────────────────────────────────────────────┘
```

---

## 2. Core Execution Pipeline

1. **Interception Layer (`pkg/proxy`)**:
   - Acts as a local reverse proxy or transparent TCP/HTTP hook between the agent runtime and upstream providers (OpenAI, Anthropic, Ollama, MCP tool daemons).
   - Captures requests and responses without introducing latency bottlenecks.

2. **Sanitization & Policy Enforcement (`pkg/policy`, `pkg/taint`)**:
   - Inspects headers and payload bodies for credentials (`Bearer`, `Authorization`, API keys), automatically replacing them with redaction placeholders.
   - Evaluates outbound URLs and tool arguments against SSRF rules and forbidden system calls.

3. **Storage & Serialization (`pkg/trace`, `pkg/storage`)**:
   - Normalizes all interactions into an immutable sequence of `TraceEvent` records within a `Session`.
   - Compresses data on the fly using magic-header-detected streaming compression.

4. **Replay & Time-Travel Engine (`pkg/replay`, `pkg/tui`)**:
   - Replays captured sessions step-by-step with zero external API calls.
   - Supports instant trajectory branching (`InteractivePlayer.ForkAtCurrentStep`), allowing developers to fork execution from step $N$ to test prompt mutations or tool failures.

5. **Distributed Fleet Telemetry (`pkg/otel`)**:
   - Emits W3C-compliant distributed trace spans (`traceparent`).
   - Correlates multi-agent hierarchical swarms into unified 128-bit trace trees.
