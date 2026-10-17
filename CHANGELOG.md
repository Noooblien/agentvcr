# Changelog

All notable changes to **AgentVCR** will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Added
- **FinOps Token Budget Circuit Breaker & Runaway Cost Protector (`pkg/finops`)**: Real-time microsecond spend velocity and token accounting guardrail preventing runaway agent loops, recursive API billing explosions, and budget overruns.
- **Model Rate Cards & Precision Microdollar Pricing**: Real-time pricing calculator modeling input, output, and cached input token rates across OpenAI (`gpt-4o`, `gpt-4o-mini`, `o1`) and Anthropic Claude (`claude-3-7-sonnet`, `claude-3-5-haiku`).
- **Defensive Multi-Threshold Enforcement**: Configurable policy tiers (`ALLOW`, `WARN`, `THROTTLE`, `DOWNGRADE`, `HARD_KILL`) with automated model downgrading (e.g. `gpt-4o` $\to$ `gpt-4o-mini`) when approaching budget limits.
- **Sliding-Window Burn-Rate Anomaly Detection**: Tracks velocity of token spend ($/minute) to intercept runaway loops and recursive recursion bugs before credit limits are exhausted.
- **Interactive FinOps Terminal Scorecard & Mermaid Visualizer (`agentvcr finops`)**: Rich ANSI terminal dashboard tracking agent spend, utilization, burn rates, and circuit breaker trip reasons with Mermaid topology diagrams.
- **Autonomous Hierarchical Memory Consolidation & Ebbinghaus Forgetting Curves (`pkg/memory`)**: Multi-tiered agent memory engine that consolidates raw episodic turns into synthesized reflection clusters and extracts permanent semantic knowledge graph triples, preventing prompt context window blowups and token waste.
- **Ebbinghaus Decay Curve & Access Reinforcement**: Exponential forgetting function $R(t) = S \cdot e^{-\lambda \Delta t} + \alpha \cdot \ln(1 + \text{AccessCount})$ that continuously prunes stale ephemeral command outputs while reinforcing accessed memories with stability multipliers.
- **Semantic Knowledge Graph & Contradiction Resolution**: Extracts structured knowledge triples (Subject-Predicate-Object) for architectural decisions and user preferences, automatically resolving contradictory facts when preferences evolve.
- **Token-Budgeted Context Recall**: Score-ranked memory retrieval combining relevance, retention, and intrinsic importance, strictly bound to a designated token ceiling for seamless prompt context injection.
- **Interactive Memory Scorecard & Hierarchy Visualizer (`agentvcr memory`)**: ANSI terminal scorecard tracking active nodes, token compression ratios, retention rankings, and Mermaid memory hierarchy export.
- **Model Gateway, Circuit Breaker & Semantic Multi-Provider Router (`pkg/gateway`)**: Production-grade LLM routing gateway featuring sliding-window circuit breakers, multi-protocol normalization (OpenAI, Anthropic, Gemini, Ollama), and automatic failover cascades.
- **Sliding-Window Circuit Breakers & Self-Healing Probes**: Thread-safe per-provider circuit breakers with configurable failure thresholds, rolling error rates, cooldown durations, and half-open canary probes for graceful degradation under rate limits (HTTP 429) or upstream outages.
- **Semantic Task Complexity Classifier & Tiered Routing**: Automatic intent analysis estimating token length, tool calling requirements, and mathematical/reasoning keywords to route queries to optimal model tiers (`TierFast`, `TierStandard`, `TierReasoning`).
- **Deterministic Semantic Replay Cache**: SHA-256 fingerprinting of normalized prompt messages, tools, and temperature, enabling sub-millisecond 0ms instant cache hits with zero dollar cost on repeat agent turns.
- **Multi-Protocol Translation Layer**: Dynamic bidirectional translation across OpenAI Chat Completion format, Anthropic Messages API, and Google Gemini format without modifying agent SDK code.
- **Interactive Gateway CLI & Topology Exporter (`agentvcr gateway`)**: Rich ANSI terminal scorecard tracking real-time provider traffic, circuit states, failover counts, and cost savings, paired with automatic Mermaid routing diagram export.
- **Distributed Agent Actor Mesh & Supervision Tree Runtime (`pkg/actor`)**: Erlang/OTP-inspired fault-tolerant actor runtime providing concurrent mailboxes, priority message queuing, supervisor trees (`OneForOne`, `OneForAll`, `RestForOne`), and autonomous crash recovery for multi-agent swarms.
- **Hierarchical Supervision Strategies & Restart Windows**: Automatic panic and runtime error containment with configurable backoff policies (`MaxRestarts` within `TimeWindow`) and graceful failure escalation.
- **Dead-Letter Office & Priority Mailbox Queuing**: Thread-safe mailbox with priority queues (`PrioritySystem`, `PriorityHigh`, `PriorityNormal`, `PriorityLow`), bounded overflow handling (`DropOldest`, `DropNewest`, `DeadLetter`), and unroutable message journals.
- **Mermaid Supervision Tree & Interactive Scorecard (`agentvcr actor`)**: Rich ANSI terminal scorecard with live processor telemetry, average latency percentiles, and automatic Mermaid flowchart export of active actor hierarchies.
- **Deterministic Container Sandbox Execution Hypervisor & CoW Overlay Rollback (`pkg/sandbox`)**: Isolated execution container hypervisor managing cgroup quotas (CPU time limit, RSS memory ceiling, max PIDs, max disk writes), network egress policies (AllowList/DenyList), and command flight recording for autonomous coding agents.
- **Copy-on-Write (CoW) Overlay Filesystem**: Multi-layered in-memory ephemeral filesystem supporting baseline read-only images, upper write deltas, file whiteouts/tombstones, and microsecond $O(1)$ checkpoint rollback without disk churn.
- **Autonomous Jailbreak & Escape Guardrails**: Real-time pre-execution pattern detection intercepting Cloud Metadata SSRF (`169.254.169.254`), privileged kernel escapes (`/proc/kcore`, `/sys/kernel`, `nsenter`), credential theft (`.ssh/id_rsa`, `/etc/shadow`), fork-bombs, and destructive wipes (`rm -rf /`).
- **Interactive Sandbox CLI & Terminal Scorecard (`agentvcr sandbox`)**: Rich ANSI terminal scorecard visualizing container status, resource telemetry, execution timeline, CoW file mutations, and security SLA compliance.
- **Database Transaction Flight Recorder & Semantic CDC Rollback Engine (`pkg/cdc`)**: Real-time transaction wire-protocol and change-data-capture journal for PostgreSQL, SQLite, DuckDB, and Chroma vector databases.
- **Automated Inverse Compensating SQL Synthesis**: Reconstructs exact inverse SQL mutations (`DELETE` for un-insert, `INSERT` with preserved 'Before' state for un-delete, `UPDATE` restoring previous column values with SQL escaping) for atomic compensation without restoring full database backups.
- **Dangerous Statement & Unbounded Query Safety Guards**: Pre-execution regex detection flagging dangerous agent hallucinations including unbounded `DELETE`/`UPDATE` without `WHERE` clauses and destructive DDL drops (`DROP TABLE`, `DROP DATABASE`, `TRUNCATE`).
- **Vector Store Transaction Journaling**: Tracks high-dimensional vector embeddings, dimensional consistency, collection upserts, and poisoned embedding mutations.
- **Time-Travel Step Rollback & Terminal Scorecard (`agentvcr cdc`)**: Synthesizes reverse-chronological transaction rollback scripts starting from any trajectory step index (`-rollback-step <n>`) with ANSI scorecard visualization.
- **StateGraph Workflow Engine & Checkpoint Time-Travel Debugger (`pkg/graph`)**: Native LangGraph/StateGraph cyclic multi-agent graph runtime featuring typed state channels, custom state reducers (`ReducerAppendList`, `ReducerMergeMap`, `ReducerOverwrite`), conditional edge routers, and cycle loop detection.
- **Microsecond Checkpoint Journal & State Rewind**: Immutable step-by-step checkpoint snapshots allowing backward time-travel, thread state branching, and retroactive state injection resumes.
- **Human-in-the-Loop Interrupt Gates (`InterruptBefore`, `InterruptAfter`)**: Configurable pause gates halting execution before or after sensitive nodes (e.g. production deployers) for human approval or parameter mutation.
- **Recursion Limit Guards & Cycle Detection**: Configurable recursion depth limiter preventing infinite agent loops with cycle counters.
- **Mermaid Flowchart Generator & Terminal Scorecard (`agentvcr graph`)**: Interactive ANSI terminal scorecard paired with dynamic Mermaid flowchart DAG visualization highlighting executed path transitions.
- **Chrome DevTools Protocol (CDP) Flight Recorder & Virtual DOM Journal (`pkg/cdp`)**: Bidirectional JSON-RPC 2.0 WebSocket frame interceptor capturing page navigation, network waterfalls, DOM mutations, console logs, runtime exceptions, and agent click/type interactions.
- **W3C HAR 1.2 Export & Network Waterfall Analytics**: Spec-compliant HTTP Archive (HAR 1.2) export with detailed connection timings (DNS, Connect, SSL, TTFB, Download), transfer sizes, and P50/P95 latency percentiles.
- **In-Memory Virtual DOM Reconstruction & Fast QuerySelector**: Real-time virtual DOM tree synchronization tracking node insertions, attribute modifications, removals, and CSS selector matching (`#id`, `.class`, `tag`).
- **Deterministic Virtual Mock CDP Server (`pkg/cdp/mock.go`)**: Zero-side-effect headless mock CDP endpoint enabling offline agent replay, mock DOM queries, and synthetic JS evaluations without launching Chromium.
- **Browser Agent SLA Auditor & Terminal Scorecard (`agentvcr cdp`)**: Interactive ANSI terminal scorecard validating page load times, 4xx/5xx HTTP errors, and uncaught JavaScript runtime exceptions.
- **Real-Time Voice API Flight Recorder & Acoustic Waveform Synchronizer (`pkg/voice`)**: Real-time dual-track flight recorder capturing bidirectional WebSocket/WebRTC audio streams for OpenAI Realtime and Gemini Live Voice APIs with microsecond timestamp synchronization.
- **Audio Energy & Decibel Telemetry**: 16-bit PCM Linear LE RMS calculation, decibel sound pressure level ($dB_{FS}$) tracking, and 8-level Unicode acoustic amplitude waveform rendering (` ▂▃▄▅▆▇█`).
- **Energy-Based Voice Activity Detection (VAD)**: Multi-packet speech boundary segmentation detecting user/agent voice onsets and trailing speech pauses with configurable decibel thresholds.
- **Turnaround Latency & Interruption Barge-In SLA Auditor**: Sub-millisecond tracking of user-to-agent turnaround latency (<800ms) and user barge-in interruption cutoff latency (<350ms) with P99 jitter analytics.
- **RIFF/WAVE 44-Byte Audio Synthesis & Terminal Scorecard (`agentvcr voice`)**: Automatic zero-dependency WAV header generation for exporting synchronized audio tracks, paired with an interactive ANSI multi-track timeline scorecard.
- **In-Browser Console Log Loopback & Hot-Patch Engine (`pkg/heal`)**: Reverse-engineered Lovable/Bolt.new-style autonomous self-healing runtime intercepting browser console errors (`ReferenceError`, `TypeError`, React hydration failures) and bundler HMR diagnostics (Vite/Next.js/Webpack).
- **Zero-Error Convergence Loop (`agentvcr heal -loopback`)**: Multi-round automated diagnostic extraction, stack frame file/line parsing, and hot-patch code synthesizer that iteratively patches code files until 0 browser errors remain.
- **Language Server Protocol (LSP 3.17) Daemon & Fast-Tab Engine (`pkg/lsp`)**: Native JSON-RPC 2.0 language server daemon operating over stdio and TCP sockets for direct Neovim, VSCode, and Zed editor integration.
- **In-Memory Document Synchronization Store**: Multi-buffer state manager supporting full (`didOpen`, `didClose`) and incremental (`didChange`) edit ranges with automatic syntax context extraction.
- **Speculative Completion & Custom Fast-Tab RPC (`agentvcr/fastTab`)**: Sub-millisecond predictive completion delivering multi-line ghost text suggestions, typing speed estimators, and live AgentVCR flight attestation telemetry over `textDocument/hover`.
- **LSP Terminal Scorecard & Interactive Demo (`agentvcr lsp --demo`)**: Rich ANSI terminal demonstration displaying active capabilities, buffer tracking, and speculative virtual ghost text diff preview.
- **Recursive Multi-Agent Research Engine & Grounded Synthesis Tree (`pkg/research`)**: Reverse-engineered Perplexity / OpenAI Deep Research-style recursive goal decomposition into structured hypothesis nodes, multi-source evidence extraction (local code, docs, trace sessions, web), cryptographic SHA-256 citation digests, and hallucination pruning.
- **Cross-Source Contradiction Resolution & Conflict Forensics**: Automated semantic conflict detection between contradictory assertions across heterogeneous documents with severity scoring and confidence-weighted resolution strategies.
- **Mermaid Research Tree Visualizer & Executive Brief Generator (`agentvcr research`)**: Interactive terminal scorecard with confidence progress bars, dynamic markdown footnotes (`[^CIT-1]`), and automatic Mermaid flowchart DAG export.
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
