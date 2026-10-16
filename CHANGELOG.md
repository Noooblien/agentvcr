# Changelog

All notable changes to **AgentVCR** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
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
