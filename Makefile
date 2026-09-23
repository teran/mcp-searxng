# =============================================================================
# mcp-searxng — Makefile (Go profile)
#
# Language-agnostic build-system INTERFACE (R07). CI binds to this interface
# and calls `make <target>` instead of raw language-specific commands, so these
# targets stay identical across CI providers (GitHub / GitLab / Forgejo).
#
# Interface targets:
#   make build                         build binaries for ALL intended
#                                      architectures (Go: goreleaser) into dist/
#                                      — single artifact (B01/B02/B04)
#   make container-image               build the image ONLY from the `build`
#                                      artifact (B04); Remote/Hybrid only
#   make container-image push=true     build + push the image (needs IMAGE + creds)
#   make test                          unit tests + coverage >= 95% gate
#                                      (+ -race in Go) (C01/C04GO)
#   make lint                          linters (golangci-lint incl. gosec +
#                                      govulncheck; go-arch-lint added later)
#   make e2e                           e2e via go-docker-testsuite (C09GO;
#                                      needs Docker) — added in a later fix
#
# gremlins (mutation, C08GO) and gitleaks (secret scan, C03) are NOT part of
# this interface — they stay as DEDICATED hard-gate CI jobs (R07/N31).
#
# A CI job MUST NOT call a target that is not declared here (N31).
# =============================================================================

# Binary name (see cmd/<server>).
SERVER ?= mcp-searxng

# Container image ref (Remote/Hybrid only).
IMAGE  ?= ghcr.io/teran/mcp-searxng

# Additional image tags (comma-separated, e.g. "master,master-abc,master-ts").
# Kept out of the required R03/R04 tag scheme here so the interface stays
# minimal; workflows pass the concrete computed tags via IMAGE_TAGS.
IMAGE_TAGS ?=

# `make container-image push=true` -> add --push (needs IMAGE + registry creds).
PUSH   ?= false

# Multi-arch platform matrix for the container image.
PLATFORMS ?= linux/amd64,linux/arm64

.PHONY: build container-image stage-platform-binaries test e2e lint fmt clean

## Build binaries for ALL intended architectures into dist/ (single artifact).
## Uses goreleaser so the same artifact feeds both `container-image` and the
## binary release (B01/B02/B04) — never a separate in-image compile (N18).
build:
	goreleaser release --skip=publish --clean

## Build the container image ONLY from the `build` artifact in dist/ (B04),
## without recompiling inside the image (N18). `make container-image push=true`
## also pushes and requires `IMAGE` + registry credentials.
## Requires Docker buildx. Declared for this Remote/Hybrid server (B03/R01).
container-image:
	@set -e; \
	$(MAKE) stage-platform-binaries; \
	TAGS="-t $(IMAGE)"; \
	for t in $$(echo "$(IMAGE_TAGS)" | tr ',' ' '); do \
		[ -n "$$t" ] && TAGS="$$TAGS -t $(IMAGE):$$t"; \
	done; \
	if [ "$(PUSH)" = "true" ]; then \
		echo ">> docker buildx build --push $(IMAGE) (tags: $(IMAGE_TAGS))"; \
		docker buildx build --push --platform $(PLATFORMS) $$TAGS .; \
	else \
		echo ">> docker buildx build --load $(IMAGE) (tags: $(IMAGE_TAGS))"; \
		docker buildx build --load --platform $(PLATFORMS) $$TAGS .; \
	fi

# Stage per-platform binaries from the goreleaser `build` artifact (dist/) into
# the flat names the Dockerfile expects (B04 — same binaries, no recompile).
stage-platform-binaries:
	@for arch in amd64 arm64; do \
		dir=$$(find dist -maxdepth 1 -type d -name "$(SERVER)_linux_$${arch}_*" | head -n1); \
		if [ -n "$$dir" ]; then cp "$$dir/$(SERVER)" "./$(SERVER)-$${arch}"; fi; \
	done

## Unit tests with the race detector + a >= 95% coverage gate (C01/C04GO).
test:
	go test -race -coverprofile=cover.out -covermode=atomic ./...
	@total=$$(go tool cover -func=cover.out | awk '/^total:/ {gsub("%","",$$3); print $$3}'); \
	echo "total coverage: $${total}%"; \
	if awk "BEGIN { exit !($${total} < 95) }"; then \
		echo "ERROR: coverage $${total}% is BELOW the 95% threshold" >&2; \
		exit 1; \
	fi; \
	echo "coverage $${total}% meets the >= 95% threshold."

## e2e via the go-docker-testsuite harness (github.com/teran/go-docker-testsuite,
## C09GO). Requires a running Docker daemon. Files carry `//go:build e2e` so
## they are excluded from the default unit run (`make test`).
## OPTIONAL — declared in a later fix once e2e tests exist (N31).
e2e:
	go test -tags e2e ./...

## Linters. gosec runs inside golangci-lint. go-arch-lint is added once the
## .go-arch-lint.yml rules land (C07GO). Findings must be FIXED (C05GO/C06GO).
lint:
	golangci-lint run ./...
	go vet ./...
	govulncheck ./...

## Optional — gofmt formatting check.
fmt:
	gofmt -l .

## Optional — clean build artifacts.
clean:
	rm -rf dist/
