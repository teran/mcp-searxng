# Security Policy

This MCP server is developed and tested largely by AI agents with minimal human
review, so security is enforced automatically by CI (lint, coverage, mutation,
gosec, govulncheck, gitleaks). The security baseline is described in `SPEC.md`
(S01–S14) — the single source of truth for the security contracts this
repository must honour.

## Reporting a Vulnerability

Please **do not** open a public issue for a security vulnerability.

Report vulnerabilities privately to the maintainers using GitHub's private
security advisory workflow:
<https://github.com/teran/mcp-searxng/security/advisories/new>.

Please include:

- the affected version(s) and how you detected the issue;
- a minimal, reproducible description of the vulnerability;
- the potential impact and, if known, a suggested fix;
- the `ALLOW_DIRS` / configuration context in which it occurs (if relevant).

## What happens next

1. The maintainers acknowledge the report within **5 business days**.
2. The issue is triaged: severity, affected versions, and a fix plan.
3. A fix is developed **test-first** and lands through the normal CI gates.
4. Once a fix is available, the vulnerability is disclosed and (if appropriate)
   a `v*` release is published.

We ask that you **do not** disclose the vulnerability publicly until a fix has
been released or the maintainers explicitly agree otherwise (responsible
disclosure).

## Supported Versions

| Version | Supported          |
|---------|--------------------|
| latest (e.g. 1.x) | ✅ |

Only the latest release line is supported. Older versions are not patched;
upgrade to a supported version.

## Security Baseline

- **TLS** is never implemented inside the server — it is terminated at the
  reverse proxy.
- **Secrets** are never leaked into tool output or logs (annotation-based
  redaction, `secret:true`).
- **Filesystem** access is scoped to `ALLOW_DIRS` (with `ALLOW_SYMLINKS=false`
  by default); anything resolving outside the scope is rejected.
- **Open-world tools** treat their output as untrusted data and are isolated
  from `ALLOW_DIRS`/secrets.
- **Destructive tools** require human confirmation (HITL).

See `SPEC.md` for the full domain-specific security audit (S14).
