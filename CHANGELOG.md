# Changelog

All notable changes to **AgentVCR** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- **Speculative Multi-Token AST Predictor & Fast-Tab Engine (`pkg/speculate`)**: Reverse-engineered Cursor-style sub-50ms multi-line code edit predictor using syntactic signature matching, dynamic pattern registration, and typing speed time-savings estimator.
- **Fast-Tab Interactive CLI Demonstration (`agentvcr speculate -fast-tab`)**: Instant terminal preview of speculative multi-line syntax drafts with line-level diff highlighting and confidence metrics.
- **Real-Time Telemetry Event Hub & Streaming Flight Controller (`pkg/dashboard`)**: In-memory Pub/Sub fan-out telemetry bus with channel-based subscriber multiplexing and SSE live broadcast streaming (`/api/stream/events`).
- **Interactive REST Fork & Time-Travel Controller (`/api/fork`)**: HTTP-controlled step-level trajectory branching API enabling external web UI consoles to duplicate, mutate, and branch running agents on the fly.
- **Flight Control Web Console & Modern UI Overhaul**: Zero-dependency dark-mode web console with real-time SSE event feed, fleet token/cost metrics, and trace inspection links.
- **Sub-Millisecond Latency Benchmarking & Profiling Engine (`pkg/perf`)**: High-precision monotonic timing suite measuring in-flight proxy overhead, MCP frame interception, CoW snapshot branching/rollback, context compaction, and JSON storage serialization with P50/P90/P99/P99.9 percentiles and memory allocation tracking.
- **Concurrency Load & Stress Runner (`pkg/perf/stress.go`)**: Multi-goroutine worker pool simulating high-concurrency bursty agent workloads with lock contention and error rate tracking.
- **Executive Performance Scorecard (`pkg/perf/scorecard.go`)**: Rich ANSI terminal scorecard rendering percentile tables, ops/sec throughput, SLA thresholds, and memory allocations.
- **Go Native Continuous Fuzz Testing (`tests/fuzz_target_test.go`)**: Coverage-guided fuzzing targets (`FuzzMCPJSONRPCFrame`, `FuzzSessionDeserialization`, `FuzzCompactorDeduplication`) protecting against malformed frames, corrupted sessions, and parser panics.
- **Context Window Compactor & Token Waste Profiler (`pkg/compact`)**: Lossless deduplication of redundant tool outputs via content hashing, automatic collapsing of intermediate retry chains, and stale read pruning.
- **Upstream KV-Cache Prefix Optimizer**: Reorganizes dynamic/volatile prompt components to guarantee high cache hit ratios on Anthropic prompt caching and OpenAI prefix caching.
- **Bidirectional Model Context Protocol (MCP) Interceptor (`pkg/mcp`)**: Real-time stdio/SSE JSON-RPC 2.0 frame capture, canonical SHA-256 argument hashing, and monotonic latency telemetry.
- **Deterministic Virtual MCP Server & Replayer**: Zero-side-effect mock tool replay matching recorded argument digests, supporting configurable simulated latency and synthetic fault injection.
- **MCP Argument Schema & Prompt Injection Guard (`pkg/mcp/validator.go`)**: Pre-execution argument type checking and OWASP-LLM prompt injection defense with formatted terminal scorecards.
- Docker & Copy-on-Write (CoW) layered filesystem snapshot driver with OCI whiteouts and $O(1)$ trajectory branching.
- Terminal TUI visual color themes (Monokai, Dracula, Gruvbox, Nord) and interactive vi keybindings (`j`/`k`/`f`/`t`/`?`/`q`).
- Instant trajectory branching at any step index preserving causal lineage metadata.
- Asynchronous OpenTelemetry OTLP collector exporter with background worker queue, exponential backoff, and W3C traceparent context.
- Multi-agent hierarchical span correlator unifying distributed subagent spans under single root trace trees.
- Transparent streaming trace compression with magic header detection (`zstd`/`gzip`).
- Markov transition behavioral drift detector and Shannon entropy policy synthesizer.
- Dynamic Information Flow Control (IFC) taint tracking with declassification gates.

---

## [v1.0.0] - 2026-10-16

### Added
- **Core Flight Recorder**: Zero-overhead reverse proxy interceptor for Anthropic Claude, OpenAI, and local LLM endpoints.
- **Model Context Protocol (MCP) Interceptor**: Real-time JSON-RPC frame interception for tool-calls and tool-results.
- **Deterministic Replay Engine**: Time-travel step-through debugging, snapshot rollbacks, and offline playback.
- **Security & Red Teaming**: Automated prompt injection test suite, secret masking, and SSRF guardrails.
- **Formal Verification**: TLA+ temporal logic safety specifications and state machine validators.
- **Cryptographic Attestations**: Post-quantum Kyber/Dilithium hybrid signatures and zero-knowledge proof verifiers.
