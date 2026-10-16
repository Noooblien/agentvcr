# AGENT.md: Autonomous AI Agent Engineering Guide

Welcome, AI Agent. This document outlines the engineering principles, architectural invariants, and contribution standards for building and maintaining **AgentVCR**.

AgentVCR is the open-source flight-recorder, time-travel debugger, and deterministic replay engine for autonomous AI agents and Model Context Protocol (MCP) toolchains.

---

## 1. Core Operating Principles

1. **Standard Library Purity**:
   - AgentVCR maintains a zero-external-dependency runtime ethos wherever possible (`go 1.21+` standard library).
   - Cryptographic primitives, serialization, HTTP/gRPC adapters, and concurrency primitives must be clean, maintainable, and robust without dependency bloat.

2. **Deterministic Reproducibility**:
   - Every agent interaction (LLM prompt/completion, MCP frame, filesystem mutation, external API call) is an immutable event in the trajectory log.
   - Timestamps, duration metrics, token usage, and payloads must be preserved exactly as captured.

3. **Strict Branch Protection & Git Hygiene**:
   - **`main` is protected.** Never attempt a direct push to `main`.
   - Always branch: `feat/<name>`, `fix/<name>`, `perf/<name>`, or `docs/<name>`.
   - Write semantic commits following Conventional Commits (`feat(scope): ...`, `fix(scope): ...`, `test(scope): ...`).
   - Create a Pull Request via GitHub CLI (`gh pr create`), verify tests pass, and squash-merge (`gh pr merge <num> --squash --delete-branch`).

4. **Test-First Integrity**:
   - No code is merged without dedicated unit and integration tests located in `tests/`.
   - Run `go test -v ./...` before opening any PR. Every test must pass cleanly.

---

## 2. Directory Layout & Subsystem Architecture

```
agentvcr/
├── pkg/
│   ├── attest/        # Cryptographic attestation, HMAC-SHA256 & Ed25519 trace signing
│   ├── bench/         # Latency, throughput, and memory regression benchmarks
│   ├── byzantine/     # Multi-agent consensus, Byzantine fault recovery
│   ├── canary/        # Autonomous canary deployments and traffic shadowing
│   ├── causal/        # Causal DAG dependency graphs of agent actions
│   ├── cluster/       # Multi-node distributed trace aggregation
│   ├── diff/          # Semantic AST & token diffing between agent trajectories
│   ├── drift/         # Markov behavioral drift detector & policy synthesizer
│   ├── ebpf/          # Kernel-level socket & syscall interception telemetry
│   ├── eval/          # Automated safety, hallucination, and alignment evals
│   ├── heal/          # Autonomous self-healing runtime repair loops
│   ├── lineage/       # Causal time-travel provenance and state reconstruction
│   ├── metrics/       # Token consumption, cost accounting, and latency metrics
│   ├── otel/          # OpenTelemetry OTLP exporter, W3C traceparent, fleet correlator
│   ├── policy/        # RBAC, SSRF defense, and tool-call sandbox policies
│   ├── pqc/           # Post-quantum cryptography (Kyber/Dilithium hybrid wrappers)
│   ├── privacy/       # Token masking, Laplace differential privacy, k-anonymity
│   ├── proxy/         # Reverse proxy interceptor for Anthropic/OpenAI/MCP APIs
│   ├── raft/          # Raft consensus cluster for distributed trace ledgers
│   ├── redteam/       # Autonomous adversarial red-teaming & prompt jailbreak suite
│   ├── replay/        # Deterministic offline mock replay engine
│   ├── sim/           # Mock environment simulator for tool responses
│   ├── snapshot/      # Ephemeral filesystem CoW snapshotting and rollback
│   ├── speculate/     # Speculative parallel trajectory execution & branch rollbacks
│   ├── stego/         # Steganography & covert channel detection in tool calls
│   ├── storage/       # Streaming compression engine (Zstd/magic headers)
│   ├── taint/         # Information flow control, source-to-sink taint tracking
│   ├── tenant/        # Multi-tenant isolation and cryptographic key derivation
│   ├── throttle/      # Sliding-window rate limiters, token bucket algorithms
│   ├── timing/        # Constant-time trajectory verification & KV-cache side-channels
│   ├── tlaplus/       # TLA+ formal verification state-machine specifications
│   ├── trace/         # Canonical execution trace schema and JSON/binary encoders
│   ├── tui/           # Terminal UI visual player, vi keybindings, custom themes
│   ├── vision/        # Perceptual hash and visual grounding attestations
│   ├── wasi/          # WebAssembly / WASI tool execution sandboxing
│   └── zkp/           # Zero-knowledge proof verification of trace execution
├── tests/             # End-to-end integration and unit tests
├── .github/           # GitHub Actions CI workflows and PR templates
├── AGENT.md           # This autonomous engineer specification
├── CONTRIBUTING.md    # Human and agent contribution guidelines
├── SECURITY.md        # Vulnerability disclosure and security policy
├── CHANGELOG.md       # Version history and release notes
└── README.md          # Project homepage and quickstart guide
```

---

## 3. Essential Commands

| Task | Command |
|---|---|
| **Run all tests** | `go test -v ./...` |
| **Run tests without cache** | `go test -count=1 -v ./...` |
| **Run specific package test** | `go test -v ./tests -run Test<Name>` |
| **Run benchmarks** | `go test -bench=. ./tests` |
| **Check git status** | `git status` |
| **Create branch** | `git checkout -b <branch-name>` |
| **Create PR** | `gh pr create --base main --head <branch> --title "..." --body "..."` |
| **Squash & Merge PR** | `gh pr merge <pr_number> --squash --delete-branch` |

---

## 4. Coding Standards

- **Error Handling**: Always return contextual wrapped errors using `fmt.Errorf("operation failed: %w", err)`. Never ignore errors.
- **Concurrency**: Guard shared state with `sync.RWMutex` or atomic primitives (`sync/atomic`). Avoid goroutine leaks by always propagating `context.Context` and closing channels properly.
- **Memory Efficiency**: Avoid unnecessary heap allocations in hot paths (e.g. proxying or trace stream encoding). Pre-allocate slices with capacities where known.
- **Docs & Comments**: Exported types and functions must have descriptive GoDoc comments explaining their purpose and edge cases.
