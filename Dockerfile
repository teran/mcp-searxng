FROM golang:1.26-alpine AS build
RUN apk add --no-cache ca-certificates && \
    echo 'nobody:x:65534:65534:nobody:/:/sbin/nologin' > /etc/passwd-minimal
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /mcp-searxng ./cmd/server

FROM scratch
COPY --from=build /etc/passwd-minimal /etc/passwd
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=build /mcp-searxng /mcp-searxng
USER 65534:65534
EXPOSE 8080
ENTRYPOINT ["/mcp-searxng"]
LABEL org.opencontainers.image.source="https://github.com/teran/mcp-searxng"
LABEL org.opencontainers.image.description="Remote MCP server for SearXNG"
LABEL org.opencontainers.image.licenses="Apache-2.0"
