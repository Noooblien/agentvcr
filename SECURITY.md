# Security Policy

## Supported Versions

AgentVCR is maintained actively. Security updates and patches are applied to the following versions:

| Version | Supported          |
| ------- | ------------------ |
| v1.0.x  | :white_check_mark: |
| < v1.0  | :x:                |

---

## Reporting a Vulnerability

The AgentVCR team takes security seriously. If you discover a vulnerability or potential security risk in AgentVCR:

1. **Do not open a public issue.**
2. Send a detailed report to the security team at **`security@noooblien.com`** or submit a private security advisory on GitHub.
3. Include:
   - Steps to reproduce the vulnerability.
   - Proof of Concept (PoC) code or sample trajectory file.
   - Impact assessment (e.g., unauthorized trace extraction, sandbox bypass, SSRF).

### Response SLA
- **Initial Acknowledgement**: Within 24 hours.
- **Triage & Severity Assessment**: Within 48 hours.
- **Fix & Coordinated Disclosure**: Typically within 7 to 14 days depending on complexity.

---

## Security Architecture & Best Practices

- **Zero-Storage Secrets**: Trajectory recordings automatically scrub authorization headers (`Bearer`, `x-api-key`, passwords) using `pkg/policy`.
- **SSRF Protection**: Reverse proxy interceptors enforce strict CIDR allowlists, preventing agents from issuing requests to private or loopback ranges unless explicitly allowed.
- **Cryptographic Attestations**: All recordings can be tamper-evident hashed using SHA-256 and signed with Ed25519 (`pkg/attest`).
