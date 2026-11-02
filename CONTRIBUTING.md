# Contributing

Thanks for your interest in contributing! Please read this guide and the
repository's `AGENTS.md` and `SPEC.md` before opening a pull/merge request.

> `SPEC.md` and `AGENTS.md` are strictly English; the README is English by
> default unless you explicitly request another language.

## How to contribute

- **Report a bug** — open an issue. Include the exact command/arguments, the
  expected and actual behaviour, and (if relevant) logs **without secrets**
  (see the redaction rules in `AGENTS.md`).
- **Request a feature** — open an issue describing the use case, who calls the
  server, and the external system it would talk to (this drives the transport
  and auth decisions, see `SPEC.md`).
- **Open a pull/merge request** — describe the change and link the related
  issue. Every change must keep CI green (see below).

## Development workflow

Work is done **test-first** (TDD):

1. **`@qa`** writes the tests first, using an **isolated context**.
2. **`@developer`** writes the implementation, using an **isolated context**.

Do not weaken tests to make an implementation pass.

## Quality gates (CI enforces these)

CI binds to the **build-system interface** (R07) — it calls `make <target>`,
not raw language-specific commands:

```bash
make lint               # all linters (golangci-lint + vet + gosec + govulncheck + go-arch-lint)
make test               # unit tests + -race + coverage >= 95% gate
make build              # binaries for all architectures via goreleaser -> dist/
make e2e                # e2e (only when e2e tests exist; needs Docker)
make container-image    # build image from the `make build` artifact (Remote/Hybrid only)
```

- **Coverage >= 95%** — `make test` fails below the threshold. Add tests with
  the code.
- **Mutation testing** (gremlins) — a hard gate at an **80% floor**. Weak tests
  are a defect; improve them, don't lower the threshold.
- **gosec / govulncheck / go-arch-lint / gitleaks** — findings are **fixed,
  not suppressed**. Never blanket-suppress a security finding.
- **e2e tests** (if present) — exercise the path between the MCP tool handler
  and the real backend/upstream; they are build-tagged and gated in CI.

## Definition of done

A change is done only when CI is green: lint, `-race` tests with coverage
>= 95%, gosec, govulncheck, go-arch-lint, the mutation gate, and — when e2e
tests exist — the `e2e` job.

## Code of Conduct

This project adheres to the [Code of Conduct](CODE_OF_CONDUCT.md). By
participating you are expected to uphold this code.
