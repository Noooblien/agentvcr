# Contributing to AgentVCR

Thank you for your interest in contributing to **AgentVCR**! We welcome contributions from the community to make autonomous agent observability, execution replay, and security testing more robust.

---

## Development Setup

1. **Prerequisites:**
   - Go 1.21 or higher
   - Git

2. **Clone and Build:**
   ```bash
   git clone https://github.com/Noooblien/agentvcr.git
   cd agentvcr
   go build -o bin/agentvcr ./cmd/agentvcr
   ```

3. **Run Test Suite:**
   ```bash
   go test -v -race ./...
   ```

---

## Branching & Pull Request Workflow

AgentVCR protects the `main` branch to guarantee production stability:

1. Create a dedicated feature branch from `main`:
   ```bash
   git checkout -b feat/your-feature-name
   ```
2. Write clean, self-documenting code with unit tests in `tests/`.
3. Ensure all tests pass: `go test -v ./...`.
4. Commit using standard conventional commits:
   - `feat(...)`: New capability
   - `fix(...)`: Bug fix
   - `perf(...)`: Performance optimization
   - `sec(...)`: Security hardening
   - `docs(...)`: Documentation improvements
5. Push to your branch and open a Pull Request.

---

## Code of Conduct

We are committed to providing a welcoming, inclusive, and harassment-free environment for all contributors.
