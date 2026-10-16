# AgentVCR

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
[![Go Report Card](https://goreportcard.com/badge/github.com/Noooblien/agentvcr)](https://goreportcard.com/report/github.com/Noooblien/agentvcr)

> **The Flight Recorder and Time-Travel Debugger for Autonomous AI Agents.**

When an autonomous agent fails a 50-step trajectory, hallucinating a parameter or corrupting state at step 23, developers currently face an un-reproducible wall of logs. 

**AgentVCR brings deterministic flight recording, execution replay, and time-travel branching to agentic AI.**

---

## Architecture Overview

```mermaid
flowchart LR
    A["Agent Runtime<br/>(LangGraph / AutoGen / OpenBot)"] -->|"LLM & Tool Calls"| P["AgentVCR Proxy<br/>(:8088)"]
    P -->|"Live Forwarding"| U["Upstream Model<br/>(OpenAI / Anthropic / Ollama)"]
    P -->|"Append Event + Checksum"| S[("VCR Trace Store<br/>.vcr_traces/*.vcr.json")]
    S -->|"Replay / Audit"| R["AgentVCR Engine"]
    R -->|"Fork & Mutate Step N"| B["Branch Trajectory"]
```

---

## Key Features

- **Transparent Low-Overhead Proxy:** Zero code changes required in your agent—point your SDK's `base_url` to `http://localhost:8088/v1`.
- **Deterministic Trajectory Recording:** Records every prompt token, completion, tool invocation argument, and system response with monotonic timestamps and payload hashing.
- **Time-Travel Forking (`fork`):** Branch past execution traces from any arbitrary step ($N$). Mutate the prompt or mock the tool response and re-run without re-executing steps $1 \dots N-1$.
- **Audit & Step-by-Step Viewer (`view`):** Inspect the exact state of the agent context window at any microsecond.

---

## Quick Start

### 1. Installation

```bash
git clone https://github.com/Noooblien/agentvcr.git
cd agentvcr
go build -o bin/agentvcr ./cmd/agentvcr
```

### 2. Start Flight Recorder

```bash
./bin/agentvcr record --port 8088 --target https://api.openai.com
```

### 3. Point your Agent to AgentVCR

In Python (OpenAI SDK):
```python
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8088/v1",
    api_key="your-api-key"
)
```

### 4. Inspect or Fork a Trajectory

```bash
# List and inspect all events in a session
./bin/agentvcr view -id trace_1760800000

# Fork from step 5 to branch execution and test alternative prompt paths
./bin/agentvcr fork -id trace_1760800000 -step 5
```

---

## Roadmap

- [x] High-performance reverse-proxy event streaming engine
- [x] Monotonic trajectory storage & JSON serialization
- [x] Step-level execution branching (`fork`)
- [x] Model Context Protocol (MCP) tool-call interceptor
- [x] Token consumption & cost metrics engine
- [x] Trajectory diffing & step divergence detection (`diff`)
- [x] Deterministic trajectory assertion & CI/CD SLA engine (`eval`)
- [x] Interactive visual terminal playback engine (`play`)
- [x] Deterministic workspace filesystem snapshotting & rollback (`snapshot`)
- [x] Distributed trace clustering and multi-agent aggregate heatmaps (`cluster`)
- [x] Multi-turn fuzzing & adversarial prompt mutation harness (`fuzz`)
- [x] Real-time streaming telemetry dashboard & HTTP API (`serve`)
- [x] Distributed multi-node agent mesh tracing with OpenTelemetry export (`export`)
- [x] Cryptographic trajectory signing & immutable tamper-proof attestation (`verify`)
- [x] WASM-sandboxed mock tool synthesizers & offline synthetic simulator (`sim`)
- [x] Agentic memory regression tester & long-horizon recall benchmarks (`bench`)
- [x] Multi-agent consensus & Byzantine fault detection replay harness (`consensus`)
- [x] Semantic vector search & RAG hallucination replay forensics (`forensics`)
- [x] Cross-trajectory causal DAG synthesizer & automated root-cause repair engine (`causal`)
- [x] Real-time runtime policy governance & zero-trust sandboxing enforcer (`policy`)
- [x] Ephemeral execution sandboxing & microsecond state lineage graphs (`lineage`)
- [x] Automated synthetic agent benchmark leaderboard & regression diffing (`leaderboard`)
- [x] Dynamic latency budget allocator & distributed token rate-limiter (`throttle`)
- [x] Cross-organization zero-knowledge trajectory attestation & cryptographic zk-proofs (`zkp`)
- [x] Adaptive speculative execution rollback & predictive branch caching (`speculate`)
- [x] Multi-tenant cryptographic namespace isolation & RBAC policy audit (`tenant`)
- [x] Ephemeral P2P agent gossip consensus & distributed Byzantine quorum ledger (`gossip`)
- [x] Distributed differential privacy token masking & k-anonymity auditing (`privacy`)
- [x] Multi-modal frame-accurate visual state recording & screen grounding attestation (`vision`)
- [x] Ephemeral eBPF kernel socket & system-call interception recorder (`ebpf`)
- [x] Canary deployment traffic shadowing & live execution chaos injection (`canary`)
- [x] Post-quantum ML-KEM & Dilithium cryptographic trajectory attestation (`pqc`)
- [x] Multi-region active-active trajectory replication & Raft ledger sync (`raft`)
- [x] Autonomous self-healing trajectory patch synthesizer & AST repair engine (`heal`)
- [x] Confidential computing hardware enclave attestation & AMD SEV-SNP verification (`enclave`)
- [x] Automated prompt-leakage side-channel analyzer & differential cache-timing forensics (`timing`)
- [x] Ephemeral zero-dependency WebAssembly WASI agent isolation sandbox (`wasi`)
- [ ] Autonomous synthetic multi-agent red-team prompt injection harness (`redteam`)

---

## License

[MIT](LICENSE) © 2026 [Rahul Singh Maraskole](https://github.com/Noooblien)
