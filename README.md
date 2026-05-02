# Vexlo

Vexlo is a self-hosted localhost tunnel written in Go. It exposes a local port through a server, captures requests and responses, persists them to SQLite, and provides a terminal-style dashboard for replay and mutation.

## What is included

- Binary TCP tunnel client/server
- SQLite persistence for sessions, requests, and replays
- Embedded dashboard assets with live updates over WebSocket
- Replay with request mutation
- GitHub Actions release workflow

## Positioning

Vexlo is currently packaged as a self-hosted tunnel server.

- You can run it locally for development.
- You can deploy it on your own VPS when you have infrastructure.
- You do not need an official hosted Vexlo service to use it.

## Project layout

```text
cmd/server          Server entrypoint
cmd/client          Client CLI entrypoint
internal/server     HTTP/API/tunnel server implementation
internal/client     Tunnel client implementation
internal/model      Shared data models
internal/storage    SQLite layer
internal/dashboard  Embedded dashboard HTML/CSS/JS + websocket hub
internal/protocol   Framed transport protocol
```

## Local development

### 1. Prerequisites

- Go 1.22 or newer

### 2. Run the server

```bash
go run ./cmd/server \
  --http-addr :8080 \
  --tcp-addr :9000 \
  --base-domain localhost \
  --host-url http://localhost:8080 \
  --capture-body-limit 262144 \
  --registration-token change-me-dev-token
```

This starts:

- Dashboard/API on `http://localhost:8080`
- Binary tunnel listener on `127.0.0.1:9000`
- SQLite database at `./vexlo.db` by default, configurable with `--db`
- Health endpoint at `http://localhost:8080/healthz`

By default, Vexlo stores up to `256 KiB` of each request and response body for dashboard history and replay. Use `--capture-body-limit 0` to disable the limit.
Tunnel registration requires `--registration-token`.

With defaults, this can usually be shortened to:

```bash
go run ./cmd/server --registration-token change-me-dev-token
```

### 3. Run your local app

For example, start your app on port `3000`.

### 4. Start the Vexlo client

Binary mode:

```bash
go run ./cmd/client --server 127.0.0.1:9000 --register-token change-me-dev-token 3000
```

With defaults, this can usually be shortened to:

```bash
go run ./cmd/client --register-token change-me-dev-token 3000
```

The client prints:

- Public tunnel path, for local development this is `http://localhost:8080/t/<subdomain>`
- Dashboard URL with the session token baked in

### 5. Use the dashboard

Open the printed dashboard URL. From there you can:

- Inspect requests in real time
- Replay requests
- Mutate headers/body before replay

The dashboard source lives in:

- `internal/dashboard/dashboard.html`
- `internal/dashboard/dashboard.css`
- `internal/dashboard/dashboard.js`

## Build binaries

```bash
go build -o dist/vexlo-server ./cmd/server
go build -o dist/vexlo ./cmd/client
```

Cross-platform release artifacts are built automatically by the tag-triggered GitHub release workflow in [.github/workflows/release.yml](.github/workflows/release.yml).

## Release

For a public release, follow [RELEASE.md](RELEASE.md).

The intended first public version is `v0.1.0`.

Release assets include:

- Linux server archives for `amd64` and `arm64`
- Linux client archives for `amd64` and `arm64`
- macOS client archives for `amd64` and `arm64`
- Windows client zip for `amd64`
- `SHA256SUMS.txt`

## Linting

Linting is enabled through GitHub Actions and `golangci-lint`.

Current checks:

- `gofmt` formatting check
- `golangci-lint`
- `go build ./...`

## Local checks

Install `golangci-lint` locally first:

<https://golangci-lint.run/welcome/install/>

Windows PowerShell:

```powershell
.\scripts\check.ps1
```

Linux or macOS:

```bash
./scripts/check.sh
```

Equivalent manual commands:

```bash
gofmt -l cmd internal
golangci-lint run
go build ./...
```

## Production notes

- For real public subdomains, run the server behind a DNS name and pass `--base-domain your-domain.example`.
- For TLS, start the server with `--tls`, expose ports `80` and `443`, and point your base domain plus subdomains at the VPS.
- The current local-first flow uses `/t/<subdomain>` when `--base-domain localhost`.
- For production, set a non-empty `--registration-token` and distribute it only to trusted clients.
- The dashboard token is now moved into an `HttpOnly` cookie and stripped from the browser address bar on first load.
- Configure `--retention-period` explicitly based on your storage and compliance requirements.
- Configure `--max-request-body-bytes`, `--max-api-body-bytes`, and the HTTP timeout flags for your workload instead of relying on defaults.
- API responses include `X-Request-Id`, and server logs now emit request IDs plus status and duration metadata for API and WebSocket endpoints.
- Generic deployment artifacts for `systemd` and self-hosting are in [deploy/README.md](deploy/README.md).

## Production server example

```bash
./vexlo-server \
  --tls \
  --http-addr :80 \
  --https-addr :443 \
  --tcp-addr :9000 \
  --base-domain vexlo.example.com \
  --host-url https://vexlo.example.com \
  --registration-token replace-with-strong-secret \
  --capture-body-limit 262144 \
  --retention-period 168h \
  --acme-email you@example.com \
  --acme-cache ./acme-cache
```

## Deployment

The repo includes ready-to-use deployment files for a Linux VPS:

- [deploy/systemd/vexlo.service](deploy/systemd/vexlo.service)
- [deploy/env/vexlo.env.example](deploy/env/vexlo.env.example)
- [deploy/scripts/install_ubuntu.sh](deploy/scripts/install_ubuntu.sh)

Typical flow on an Ubuntu VPS:

```bash
chmod +x deploy/scripts/install_ubuntu.sh
sudo ./deploy/scripts/install_ubuntu.sh \
  vexlo.example.com \
  https://vexlo.example.com \
  you@example.com \
  https://github.com/yourorg/vexlo/releases/latest/download/vexlo-server-linux-amd64.tar.gz
sudo systemctl start vexlo
sudo journalctl -u vexlo -f
```

## Verification

```bash
go build ./...
```

For release history, see [CHANGELOG.md](CHANGELOG.md).
